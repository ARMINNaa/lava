package keeper

import (
	"fmt"
	"math"
	"math/big"

	sdk "github.com/cosmos/cosmos-sdk/types"
	commontypes "github.com/lavanet/lava/common/types"
	"github.com/lavanet/lava/utils"
	epochstoragetypes "github.com/lavanet/lava/x/epochstorage/types"
	pairingfilters "github.com/lavanet/lava/x/pairing/keeper/filters"
	projectstypes "github.com/lavanet/lava/x/projects/types"
	spectypes "github.com/lavanet/lava/x/spec/types"
	tendermintcrypto "github.com/tendermint/tendermint/crypto"
)

func (k Keeper) VerifyPairingData(ctx sdk.Context, chainID string, clientAddress sdk.AccAddress, block uint64) (epoch uint64, errorRet error) {
	// TODO: add support for spec changes
	foundAndActive, _ := k.specKeeper.IsSpecFoundAndActive(ctx, chainID)
	if !foundAndActive {
		return 0, fmt.Errorf("spec not found and active for chainID given: %s", chainID)
	}
	earliestSavedEpoch := k.epochStorageKeeper.GetEarliestEpochStart(ctx)
	if block < earliestSavedEpoch {
		return 0, fmt.Errorf("block %d is earlier than earliest saved block %d", block, earliestSavedEpoch)
	}

	requestedEpochStart, _, err := k.epochStorageKeeper.GetEpochStartForBlock(ctx, block)
	if err != nil {
		return 0, err
	}
	currentEpochStart := k.epochStorageKeeper.GetEpochStart(ctx)

	if requestedEpochStart > currentEpochStart {
		return 0, utils.LavaFormatWarning("VerifyPairing requested epoch is too new", fmt.Errorf("cant get epoch start for future block"),
			utils.Attribute{Key: "requested block", Value: block},
			utils.Attribute{Key: "requested epoch", Value: requestedEpochStart},
			utils.Attribute{Key: "current epoch", Value: currentEpochStart},
		)
	}

	blocksToSave, err := k.epochStorageKeeper.BlocksToSave(ctx, uint64(ctx.BlockHeight()))
	if err != nil {
		return 0, err
	}

	if requestedEpochStart+blocksToSave < currentEpochStart {
		return 0, fmt.Errorf("requestedEpochStart %d is earlier current epoch %d by more than BlocksToSave %d", requestedEpochStart, currentEpochStart, blocksToSave)
	}
	return requestedEpochStart, nil
}

func (k Keeper) VerifyClientStake(ctx sdk.Context, chainID string, clientAddress sdk.Address, block uint64, epoch uint64) (clientStakeEntryRet *epochstoragetypes.StakeEntry, errorRet error) {
	verifiedUser := false

	// we get the user stakeEntries at the time of check. for unstaking users, we make sure users can't unstake sooner than blocksToSave so we can charge them if the pairing is valid
	userStakedEntries, found, _ := k.epochStorageKeeper.GetEpochStakeEntries(ctx, epoch, chainID)
	if !found {
		return nil, utils.LavaFormatWarning("no EpochStakeEntries entries at all for this spec", fmt.Errorf("user stake entries not found"),
			utils.Attribute{Key: "chainID", Value: chainID},
			utils.Attribute{Key: "query Epoch", Value: epoch},
			utils.Attribute{Key: "query block", Value: block},
		)
	}
	for i, clientStakeEntry := range userStakedEntries {
		clientAddr, err := sdk.AccAddressFromBech32(clientStakeEntry.Address)
		if err != nil {
			panic(fmt.Sprintf("invalid user address saved in keeper %s, err: %s", clientStakeEntry.Address, err))
		}
		if clientAddr.Equals(clientAddress) {
			if clientStakeEntry.StakeAppliedBlock > block {
				// client is not valid for new pairings yet, or was jailed
				return nil, fmt.Errorf("found staked user %+v, but his stakeAppliedBlock %d, was bigger than checked block: %d", clientStakeEntry, clientStakeEntry.StakeAppliedBlock, block)
			}
			verifiedUser = true
			clientStakeEntryRet = &userStakedEntries[i]
			break
		}
	}
	if !verifiedUser {
		return nil, fmt.Errorf("client: %s isn't staked for spec %s at block %d", clientAddress, chainID, block)
	}
	return clientStakeEntryRet, nil
}

func (k Keeper) GetProjectData(ctx sdk.Context, developerKey sdk.AccAddress, chainID string, blockHeight uint64) (proj projectstypes.Project, errRet error) {
	project, err := k.projectsKeeper.GetProjectForDeveloper(ctx, developerKey.String(), blockHeight)
	if err != nil {
		return projectstypes.Project{}, err
	}

	if !project.Enabled {
		return projectstypes.Project{}, utils.LavaFormatWarning("the developers project is disabled", fmt.Errorf("cannot get project data"),
			utils.Attribute{Key: "project", Value: project.Index},
		)
	}

	return project, nil
}

func (k Keeper) GetPairingForClient(ctx sdk.Context, chainID string, clientAddress sdk.AccAddress) (providers []epochstoragetypes.StakeEntry, errorRet error) {
	providers, _, err := k.getPairingForClient(ctx, chainID, clientAddress, uint64(ctx.BlockHeight()))
	return providers, err
}

// function used to get a new pairing from provider and client
// first argument has all metadata, second argument is only the addresses
func (k Keeper) getPairingForClient(ctx sdk.Context, chainID string, clientAddress sdk.AccAddress, block uint64) (providers []epochstoragetypes.StakeEntry, allowedCU uint64, errorRet error) {
	var strictestPolicy projectstypes.Policy

	epoch, err := k.VerifyPairingData(ctx, chainID, clientAddress, block)
	if err != nil {
		return nil, 0, fmt.Errorf("invalid pairing data: %s", err)
	}

	project, err := k.GetProjectData(ctx, clientAddress, chainID, block)
	if err != nil {
		return nil, 0, err
	}

	strictestPolicy, allowedCU, err = k.getProjectStrictestPolicy(ctx, project, chainID)
	if err != nil {
		return nil, 0, fmt.Errorf("invalid user for pairing: %s", err.Error())
	}

	possibleProviders, found, epochHash := k.epochStorageKeeper.GetEpochStakeEntries(ctx, epoch, chainID)
	if !found {
		return nil, 0, fmt.Errorf("did not find providers for pairing: epoch:%d, chainID: %s", block, chainID)
	}

	filters := pairingfilters.GetAllFilters()

	possibleProviders, err = pairingfilters.FilterProviders(ctx, filters, possibleProviders, strictestPolicy, epoch)
	if err != nil {
		return nil, 0, err
	}

	providers, err = k.calculatePairingForClient(ctx, possibleProviders, project.Index, block,
		chainID, epochHash, strictestPolicy.MaxProvidersToPair)

	return providers, allowedCU, err
}

func (k Keeper) getProjectStrictestPolicy(ctx sdk.Context, project projectstypes.Project, chainID string) (projectstypes.Policy, uint64, error) {
	plan, err := k.subscriptionKeeper.GetPlanFromSubscription(ctx, project.GetSubscription())
	if err != nil {
		return projectstypes.Policy{}, 0, err
	}

	planPolicy := plan.GetPlanPolicy()
	policies := []*projectstypes.Policy{&planPolicy}
	if project.SubscriptionPolicy != nil {
		policies = append(policies, project.SubscriptionPolicy)
	}
	if project.AdminPolicy != nil {
		policies = append(policies, project.AdminPolicy)
	}

	if !projectstypes.CheckChainIdExistsInPolicies(chainID, policies) {
		return projectstypes.Policy{}, 0, fmt.Errorf("chain ID not found in any of the policies")
	}

	geolocation := k.CalculateEffectiveGeolocationFromPolicies(policies)

	providersToPair, err := k.CalculateEffectiveProvidersToPairFromPolicies(policies)
	if err != nil {
		return projectstypes.Policy{}, 0, err
	}

	sub, found := k.subscriptionKeeper.GetSubscription(ctx, project.GetSubscription())
	if !found {
		return projectstypes.Policy{}, 0, fmt.Errorf("could not find subscription with address %s", project.GetSubscription())
	}
	allowedCU := k.CalculateEffectiveAllowedCuPerEpochFromPolicies(policies, project.GetUsedCu(), sub.GetMonthCuLeft())

	selectedProvidersMode, selectedProvidersList := k.CalculateEffectiveSelectedProviders(policies)

	strictestPolicy := projectstypes.Policy{
		GeolocationProfile:    geolocation,
		MaxProvidersToPair:    providersToPair,
		SelectedProvidersMode: selectedProvidersMode,
		SelectedProviders:     selectedProvidersList,
	}

	return strictestPolicy, allowedCU, nil
}

func (k Keeper) CalculateEffectiveSelectedProviders(policies []*projectstypes.Policy) (projectstypes.SELECTED_PROVIDERS_MODE, []string) {
	selectedProvidersModeList := []projectstypes.SELECTED_PROVIDERS_MODE{}
	selectedProvidersList := [][]string{}
	for _, p := range policies {
		selectedProvidersModeList = append(selectedProvidersModeList, p.SelectedProvidersMode)
		if p.SelectedProvidersMode == projectstypes.SELECTED_PROVIDERS_MODE_EXCLUSIVE || p.SelectedProvidersMode == projectstypes.SELECTED_PROVIDERS_MODE_MIXED {
			if len(p.SelectedProviders) != 0 {
				selectedProvidersList = append(selectedProvidersList, p.SelectedProviders)
			}
		}
	}

	effectiveMode := commontypes.FindMax(selectedProvidersModeList)
	effectiveSelectedProviders := commontypes.Intersection(selectedProvidersList...)

	return effectiveMode, effectiveSelectedProviders
}

func (k Keeper) CalculateEffectiveGeolocationFromPolicies(policies []*projectstypes.Policy) uint64 {
	geolocation := uint64(math.MaxUint64)

	// geolocation is a bitmap. common denominator can be calculated with logical AND
	for _, policy := range policies {
		if policy != nil {
			geolocation &= policy.GetGeolocationProfile()
		}
	}

	return geolocation
}

func (k Keeper) CalculateEffectiveProvidersToPairFromPolicies(policies []*projectstypes.Policy) (uint64, error) {
	providersToPair := uint64(math.MaxUint64)

	for _, policy := range policies {
		val := policy.GetMaxProvidersToPair()
		if policy != nil && val < providersToPair {
			providersToPair = val
		}
	}

	if providersToPair == uint64(math.MaxUint64) {
		return 0, fmt.Errorf("could not calculate providersToPair value: all policies are nil")
	}

	return providersToPair, nil
}

func (k Keeper) CalculateEffectiveAllowedCuPerEpochFromPolicies(policies []*projectstypes.Policy, cuUsedInProject uint64, cuLeftInSubscription uint64) uint64 {
	var policyEpochCuLimit []uint64
	var policyTotalCuLimit []uint64
	for _, policy := range policies {
		if policy != nil {
			policyEpochCuLimit = append(policyEpochCuLimit, policy.GetEpochCuLimit())
			policyTotalCuLimit = append(policyTotalCuLimit, policy.GetTotalCuLimit())
		}
	}

	effectiveTotalCuOfProject := commontypes.FindMin(policyTotalCuLimit)
	cuLeftInProject := effectiveTotalCuOfProject - cuUsedInProject

	effectiveEpochCuOfProject := commontypes.FindMin(policyEpochCuLimit)

	return commontypes.FindMin([]uint64{effectiveEpochCuOfProject, cuLeftInProject, cuLeftInSubscription})
}

func (k Keeper) ValidatePairingForClient(ctx sdk.Context, chainID string, clientAddress sdk.AccAddress, providerAddress sdk.AccAddress, epoch uint64) (isValidPairing bool, allowedCU uint64, pairedProviders uint64, errorRet error) {
	epoch, _, err := k.epochStorageKeeper.GetEpochStartForBlock(ctx, epoch)
	if err != nil {
		return false, allowedCU, 0, err
	}

	validAddresses, allowedCU, err := k.getPairingForClient(ctx, chainID, clientAddress, epoch)
	if err != nil {
		return false, allowedCU, 0, err
	}

	for _, possibleAddr := range validAddresses {
		providerAccAddr, err := sdk.AccAddressFromBech32(possibleAddr.Address)
		if err != nil {
			panic(fmt.Sprintf("invalid provider address saved in keeper %s, err: %s", providerAccAddr, err))
		}

		if providerAccAddr.Equals(providerAddress) {
			return true, allowedCU, uint64(len(validAddresses)), nil
		}
	}

	return false, allowedCU, 0, nil
}

func (k Keeper) calculatePairingForClient(ctx sdk.Context, providers []epochstoragetypes.StakeEntry, developerAddress string, epochStartBlock uint64, chainID string, epochHash []byte, providersToPair uint64) (validProviders []epochstoragetypes.StakeEntry, err error) {
	if epochStartBlock > uint64(ctx.BlockHeight()) {
		k.Logger(ctx).Error("\ninvalid session start\n")
		panic(fmt.Sprintf("invalid session start saved in keeper %d, current block was %d", epochStartBlock, uint64(ctx.BlockHeight())))
	}

	spec, found := k.specKeeper.GetSpec(ctx, chainID)
	if !found {
		return nil, fmt.Errorf("spec not found or not enabled")
	}

	if spec.ProvidersTypes == spectypes.Spec_dynamic {
		// calculates a hash and randomly chooses the providers

		validProviders = k.returnSubsetOfProvidersByStake(ctx, developerAddress, providers, providersToPair, epochStartBlock, chainID, epochHash)
	} else {
		validProviders = k.returnSubsetOfProvidersByHighestStake(ctx, providers, providersToPair)
	}

	return validProviders, nil
}

// this function randomly chooses count providers by weight
func (k Keeper) returnSubsetOfProvidersByStake(ctx sdk.Context, clientAddress string, providersMaps []epochstoragetypes.StakeEntry, count uint64, block uint64, chainID string, epochHash []byte) (returnedProviders []epochstoragetypes.StakeEntry) {
	stakeSum := sdk.NewCoin(epochstoragetypes.TokenDenom, sdk.NewInt(0))
	hashData := make([]byte, 0)
	for _, stakedProvider := range providersMaps {
		stakeSum = stakeSum.Add(stakedProvider.Stake)
	}
	if stakeSum.IsZero() {
		// list is empty
		return returnedProviders
	}

	// add the session start block hash to the function to make it as unpredictable as we can
	hashData = append(hashData, epochHash...)
	hashData = append(hashData, chainID...)       // to make this pairing unique per chainID
	hashData = append(hashData, clientAddress...) // to make this pairing unique per consumer

	indexToSkip := make(map[int]bool) // a trick to create a unique set in golang
	for it := 0; it < int(count); it++ {
		hash := tendermintcrypto.Sha256(hashData) // TODO: we use cheaper algo for speed
		bigIntNum := new(big.Int).SetBytes(hash)
		hashAsNumber := sdk.NewIntFromBigInt(bigIntNum)
		modRes := hashAsNumber.Mod(stakeSum.Amount)

		newStakeSum := sdk.NewCoin(epochstoragetypes.TokenDenom, sdk.NewInt(0))
		// we loop the servicers list form the end because the list is sorted, biggest is last,
		// and statistically this will have less iterations

		for idx := len(providersMaps) - 1; idx >= 0; idx-- {
			stakedProvider := providersMaps[idx]
			if indexToSkip[idx] {
				// this is an index we added
				continue
			}
			newStakeSum = newStakeSum.Add(stakedProvider.Stake)
			if modRes.LT(newStakeSum.Amount) {
				// we hit our chosen provider
				returnedProviders = append(returnedProviders, stakedProvider)
				stakeSum = stakeSum.Sub(stakedProvider.Stake) // we remove this provider from the random pool, so the sum is lower now
				indexToSkip[idx] = true
				break
			}
		}
		if uint64(len(returnedProviders)) >= count {
			return returnedProviders
		}
		if stakeSum.IsZero() {
			break
		}
		hashData = append(hashData, []byte{uint8(it)}...)
	}
	return returnedProviders
}

func (k Keeper) returnSubsetOfProvidersByHighestStake(ctx sdk.Context, providersEntries []epochstoragetypes.StakeEntry, count uint64) (returnedProviders []epochstoragetypes.StakeEntry) {
	if uint64(len(providersEntries)) <= count {
		return providersEntries
	} else {
		return providersEntries[0:count]
	}
}

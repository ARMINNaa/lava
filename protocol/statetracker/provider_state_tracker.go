package statetracker

import (
	"context"

	"github.com/cosmos/cosmos-sdk/client"
	"github.com/cosmos/cosmos-sdk/client/tx"
	"github.com/lavanet/lava/protocol/chaintracker"
	"github.com/lavanet/lava/protocol/lavasession"
	"github.com/lavanet/lava/protocol/rpcprovider/reliabilitymanager"
	"github.com/lavanet/lava/utils"
	pairingtypes "github.com/lavanet/lava/x/pairing/types"
	protocoltypes "github.com/lavanet/lava/x/protocol/types"
)

// ProviderStateTracker PST is a class for tracking provider data from the lava blockchain, such as epoch changes.
// it allows also to query specific data form the blockchain and acts as a single place to send transactions
type ProviderStateTracker struct {
	stateQuery *ProviderStateQuery
	txSender   *ProviderTxSender
	*StateTracker
}

func NewProviderStateTracker(ctx context.Context, txFactory tx.Factory, clientCtx client.Context, chainFetcher chaintracker.ChainFetcher) (ret *ProviderStateTracker, err error) {
	stateTrackerBase, err := NewStateTracker(ctx, txFactory, clientCtx, chainFetcher)
	if err != nil {
		return nil, err
	}
	txSender, err := NewProviderTxSender(ctx, clientCtx, txFactory)
	if err != nil {
		return nil, err
	}
	pst := &ProviderStateTracker{StateTracker: stateTrackerBase, stateQuery: NewProviderStateQuery(ctx, clientCtx), txSender: txSender}
	return pst, nil
}

func (pst *ProviderStateTracker) RegisterForEpochUpdates(ctx context.Context, epochUpdatable EpochUpdatable) {
	epochUpdater := NewEpochUpdater(&pst.stateQuery.EpochStateQuery)
	epochUpdaterRaw := pst.StateTracker.RegisterForUpdates(ctx, epochUpdater)
	epochUpdater, ok := epochUpdaterRaw.(*EpochUpdater)
	if !ok {
		utils.LavaFormatFatal("invalid updater type returned from RegisterForUpdates", nil, utils.Attribute{Key: "updater", Value: epochUpdaterRaw})
	}
	epochUpdater.RegisterEpochUpdatable(ctx, epochUpdatable)
}

func (pst *ProviderStateTracker) RegisterForSpecUpdates(ctx context.Context, specUpdatable SpecUpdatable, endpoint lavasession.RPCEndpoint) error {
	// register for spec updates sets spec and updates when a spec has been modified
	specUpdater := NewSpecUpdater(endpoint.ChainID, pst.stateQuery, pst.EventTracker)
	specUpdaterRaw := pst.StateTracker.RegisterForUpdates(ctx, specUpdater)
	specUpdater, ok := specUpdaterRaw.(*SpecUpdater)
	if !ok {
		utils.LavaFormatFatal("invalid updater type returned from RegisterForUpdates", nil, utils.Attribute{Key: "updater", Value: specUpdaterRaw})
	}
	return specUpdater.RegisterSpecUpdatable(ctx, &specUpdatable, endpoint)
}

func (pst *ProviderStateTracker) RegisterForVersionUpdates(ctx context.Context, version *protocoltypes.Version, versionValidator VersionValidationInf) {
	versionUpdater := NewVersionUpdater(pst.stateQuery, pst.EventTracker, version, versionValidator)
	versionUpdaterRaw := pst.StateTracker.RegisterForUpdates(ctx, versionUpdater)
	versionUpdater, ok := versionUpdaterRaw.(*VersionUpdater)
	if !ok {
		utils.LavaFormatFatal("invalid updater type returned from RegisterForUpdates", nil, utils.Attribute{Key: "updater", Value: versionUpdaterRaw})
	}
	versionUpdater.RegisterVersionUpdatable()
}

func (pst *ProviderStateTracker) RegisterReliabilityManagerForVoteUpdates(ctx context.Context, voteUpdatable VoteUpdatable, endpointP *lavasession.RPCProviderEndpoint) {
	voteUpdater := NewVoteUpdater(pst.EventTracker)
	voteUpdaterRaw := pst.StateTracker.RegisterForUpdates(ctx, voteUpdater)
	voteUpdater, ok := voteUpdaterRaw.(*VoteUpdater)
	if !ok {
		utils.LavaFormatFatal("invalid updater type returned from RegisterForUpdates", nil, utils.Attribute{Key: "updater", Value: voteUpdaterRaw})
	}
	endpoint := lavasession.RPCEndpoint{ChainID: endpointP.ChainID, ApiInterface: endpointP.ApiInterface}
	voteUpdater.RegisterVoteUpdatable(ctx, &voteUpdatable, endpoint)
}

func (pst *ProviderStateTracker) RegisterPaymentUpdatableForPayments(ctx context.Context, paymentUpdatable PaymentUpdatable) {
	payemntUpdater := NewPaymentUpdater(pst.EventTracker)
	payemntUpdaterRaw := pst.StateTracker.RegisterForUpdates(ctx, payemntUpdater)
	payemntUpdater, ok := payemntUpdaterRaw.(*PaymentUpdater)
	if !ok {
		utils.LavaFormatFatal("invalid updater type returned from RegisterForUpdates", nil, utils.Attribute{Key: "updater", Value: payemntUpdaterRaw})
	}

	payemntUpdater.RegisterPaymentUpdatable(ctx, &paymentUpdatable)
}

func (pst *ProviderStateTracker) TxRelayPayment(ctx context.Context, relayRequests []*pairingtypes.RelaySession, description string, latestBlocks []*pairingtypes.LatestBlockReport) error {
	return pst.txSender.TxRelayPayment(ctx, relayRequests, description, latestBlocks)
}

func (pst *ProviderStateTracker) SendVoteReveal(voteID string, vote *reliabilitymanager.VoteData) error {
	return pst.txSender.SendVoteReveal(voteID, vote)
}

func (pst *ProviderStateTracker) SendVoteCommitment(voteID string, vote *reliabilitymanager.VoteData) error {
	return pst.txSender.SendVoteCommitment(voteID, vote)
}

func (pst *ProviderStateTracker) LatestBlock() int64 {
	return pst.StateTracker.chainTracker.GetLatestBlockNum()
}

func (pst *ProviderStateTracker) GetMaxCuForUser(ctx context.Context, consumerAddress, chainID string, epoch uint64) (maxCu uint64, err error) {
	return pst.stateQuery.GetMaxCuForUser(ctx, consumerAddress, chainID, epoch)
}

func (pst *ProviderStateTracker) VerifyPairing(ctx context.Context, consumerAddress, providerAddress string, epoch uint64, chainID string) (valid bool, total int64, projectId string, err error) {
	return pst.stateQuery.VerifyPairing(ctx, consumerAddress, providerAddress, epoch, chainID)
}

func (pst *ProviderStateTracker) GetEpochSize(ctx context.Context) (uint64, error) {
	return pst.stateQuery.GetEpochSize(ctx)
}

func (pst *ProviderStateTracker) EarliestBlockInMemory(ctx context.Context) (uint64, error) {
	return pst.stateQuery.EarliestBlockInMemory(ctx)
}

func (pst *ProviderStateTracker) GetRecommendedEpochNumToCollectPayment(ctx context.Context) (uint64, error) {
	return pst.stateQuery.GetRecommendedEpochNumToCollectPayment(ctx)
}

func (pst *ProviderStateTracker) GetEpochSizeMultipliedByRecommendedEpochNumToCollectPayment(ctx context.Context) (uint64, error) {
	return pst.stateQuery.GetEpochSizeMultipliedByRecommendedEpochNumToCollectPayment(ctx)
}

func (pst *ProviderStateTracker) GetProtocolVersion(ctx context.Context) (*protocoltypes.Version, error) {
	return pst.stateQuery.GetProtocolVersion(ctx)
}

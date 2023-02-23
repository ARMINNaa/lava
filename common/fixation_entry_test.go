package common_test

import (
	"encoding/binary"
	"testing"

	"github.com/cosmos/cosmos-sdk/codec"
	codectypes "github.com/cosmos/cosmos-sdk/codec/types"
	"github.com/cosmos/cosmos-sdk/store"
	"github.com/cosmos/cosmos-sdk/store/prefix"
	storetypes "github.com/cosmos/cosmos-sdk/store/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/lavanet/lava/common"
	"github.com/lavanet/lava/common/types"
	"github.com/stretchr/testify/require"
	"github.com/tendermint/tendermint/libs/log"
	tmproto "github.com/tendermint/tendermint/proto/tendermint/types"
	tmdb "github.com/tendermint/tm-db"
)

// Helper function to init a mock keeper and context
func initCtxAndVersionedStore(t *testing.T) (*common.VersionedStore, sdk.Context) {
	db := tmdb.NewMemDB()
	stateStore := store.NewCommitMultiStore(db)

	registry := codectypes.NewInterfaceRegistry()
	cdc := codec.NewProtoCodec(registry)

	mockStoreKey := sdk.NewKVStoreKey("storeKey")
	mockMemStoreKey := storetypes.NewMemoryStoreKey("storeMemKey")
	stateStore.MountStoreWithDB(mockStoreKey, sdk.StoreTypeIAVL, db)
	stateStore.MountStoreWithDB(mockMemStoreKey, sdk.StoreTypeMemory, nil)

	require.NoError(t, stateStore.LoadLatestVersion())

	vs := common.NewVersionedStore(mockStoreKey, cdc)

	ctx := sdk.NewContext(stateStore, tmproto.Header{}, false, log.TestingLogger())

	return vs, ctx
}

// Test addition and removal of a fixation entry
func TestFixationEntryAdditionAndRemoval(t *testing.T) {
	// create dummy data for dummy entry
	marshaledData := make([]byte, 8)
	binary.LittleEndian.PutUint64(marshaledData, 1)
	dummyFixationKey := "dummyFix_"
	dummyIndex := "index"
	dummyObj := sdk.Coin{Denom: "utest", Amount: sdk.ZeroInt()}

	// init VersionedStore + context
	vs, ctx := initCtxAndVersionedStore(t)

	// add dummy entry
	blockToAddEntry := uint64(ctx.BlockHeight())
	err := vs.AppendEntry(ctx, dummyFixationKey, dummyIndex, blockToAddEntry, &dummyObj)
	require.Nil(t, err)

	// get all entry indices and make sure there is only one index
	indexList := vs.GetAllEntryIndices(ctx, dummyFixationKey)
	require.Equal(t, 1, len(indexList))

	// get the entry from the storage
	var dummyCoin sdk.Coin
	err = vs.GetEntryForBlock(ctx, dummyFixationKey, dummyIndex, blockToAddEntry, &dummyCoin, types.SUB_REFERENCE)
	require.Nil(t, err)

	// make sure that one entry's data is the same data that was used to create it
	require.True(t, dummyCoin.IsEqual(dummyObj))

	// remove the entry by advancing over the STALE_ENTRY_TIME and appending a new one (append triggers the removal func)
	ctx = ctx.WithBlockHeight(types.STALE_ENTRY_TIME + int64(blockToAddEntry) + 1)
	dummyObj2 := sdk.Coin{Denom: "utest", Amount: sdk.OneInt()}
	blockToAddEntryAfterStale := uint64(ctx.BlockHeight())
	err = vs.AppendEntry(ctx, dummyFixationKey, dummyIndex, blockToAddEntryAfterStale, &dummyObj2)
	require.Nil(t, err)

	// make sure there the old entry was deleted (check block)
	err = vs.GetEntryForBlock(ctx, dummyFixationKey, dummyIndex, blockToAddEntry, &dummyCoin, types.DO_NOTHING)
	require.NotNil(t, err)

	// get the latest version and make sure it's equal to dummyObj2
	err = vs.GetEntryForBlock(ctx, dummyFixationKey, dummyIndex, blockToAddEntryAfterStale, &dummyCoin, types.DO_NOTHING)
	require.Nil(t, err)
	require.True(t, dummyCoin.IsEqual(dummyObj2))

	// make sure dummy index is still in the entry index list
	indexList = vs.GetAllEntryIndices(ctx, dummyFixationKey)
	require.Equal(t, 1, len(indexList))
}

// Test that when adds two entries with the same block and index and makes sure that only the latest one is kept
func TestAdditionOfTwoEntriesWithSameIndexInSameBlock(t *testing.T) {
	// create dummy data for two dummy entries
	dummyIndex := "index"
	dummyFixationKey := "dummyFix_"
	dummyObj := sdk.Coin{Denom: "utest", Amount: sdk.ZeroInt()}
	dummyObj2 := sdk.Coin{Denom: "utest", Amount: sdk.OneInt()}

	// init VersionedStore + context
	vs, ctx := initCtxAndVersionedStore(t)

	// add the first dummy entry
	blockToAddEntry := uint64(0)
	err := vs.AppendEntry(ctx, dummyFixationKey, dummyIndex, blockToAddEntry, &dummyObj)
	require.Nil(t, err)

	// add the second dummy entry
	err = vs.AppendEntry(ctx, dummyFixationKey, dummyIndex, blockToAddEntry, &dummyObj2)
	require.Nil(t, err)

	// get all entry indices and make sure there is only one index
	indexList := vs.GetAllEntryIndices(ctx, dummyFixationKey)
	require.Equal(t, 1, len(indexList))

	// get the entry from the storage
	var dummyCoin sdk.Coin
	err = vs.GetEntryForBlock(ctx, dummyFixationKey, dummyIndex, blockToAddEntry, &dummyCoin, types.DO_NOTHING)
	require.Nil(t, err)

	// make sure that one entry's data is the same data of the second dummy entry
	require.True(t, dummyCoin.IsEqual(dummyObj2))

	// make sure dummy index is still in the entry index list
	indexList = vs.GetAllEntryIndices(ctx, dummyFixationKey)
	require.Equal(t, 1, len(indexList))
}

// Test adding entry versions and getting an older version
func TestEntryVersions(t *testing.T) {
	// create dummy data for two dummy entries
	dummyIndex := "index"
	dummyFixationKey := "dummyFix_"
	dummyObj := sdk.Coin{Denom: "utest", Amount: sdk.ZeroInt()}
	dummyObj2 := sdk.Coin{Denom: "utest", Amount: sdk.OneInt()}

	// init VersionedStore + context
	vs, ctx := initCtxAndVersionedStore(t)

	// add the first dummy entry
	blockToAddFirstEntry := uint64(10)
	err := vs.AppendEntry(ctx, dummyFixationKey, dummyIndex, blockToAddFirstEntry, &dummyObj)
	require.Nil(t, err)

	// add the second dummy entry
	blockToAddSecondEntry := uint64(20)
	err = vs.AppendEntry(ctx, dummyFixationKey, dummyIndex, blockToAddSecondEntry, &dummyObj2)
	require.Nil(t, err)

	// get the older version from block blockToAddFirstEntry
	var dummyCoin sdk.Coin
	found := vs.GetEntry(ctx, dummyFixationKey, dummyIndex, blockToAddFirstEntry, &dummyCoin, types.DO_NOTHING)
	require.True(t, found)

	// verify the data matches the old entry from storage
	require.True(t, dummyCoin.IsEqual(dummyObj))

	// make sure dummy index is still in the entry index list
	indexList := vs.GetAllEntryIndices(ctx, dummyFixationKey)
	require.Equal(t, 1, len(indexList))
}

// Test adding entry versions with different fixation keys
func TestDifferentFixationKeys(t *testing.T) {
	// create dummy data for two dummy entries
	dummyIndex := "index"
	dummyFixationKey := "dummyFix_"
	dummyFixationKey2 := "dummyFix2_"
	dummyObj := sdk.Coin{Denom: "utest", Amount: sdk.ZeroInt()}
	dummyObj2 := sdk.Coin{Denom: "utest", Amount: sdk.OneInt()}
	blockToAddEntry := uint64(10)
	// init VersionedStore + context
	vs, ctx := initCtxAndVersionedStore(t)

	// add the first dummy entry
	err := vs.AppendEntry(ctx, dummyFixationKey, dummyIndex, blockToAddEntry, &dummyObj)
	require.Nil(t, err)

	// add the second dummy entry
	err = vs.AppendEntry(ctx, dummyFixationKey2, dummyIndex, blockToAddEntry, &dummyObj2)
	require.Nil(t, err)

	// get all indices with dummyFixationKey and dummyIndex. make sure there is one entry
	indexList := vs.GetAllEntryIndices(ctx, dummyFixationKey)
	require.Equal(t, 1, len(indexList))

	// verify the data matches the entry from dummyFixationKey storage
	var dummyCoin sdk.Coin
	found := vs.GetEntry(ctx, dummyFixationKey, dummyIndex, blockToAddEntry, &dummyCoin, types.DO_NOTHING)
	require.True(t, found)
	require.True(t, dummyCoin.IsEqual(dummyObj))

	// get all indices with dummyFixationKey2 and dummyIndex. make sure there is one entry
	indexList = vs.GetAllEntryIndices(ctx, dummyFixationKey2)
	require.Equal(t, 1, len(indexList))

	// verify the data matches the entry from dummyFixationKey storage
	found = vs.GetEntry(ctx, dummyFixationKey2, dummyIndex, blockToAddEntry, &dummyCoin, types.DO_NOTHING)
	require.True(t, found)
	require.True(t, dummyCoin.IsEqual(dummyObj2))
}

// Test that the appended entries are sorted (first element is oldest)
func TestEntriesSort(t *testing.T) {
	// create dummy data for two dummy entries
	dummyIndex := "index"
	dummyFixationKey := "dummyFix_"
	dummyObj := sdk.Coin{Denom: "utest", Amount: sdk.ZeroInt()}
	dummyObj2 := sdk.Coin{Denom: "utest", Amount: sdk.OneInt()}
	dummyObj3 := sdk.Coin{Denom: "utest", Amount: sdk.OneInt().Mul(sdk.NewIntFromUint64(2))}

	// init VersionedStore + context
	vs, ctx := initCtxAndVersionedStore(t)

	// add the first dummy entry
	blockToAddEntry := uint64(10)
	err := vs.AppendEntry(ctx, dummyFixationKey, dummyIndex, blockToAddEntry, &dummyObj)
	require.Nil(t, err)

	// add the second dummy entry
	blockToAddEntry += 10
	err = vs.AppendEntry(ctx, dummyFixationKey, dummyIndex, blockToAddEntry, &dummyObj2)
	require.Nil(t, err)

	// add the third dummy entry
	blockToAddEntry += 10
	err = vs.AppendEntry(ctx, dummyFixationKey, dummyIndex, blockToAddEntry, &dummyObj3)
	require.Nil(t, err)

	// get the relevant store and init an iterator and verify the entries are organized from oldest to latest (first element is oldest)
	store := prefix.NewStore(ctx.KVStore(vs.GetStoreKey()), types.KeyPrefix(types.EntryKey+dummyFixationKey+dummyIndex))
	iterator := sdk.KVStorePrefixIterator(store, []byte{})
	defer iterator.Close()

	// iterate over entries
	proposedBlock := uint64(0)
	sorted := false
	var oldEntry types.Entry
	for ; iterator.Valid(); iterator.Next() {
		// umarshal the old entry version
		vs.GetCdc().MustUnmarshal(iterator.Value(), &oldEntry)

		// proposedBlock should always be smaller than the next entry's block if the elements in the store are sorted
		if proposedBlock < oldEntry.Block {
			sorted = true
		} else {
			sorted = false
		}
		proposedBlock = oldEntry.Block
	}
	require.True(t, sorted)

	// verify the last element is the latest entry
	require.Equal(t, oldEntry.Block, proposedBlock)
}

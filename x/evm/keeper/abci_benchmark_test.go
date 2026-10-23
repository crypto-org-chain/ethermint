package keeper_test

import (
	"math/big"
	"testing"

	"cosmossdk.io/log/v2"
	cmtproto "github.com/cometbft/cometbft/proto/tendermint/types"
	dbm "github.com/cosmos/cosmos-db"
	"github.com/cosmos/cosmos-sdk/store/v2/rootmulti"
	storetypes "github.com/cosmos/cosmos-sdk/store/v2/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/ethereum/go-ethereum/common"
	"github.com/stretchr/testify/require"

	"github.com/evmos/ethermint/x/evm/types"
)

// BenchmarkSweepZeroStorage measures one migration-size sweep batch plus its commit on a disk-backed IAVL store.
// A third of the visited slots are legacy zero slots; Cronos mainnet had 33.9%.
func BenchmarkSweepZeroStorage(b *testing.B) {
	const baseSlots = 100_000
	batch := types.MigrationStorageSweepBatch
	suite := StatedbBenchmarkTestSuite{}
	suite.SetupTest(b)
	k := suite.App.EvmKeeper
	storeKey := suite.App.GetKey(types.StoreKey)

	db, err := dbm.NewGoLevelDB("sweep", b.TempDir(), nil)
	require.NoError(b, err)
	b.Cleanup(func() { _ = db.Close() })
	cms := rootmulti.NewStore(db, log.NewNopLogger())
	cms.MountStoreWithDB(storeKey, storetypes.StoreTypeIAVL, nil)
	require.NoError(b, cms.LoadLatestVersion())
	newCtx := func(ms storetypes.MultiStore) sdk.Context {
		return sdk.NewContext(ms, cmtproto.Header{}, false, log.NewNopLogger())
	}

	zero := make([]byte, common.HashLength)
	value := common.BigToHash(big.NewInt(1)).Bytes()
	writeSlots := func(store storetypes.KVStore, addr common.Address, n uint64) {
		for j := uint64(0); j < n; j++ {
			v := value
			if j%3 == 0 {
				v = zero
			}
			store.Set(types.StateKey(addr, common.BigToHash(new(big.Int).SetUint64(j))), v)
		}
	}

	// params and unrelated storage at the highest address, so the tree has a realistic size
	cache := cms.CacheMultiStore()
	require.NoError(b, k.SetParams(newCtx(cache), types.DefaultParams()))
	writeSlots(cache.GetKVStore(storeKey), common.MaxAddress, baseSlots)
	cache.Write()
	cms.Commit()

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		b.StopTimer()
		// one extra slot keeps the batch mid-sweep instead of finishing it
		addr := common.BigToAddress(big.NewInt(int64(i + 1)))
		cache = cms.CacheMultiStore()
		store := cache.GetKVStore(storeKey)
		writeSlots(store, addr, batch+1)
		store.Set(types.KeyPrefixStorageSweep, types.AddressStoragePrefix(addr))
		cache.Write()
		cms.Commit()
		b.StartTimer()

		cache = cms.CacheMultiStore()
		k.SweepZeroStorageForTest(newCtx(cache), batch)
		cache.Write()
		cms.Commit()
	}
}

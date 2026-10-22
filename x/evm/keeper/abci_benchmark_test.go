package keeper_test

import (
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/evmos/ethermint/x/evm/types"
)

// BenchmarkSweepZeroStorage measures one default-size sweep batch where every visited slot is a legacy zero slot.
func BenchmarkSweepZeroStorage(b *testing.B) {
	batch := types.DefaultStorageSweepBatch
	suite := StatedbBenchmarkTestSuite{}
	suite.SetupTest(b)
	k := suite.App.EvmKeeper
	storeKey := suite.App.GetKey(types.StoreKey)
	zero := make([]byte, common.HashLength)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		b.StopTimer()
		// a fresh cache per run, so deletes from earlier runs don't slow iteration
		ctx, _ := suite.Ctx.CacheContext()
		store := ctx.KVStore(storeKey)
		for j := uint64(0); j < batch; j++ {
			store.Set(types.StateKey(suite.Address, common.BigToHash(new(big.Int).SetUint64(j))), zero)
		}
		store.Set(types.KeyPrefixStorageSweep, types.KeyPrefixStorage)
		b.StartTimer()

		k.SweepZeroStorage(ctx, batch)
	}
}

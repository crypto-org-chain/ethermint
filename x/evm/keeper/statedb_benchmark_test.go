package keeper_test

import (
	"math/big"
	"testing"

	"github.com/holiman/uint256"
	"github.com/stretchr/testify/require"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/tracing"
	ethtypes "github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"

	"github.com/evmos/ethermint/tests"
	"github.com/evmos/ethermint/testutil"
	"github.com/evmos/ethermint/x/evm/types"
)

type StatedbBenchmarkTestSuite struct {
	testutil.BaseTestSuiteWithAccount
}

func BenchmarkCreateAccountNew(b *testing.B) {
	suite := StatedbBenchmarkTestSuite{}
	suite.SetupTest(b)
	vmdb := suite.StateDB()

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		b.StopTimer()
		addr := tests.GenerateAddress()
		b.StartTimer()
		vmdb.CreateAccount(addr)
	}
}

func BenchmarkCreateAccountExisting(b *testing.B) {
	suite := StatedbBenchmarkTestSuite{}
	suite.SetupTest(b)
	vmdb := suite.StateDB()

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		vmdb.CreateAccount(suite.Address)
	}
}

func BenchmarkAddBalance(b *testing.B) {
	suite := StatedbBenchmarkTestSuite{}
	suite.SetupTest(b)
	vmdb := suite.StateDB()

	amt := uint256.NewInt(10)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		vmdb.AddBalance(suite.Address, amt, tracing.BalanceChangeTransfer)
	}
}

func BenchmarkSetCode(b *testing.B) {
	suite := StatedbBenchmarkTestSuite{}
	suite.SetupTest(b)
	vmdb := suite.StateDB()

	hash := crypto.Keccak256Hash([]byte("code")).Bytes()

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		vmdb.SetCode(suite.Address, hash, 0)
	}
}

func BenchmarkSetState(b *testing.B) {
	suite := StatedbBenchmarkTestSuite{}
	suite.SetupTest(b)
	vmdb := suite.StateDB()

	hash := crypto.Keccak256Hash([]byte("topic")).Bytes()

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		vmdb.SetCode(suite.Address, hash, 0)
	}
}

func BenchmarkAddLog(b *testing.B) {
	suite := StatedbBenchmarkTestSuite{}
	suite.SetupTest(b)
	vmdb := suite.StateDB()

	topic := crypto.Keccak256Hash([]byte("topic"))
	txHash := crypto.Keccak256Hash([]byte("tx_hash"))
	blockHash := crypto.Keccak256Hash([]byte("block_hash"))

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		vmdb.AddLog(&ethtypes.Log{
			Address:     suite.Address,
			Topics:      []common.Hash{topic},
			Data:        []byte("data"),
			BlockNumber: 1,
			TxHash:      txHash,
			TxIndex:     1,
			BlockHash:   blockHash,
			Index:       1,
			Removed:     false,
		})
	}
}

func BenchmarkSnapshot(b *testing.B) {
	suite := StatedbBenchmarkTestSuite{}
	suite.SetupTest(b)
	vmdb := suite.StateDB()

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		target := vmdb.Snapshot()
		require.Equal(b, i, target)
	}

	for i := b.N - 1; i >= 0; i-- {
		require.NotPanics(b, func() {
			vmdb.RevertToSnapshot(i)
		})
	}
}

func BenchmarkSubBalance(b *testing.B) {
	suite := StatedbBenchmarkTestSuite{}
	suite.SetupTest(b)
	vmdb := suite.StateDB()

	amt := uint256.NewInt(10)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		vmdb.SubBalance(suite.Address, amt, tracing.BalanceChangeTransfer)
	}
}

func BenchmarkSetNonce(b *testing.B) {
	suite := StatedbBenchmarkTestSuite{}
	suite.SetupTest(b)
	vmdb := suite.StateDB()

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		vmdb.SetNonce(suite.Address, 1, tracing.NonceChangeUnspecified)
	}
}

func BenchmarkAddRefund(b *testing.B) {
	suite := StatedbBenchmarkTestSuite{}
	suite.SetupTest(b)
	vmdb := suite.StateDB()

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		vmdb.AddRefund(1)
	}
}

func BenchmarkSuicide(b *testing.B) {
	suite := StatedbBenchmarkTestSuite{}
	suite.SetupTest(b)
	vmdb := suite.StateDB()

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		b.StopTimer()
		addr := tests.GenerateAddress()
		vmdb.CreateAccount(addr)
		b.StartTimer()

		vmdb.SelfDestruct(addr)
	}
}

func BenchmarkKeeperSetState(b *testing.B) {
	padded := common.BigToHash(big.NewInt(1)).Bytes()
	benchmarks := []struct {
		name     string
		compact  bool
		cached   bool // DeliverTx has the block config cached; queries and genesis may not
		resolved bool // SetStateCompact with the flag passed in, the develop-equivalent path without a lookup
		value    []byte
	}{
		{"legacy, no lookup", false, true, true, padded},
		{"legacy", false, true, false, padded},
		{"compact", true, true, false, padded},
		{"compact, params not cached", true, false, false, padded},
		{"compact, value already trimmed", true, true, false, []byte{1}},
	}
	for _, bm := range benchmarks {
		b.Run(bm.name, func(b *testing.B) {
			suite := StatedbBenchmarkTestSuite{}
			suite.SetupTest(b)
			k := suite.App.EvmKeeper
			suite.UpdateEVMParams(b, func(p *types.Params) { p.CompactStorage = bm.compact })
			if bm.cached {
				_, err := k.EVMBlockConfig(suite.Ctx, k.ChainID())
				require.NoError(b, err)
			}
			key := common.BytesToHash([]byte("key"))

			b.ResetTimer()
			b.ReportAllocs()

			for i := 0; i < b.N; i++ {
				if bm.resolved {
					k.SetStateCompact(suite.Ctx, suite.Address, key, bm.value, bm.compact)
				} else {
					k.SetState(suite.Ctx, suite.Address, key, bm.value)
				}
			}
		})
	}
}

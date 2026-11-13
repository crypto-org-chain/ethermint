package keeper_test

import (
	"bytes"
	"math/big"
	"slices"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/evmos/ethermint/testutil"
	evmtypes "github.com/evmos/ethermint/x/evm/types"
	"github.com/stretchr/testify/suite"
)

type ABCITestSuite struct {
	testutil.BaseTestSuite
}

func TestABCITestSuite(t *testing.T) {
	suite.Run(t, new(ABCITestSuite))
}

func (suite *ABCITestSuite) TestInitGenesis() {
	suite.SetupTest()
	em := suite.Ctx.EventManager()
	suite.Require().Equal(0, len(em.Events()))

	suite.Require().NoError(suite.App.EvmKeeper.EndBlock(suite.Ctx))

	// should emit 1 EventTypeBlockBloom event on EndBlock
	suite.Require().Equal(1, len(em.Events()))
	suite.Require().Equal(evmtypes.EventTypeBlockBloom, em.Events()[0].Type)
}

func (suite *ABCITestSuite) TestSweepZeroStorage() {
	// lowest addresses, so swept first
	addrA, addrB := common.BigToAddress(big.NewInt(1)), common.BigToAddress(big.NewInt(2))
	zeroA := evmtypes.StateKey(addrA, common.BigToHash(big.NewInt(1)))
	keptA := evmtypes.StateKey(addrA, common.BigToHash(big.NewInt(2)))
	zeroB := evmtypes.StateKey(addrB, common.BigToHash(big.NewInt(1)))
	keptB := evmtypes.StateKey(addrB, common.BigToHash(big.NewInt(2)))
	slots := []struct{ key, value []byte }{
		{zeroA, make([]byte, common.HashLength)},
		{keptA, common.BigToHash(big.NewInt(7)).Bytes()},
		{zeroB, make([]byte, common.HashLength)},
		{keptB, []byte{7}},
		// all zeros but outside the storage prefix
		{evmtypes.GetHeaderHashKey(1), make([]byte, common.HashLength)},
	}

	testCases := []struct {
		name       string
		cursor     []byte // nil: no sweep running
		beginBlock bool   // run the sweep through BeginBlock, with its fixed batch
		limit      uint64
		runs       int
		expDeleted [][]byte
		expCursor  []byte
	}{
		{
			name:  "no sweep running",
			limit: 10,
			runs:  1,
		},
		{
			name:       "one run visits exactly the limit",
			cursor:     evmtypes.KeyPrefixStorage,
			limit:      3,
			runs:       1,
			expDeleted: [][]byte{zeroA, zeroB},
			expCursor:  keptB,
		},
		{
			name:       "runs resume from the cursor",
			cursor:     evmtypes.KeyPrefixStorage,
			limit:      1,
			runs:       3,
			expDeleted: [][]byte{zeroA, zeroB},
			expCursor:  keptB,
		},
		{
			// a tx deleted the cursor slot between batches
			name:       "resumes when the cursor slot was deleted",
			cursor:     evmtypes.StateKey(addrA, common.BigToHash(big.NewInt(3))),
			limit:      1,
			runs:       1,
			expDeleted: [][]byte{zeroB},
			expCursor:  keptB,
		},
		{
			name:       "sweep finishes and clears the cursor",
			cursor:     evmtypes.KeyPrefixStorage,
			limit:      1000,
			runs:       1,
			expDeleted: [][]byte{zeroA, zeroB},
		},
		{
			name:       "begin block runs the sweep",
			cursor:     evmtypes.KeyPrefixStorage,
			beginBlock: true,
			runs:       1,
			expDeleted: [][]byte{zeroA, zeroB},
		},
	}

	for _, tc := range testCases {
		suite.Run(tc.name, func() {
			suite.SetupTest()
			store := suite.Ctx.KVStore(suite.App.GetKey(evmtypes.StoreKey))
			for _, s := range slots {
				store.Set(s.key, s.value)
			}
			if tc.cursor != nil {
				store.Set(evmtypes.KeyPrefixStorageSweep, tc.cursor)
			}

			for i := 0; i < tc.runs; i++ {
				if tc.beginBlock {
					suite.Require().NoError(suite.App.EvmKeeper.BeginBlock(suite.Ctx))
				} else {
					suite.App.EvmKeeper.SweepZeroStorageForTest(suite.Ctx, tc.limit)
				}
			}

			for _, s := range slots {
				deleted := slices.ContainsFunc(tc.expDeleted, func(k []byte) bool { return bytes.Equal(k, s.key) })
				if deleted {
					suite.Require().Nil(store.Get(s.key))
				} else {
					suite.Require().Equal(s.value, store.Get(s.key))
				}
			}
			suite.Require().Equal(tc.expCursor, store.Get(evmtypes.KeyPrefixStorageSweep))
		})
	}
}

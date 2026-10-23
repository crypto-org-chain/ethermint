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
		batch      uint64
		blocks     int
		expDeleted [][]byte
		expCursor  []byte
	}{
		{
			name:   "no sweep running",
			batch:  10,
			blocks: 1,
		},
		{
			name:       "one block visits exactly the batch",
			cursor:     evmtypes.KeyPrefixStorage,
			batch:      3,
			blocks:     1,
			expDeleted: [][]byte{zeroA, zeroB},
			expCursor:  keptB,
		},
		{
			name:       "batches resume from the cursor",
			cursor:     evmtypes.KeyPrefixStorage,
			batch:      1,
			blocks:     3,
			expDeleted: [][]byte{zeroA, zeroB},
			expCursor:  keptB,
		},
		{
			// a tx deleted the cursor slot between batches
			name:       "resumes when the cursor slot was deleted",
			cursor:     evmtypes.StateKey(addrA, common.BigToHash(big.NewInt(3))),
			batch:      1,
			blocks:     1,
			expDeleted: [][]byte{zeroB},
			expCursor:  keptB,
		},
		{
			name:       "sweep finishes and clears the cursor",
			cursor:     evmtypes.KeyPrefixStorage,
			batch:      1000,
			blocks:     1,
			expDeleted: [][]byte{zeroA, zeroB},
		},
	}

	for _, tc := range testCases {
		suite.Run(tc.name, func() {
			suite.SetupTest()
			suite.UpdateEVMParams(suite.T(), func(p *evmtypes.Params) { p.StorageSweepBatch = tc.batch })
			store := suite.Ctx.KVStore(suite.App.GetKey(evmtypes.StoreKey))
			for _, s := range slots {
				store.Set(s.key, s.value)
			}
			if tc.cursor != nil {
				store.Set(evmtypes.KeyPrefixStorageSweep, tc.cursor)
			}

			for i := 0; i < tc.blocks; i++ {
				suite.Require().NoError(suite.App.EvmKeeper.BeginBlock(suite.Ctx))
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
			expBatch := tc.batch
			if tc.cursor != nil && tc.expCursor == nil {
				expBatch = 0 // a finished sweep resets the batch
			}
			suite.Require().Equal(expBatch, suite.App.EvmKeeper.GetParams(suite.Ctx).StorageSweepBatch)
		})
	}
}

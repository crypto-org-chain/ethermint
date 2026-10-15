package evm_test

import (
	"math/big"
	"slices"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/tracing"
	"github.com/holiman/uint256"

	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"
	"github.com/evmos/ethermint/crypto/ethsecp256k1"
	"github.com/evmos/ethermint/testutil"
	ethermint "github.com/evmos/ethermint/types"
	"github.com/evmos/ethermint/x/evm"
	"github.com/evmos/ethermint/x/evm/statedb"
	"github.com/evmos/ethermint/x/evm/types"
	"github.com/stretchr/testify/suite"
)

type GenesisTestSuite struct {
	testutil.BaseTestSuite
}

func TestGenesisTestSuite(t *testing.T) {
	suite.Run(t, new(GenesisTestSuite))
}

func (suite *GenesisTestSuite) TestInitGenesis() {
	privkey, err := ethsecp256k1.GenerateKey()
	suite.Require().NoError(err)

	address := common.HexToAddress(privkey.PubKey().Address().String())

	var vmdb *statedb.StateDB

	storage := types.Storage{
		{Key: common.BytesToHash([]byte("key")).String(), Value: common.BytesToHash([]byte("value")).String()},
		{Key: common.BytesToHash([]byte("zero")).String(), Value: common.Hash{}.String()},
	}

	testCases := []struct {
		name     string
		malleate func()
		genState *types.GenesisState
		expPanic bool
	}{
		{
			"default",
			func() {},
			types.DefaultGenesisState(),
			false,
		},
		{
			"valid account",
			func() {
				vmdb.AddBalance(address, uint256.NewInt(1), tracing.BalanceChangeTransfer)
			},
			&types.GenesisState{
				Params:         types.DefaultParams(),
				Accounts:       []types.GenesisAccount{{Address: address.String(), Storage: storage}},
				CompactStorage: true,
			},
			false,
		},
		{
			"valid account - legacy storage format",
			func() {
				vmdb.AddBalance(address, uint256.NewInt(1), tracing.BalanceChangeTransfer)
			},
			&types.GenesisState{
				Params:   types.DefaultParams(),
				Accounts: []types.GenesisAccount{{Address: address.String(), Storage: storage}},
			},
			false,
		},
		{
			"account not found",
			func() {},
			&types.GenesisState{
				Params: types.DefaultParams(),
				Accounts: []types.GenesisAccount{
					{
						Address: address.String(),
					},
				},
			},
			true,
		},
		{
			"invalid account type",
			func() {
				acc := authtypes.NewBaseAccountWithAddress(address.Bytes())
				acc.AccountNumber = suite.App.AccountKeeper.NextAccountNumber(suite.Ctx, acc)
				suite.App.AccountKeeper.SetAccount(suite.Ctx, acc)
			},
			&types.GenesisState{
				Params: types.DefaultParams(),
				Accounts: []types.GenesisAccount{
					{
						Address: address.String(),
					},
				},
			},
			true,
		},
		{
			"invalid code hash",
			func() {
				acc := suite.App.AccountKeeper.NewAccountWithAddress(suite.Ctx, address.Bytes())
				suite.App.AccountKeeper.SetAccount(suite.Ctx, acc)
			},
			&types.GenesisState{
				Params: types.DefaultParams(),
				Accounts: []types.GenesisAccount{
					{
						Address: address.String(),
						Code:    "ffffffff",
					},
				},
			},
			true,
		},
		{
			"ignore empty account code checking",
			func() {
				acc := suite.App.AccountKeeper.NewAccountWithAddress(suite.Ctx, address.Bytes())

				suite.App.AccountKeeper.SetAccount(suite.Ctx, acc)
			},
			&types.GenesisState{
				Params: types.DefaultParams(),
				Accounts: []types.GenesisAccount{
					{
						Address: address.String(),
						Code:    "",
					},
				},
			},
			false,
		},
		{
			"ignore empty account code checking with non-empty codehash",
			func() {
				ethAcc := &ethermint.EthAccount{
					BaseAccount: authtypes.NewBaseAccount(address.Bytes(), nil, 0, 0),
					CodeHash:    common.BytesToHash([]byte{1, 2, 3}).Hex(),
				}
				ethAcc.AccountNumber = suite.App.AccountKeeper.NextAccountNumber(suite.Ctx, ethAcc)

				suite.App.AccountKeeper.SetAccount(suite.Ctx, ethAcc)
			},
			&types.GenesisState{
				Params: types.DefaultParams(),
				Accounts: []types.GenesisAccount{
					{
						Address: address.String(),
						Code:    "",
					},
				},
			},
			false,
		},
	}

	for _, tc := range testCases {
		suite.Run(tc.name, func() {
			suite.SetupTest()
			vmdb = suite.StateDB()

			tc.malleate()
			vmdb.Commit()

			// app setup already ran InitGenesis; clear the switch so each case starts from a fresh store
			store := suite.Ctx.KVStore(suite.App.GetKey(types.StoreKey))
			store.Delete(types.KeyPrefixCompactStorage)

			if tc.expPanic {
				suite.Require().Panics(
					func() {
						_ = evm.InitGenesis(suite.Ctx, suite.App.EvmKeeper, suite.App.AccountKeeper, *tc.genState)
					},
				)
			} else {
				suite.Require().NotPanics(
					func() {
						_ = evm.InitGenesis(suite.Ctx, suite.App.EvmKeeper, suite.App.AccountKeeper, *tc.genState)
					},
				)

				suite.Require().Equal(tc.genState.CompactStorage, store.Has(types.KeyPrefixCompactStorage))
				for _, acc := range tc.genState.Accounts {
					for _, s := range acc.Storage {
						raw := store.Get(types.StateKey(common.HexToAddress(acc.Address), common.HexToHash(s.Key)))
						value := common.HexToHash(s.Value)
						suite.Require().Equal(value, common.BytesToHash(raw))
						expLen := common.HashLength
						if tc.genState.CompactStorage {
							expLen = len(common.TrimLeftZeroes(value.Bytes()))
						}
						suite.Require().Len(raw, expLen)
					}
				}

				exported := evm.ExportGenesis(suite.Ctx, suite.App.EvmKeeper, suite.App.AccountKeeper)
				suite.Require().Equal(tc.genState.CompactStorage, exported.CompactStorage)
			}
		})
	}
}

func (suite *GenesisTestSuite) TestExportImportMidSweep() {
	suite.SetupTest()
	address := common.BigToAddress(big.NewInt(1))
	vmdb := suite.StateDB()
	vmdb.AddBalance(address, uint256.NewInt(1), tracing.BalanceChangeTransfer)
	suite.Require().NoError(vmdb.Commit())

	// legacy slots the sweep has not reached yet
	store := suite.Ctx.KVStore(suite.App.GetKey(types.StoreKey))
	zeroKey, keptKey := common.BigToHash(big.NewInt(1)), common.BigToHash(big.NewInt(2))
	keptValue := common.BigToHash(big.NewInt(7))
	store.Set(types.StateKey(address, zeroKey), make([]byte, common.HashLength))
	store.Set(types.StateKey(address, keptKey), keptValue.Bytes())
	store.Set(types.KeyPrefixStorageSweep, types.KeyPrefixStorage)

	exported := evm.ExportGenesis(suite.Ctx, suite.App.EvmKeeper, suite.App.AccountKeeper)
	suite.Require().True(exported.CompactStorage)
	// the sweep cursor isn't exported, so the zero slot travels in the export
	idx := slices.IndexFunc(exported.Accounts, func(a types.GenesisAccount) bool { return a.Address == address.String() })
	suite.Require().GreaterOrEqual(idx, 0)
	suite.Require().Contains(exported.Accounts[idx].Storage, types.State{Key: zeroKey.String(), Value: common.Hash{}.String()})

	// import into a fresh store: neither the switch nor the cursor carries over except through the export
	store.Delete(types.KeyPrefixCompactStorage)
	store.Delete(types.KeyPrefixStorageSweep)
	_ = evm.InitGenesis(suite.Ctx, suite.App.EvmKeeper, suite.App.AccountKeeper, *exported)
	suite.Require().True(store.Has(types.KeyPrefixCompactStorage))
	suite.Require().False(store.Has(types.KeyPrefixStorageSweep))
	suite.Require().Nil(store.Get(types.StateKey(address, zeroKey)))
	suite.Require().Equal([]byte{7}, store.Get(types.StateKey(address, keptKey)))
}

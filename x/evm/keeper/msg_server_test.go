package keeper_test

import (
	"math/big"
	"testing"

	storetypes "github.com/cosmos/cosmos-sdk/store/v2/types"
	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"
	govtypes "github.com/cosmos/cosmos-sdk/x/gov/types"
	"github.com/stretchr/testify/suite"

	ethtypes "github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/params"
	"github.com/evmos/ethermint/testutil"
	utiltx "github.com/evmos/ethermint/testutil/tx"
	"github.com/evmos/ethermint/x/evm/statedb"
	"github.com/evmos/ethermint/x/evm/types"
)

type MsgServerTestSuite struct {
	testutil.BaseTestSuiteWithAccount
}

func TestMsgServerTestSuite(t *testing.T) {
	suite.Run(t, new(MsgServerTestSuite))
}

func (suite *MsgServerTestSuite) TestEthereumTx() {
	var (
		err             error
		msg             *types.MsgEthereumTx
		signer          ethtypes.Signer
		vmdb            *statedb.StateDB
		expectedGasUsed uint64
	)

	testCases := []struct {
		name     string
		malleate func()
		expErr   bool
	}{
		{
			"Deploy contract tx - insufficient gas",
			func() {
				msg, err = utiltx.CreateContractMsgTx(
					vmdb.GetNonce(suite.Address),
					signer,
					big.NewInt(1),
					suite.Address,
					suite.Signer,
				)
				suite.Require().NoError(err)
			},
			true,
		},
		{
			"Transfer funds tx",
			func() {
				msg, _, err = newEthMsgTx(
					vmdb.GetNonce(suite.Address),
					suite.Address,
					suite.Signer,
					signer,
					ethtypes.AccessListTxType,
					nil,
					nil,
				)
				suite.Require().NoError(err)
				expectedGasUsed = params.TxGas
			},
			false,
		},
	}

	for _, tc := range testCases {
		suite.Run(tc.name, func() {
			suite.SetupTest(suite.T())
			signer = ethtypes.LatestSignerForChainID(suite.App.EvmKeeper.ChainID())
			vmdb = suite.StateDB()

			tc.malleate()
			res, err := suite.App.EvmKeeper.EthereumTx(suite.Ctx, msg)
			if tc.expErr {
				suite.Require().Error(err)
				return
			}
			suite.Require().NoError(err)
			suite.Require().Equal(expectedGasUsed, res.GasUsed)
			suite.Require().False(res.Failed())
		})
	}
}

func (suite *MsgServerTestSuite) TestUpdateParams() {
	testCases := []struct {
		name      string
		request   *types.MsgUpdateParams
		expectErr bool
	}{
		{
			name:      "fail - invalid authority",
			request:   &types.MsgUpdateParams{Authority: "foobar"},
			expectErr: true,
		},
		{
			name: "pass - valid Update msg",
			request: &types.MsgUpdateParams{
				Authority: authtypes.NewModuleAddress(govtypes.ModuleName).String(),
				Params:    types.DefaultParams(),
			},
			expectErr: false,
		},
		{
			name: "fail - storage sweep batch above the maximum",
			request: &types.MsgUpdateParams{
				Authority: authtypes.NewModuleAddress(govtypes.ModuleName).String(),
				Params: func() types.Params {
					p := types.DefaultParams()
					p.StorageSweepBatch = types.MaxStorageSweepBatch + 1
					return p
				}(),
			},
			expectErr: true,
		},
		{
			name: "fail - turning compact storage off",
			request: &types.MsgUpdateParams{
				Authority: authtypes.NewModuleAddress(govtypes.ModuleName).String(),
				Params: func() types.Params {
					p := types.DefaultParams()
					p.CompactStorage = false
					return p
				}(),
			},
			expectErr: true,
		},
	}

	for _, tc := range testCases {
		suite.Run("MsgUpdateParams", func() {
			suite.SetupTest(suite.T())
			_, err := suite.App.EvmKeeper.UpdateParams(suite.Ctx, tc.request)
			if tc.expectErr {
				suite.Require().Error(err)
			} else {
				suite.Require().NoError(err)
			}
		})
	}
}

func (suite *MsgServerTestSuite) TestUpdateParamsKeepsCompactStorageOnWithinBlock() {
	suite.SetupTest(suite.T())
	k := suite.App.EvmKeeper
	authority := authtypes.NewModuleAddress(govtypes.ModuleName).String()
	params := k.GetParams(suite.Ctx)
	params.CompactStorage = false
	suite.Require().NoError(k.SetParams(suite.Ctx, params))
	k.RemoveParamsCache(suite.Ctx)
	// BeginBlock caches the compact-off params for the whole block
	_, err := k.EVMBlockConfig(suite.Ctx, k.ChainID())
	suite.Require().NoError(err)

	params.CompactStorage = true
	_, err = k.UpdateParams(suite.Ctx, &types.MsgUpdateParams{Authority: authority, Params: params})
	suite.Require().NoError(err)
	params.CompactStorage = false
	_, err = k.UpdateParams(suite.Ctx, &types.MsgUpdateParams{Authority: authority, Params: params})
	suite.Require().Error(err)
}

func (suite *MsgServerTestSuite) TestUpdateParamsSweepBatch() {
	testCases := []struct {
		name      string
		sweeping  bool
		batch     uint64
		expectErr bool
	}{
		{name: "zero batch without a sweep", batch: 0},
		{name: "zero batch while a sweep runs", sweeping: true, batch: 0, expectErr: true},
		{name: "lower batch while a sweep runs", sweeping: true, batch: 1},
	}

	for _, tc := range testCases {
		suite.Run(tc.name, func() {
			suite.SetupTest(suite.T())
			k := suite.App.EvmKeeper
			if tc.sweeping {
				suite.Ctx.KVStore(suite.App.GetKey(types.StoreKey)).Set(types.KeyPrefixStorageSweep, types.KeyPrefixStorage)
			}
			params := k.GetParams(suite.Ctx)
			params.StorageSweepBatch = tc.batch
			_, err := k.UpdateParams(suite.Ctx, &types.MsgUpdateParams{
				Authority: authtypes.NewModuleAddress(govtypes.ModuleName).String(),
				Params:    params,
			})
			if tc.expectErr {
				suite.Require().Error(err)
			} else {
				suite.Require().NoError(err)
			}
		})
	}
}

func (suite *MsgServerTestSuite) TestUpdateParamsGas() {
	suite.SetupTest(suite.T())
	k := suite.App.EvmKeeper
	// a running sweep makes both guards read the store
	suite.Ctx.KVStore(suite.App.GetKey(types.StoreKey)).Set(types.KeyPrefixStorageSweep, types.KeyPrefixStorage)
	params := k.GetParams(suite.Ctx)
	params.StorageSweepBatch = types.DefaultStorageSweepBatch

	ctx := suite.Ctx.WithGasMeter(storetypes.NewInfiniteGasMeter())
	_, err := k.UpdateParams(ctx, &types.MsgUpdateParams{
		Authority: authtypes.NewModuleAddress(govtypes.ModuleName).String(),
		Params:    params,
	})
	suite.Require().NoError(err)

	// the guards read without gas, so only the params write is charged
	expCtx := suite.Ctx.WithGasMeter(storetypes.NewInfiniteGasMeter())
	suite.Require().NoError(k.SetParams(expCtx, params))
	suite.Require().Equal(expCtx.GasMeter().GasConsumed(), ctx.GasMeter().GasConsumed())
}

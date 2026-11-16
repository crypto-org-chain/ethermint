package keeper_test

import (
	"reflect"
	"testing"

	sdkmath "cosmossdk.io/math"
	"github.com/cosmos/cosmos-sdk/codec"
	storetypes "github.com/cosmos/cosmos-sdk/store/v2/types"
	sdktestutil "github.com/cosmos/cosmos-sdk/testutil"
	sdk "github.com/cosmos/cosmos-sdk/types"
	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"
	govtypes "github.com/cosmos/cosmos-sdk/x/gov/types"
	"github.com/evmos/ethermint/testutil"
	"github.com/evmos/ethermint/types/legacyparams"
	"github.com/evmos/ethermint/x/evm/keeper"
	v0types "github.com/evmos/ethermint/x/evm/migrations/v0/types"
	v4types "github.com/evmos/ethermint/x/evm/migrations/v4/types"
	"github.com/evmos/ethermint/x/evm/types"
	"github.com/stretchr/testify/suite"
)

type ParamsTestSuite struct {
	testutil.BaseTestSuite
}

func TestParamsTestSuite(t *testing.T) {
	suite.Run(t, new(ParamsTestSuite))
}

func (suite *ParamsTestSuite) TestParams() {
	suite.SetupTest()
	params := suite.App.EvmKeeper.GetParams(suite.Ctx)
	suite.App.EvmKeeper.SetParams(suite.Ctx, params)
	testCases := []struct {
		name      string
		paramsFun func() interface{}
		getFun    func() interface{}
		expected  bool
	}{
		{
			"success - Checks if the default params are set correctly",
			func() interface{} {
				return types.DefaultParams()
			},
			func() interface{} {
				return suite.App.EvmKeeper.GetParams(suite.Ctx)
			},
			true,
		},
		{
			"success - EvmDenom param is set to \"inj\" and can be retrieved correctly",
			func() interface{} {
				params.EvmDenom = "inj"
				suite.App.EvmKeeper.SetParams(suite.Ctx, params)
				return params.EvmDenom
			},
			func() interface{} {
				evmParams := suite.App.EvmKeeper.GetParams(suite.Ctx)
				return evmParams.GetEvmDenom()
			},
			true,
		},
		{
			"success - Check EnableCreate param is set to false and can be retrieved correctly",
			func() interface{} {
				params.EnableCreate = false
				suite.App.EvmKeeper.SetParams(suite.Ctx, params)
				return params.EnableCreate
			},
			func() interface{} {
				evmParams := suite.App.EvmKeeper.GetParams(suite.Ctx)
				return evmParams.GetEnableCreate()
			},
			true,
		},
		{
			"success - Check EnableCall param is set to false and can be retrieved correctly",
			func() interface{} {
				params.EnableCall = false
				suite.App.EvmKeeper.SetParams(suite.Ctx, params)
				return params.EnableCall
			},
			func() interface{} {
				evmParams := suite.App.EvmKeeper.GetParams(suite.Ctx)
				return evmParams.GetEnableCall()
			},
			true,
		},
		{
			"success - Check AllowUnprotectedTxs param is set to false and can be retrieved correctly",
			func() interface{} {
				params.AllowUnprotectedTxs = false
				suite.App.EvmKeeper.SetParams(suite.Ctx, params)
				return params.AllowUnprotectedTxs
			},
			func() interface{} {
				evmParams := suite.App.EvmKeeper.GetParams(suite.Ctx)
				return evmParams.GetAllowUnprotectedTxs()
			},
			true,
		},
		{
			"success - Check ChainConfig param is set to the default value and can be retrieved correctly",
			func() interface{} {
				params.ChainConfig = types.DefaultChainConfig()
				suite.App.EvmKeeper.SetParams(suite.Ctx, params)
				return params.ChainConfig
			},
			func() interface{} {
				evmParams := suite.App.EvmKeeper.GetParams(suite.Ctx)
				return evmParams.GetChainConfig()
			},
			true,
		},
		{
			"success - Check GetParams returns zero-value params when none are stored",
			func() interface{} {
				suite.Ctx.KVStore(suite.App.GetKey(types.StoreKey)).Delete(types.KeyPrefixParams)
				return types.Params{}
			},
			func() interface{} {
				return suite.App.EvmKeeper.GetParams(suite.Ctx)
			},
			true,
		},
	}
	for _, tc := range testCases {
		suite.Run(tc.name, func() {
			outcome := reflect.DeepEqual(tc.paramsFun(), tc.getFun())
			suite.Require().Equal(tc.expected, outcome)
		})
	}
}

func (suite *ParamsTestSuite) TestGetParamsHistoricalLayouts() {
	cdc := suite.App.AppCodec()
	amino := codec.NewLegacyAmino()
	evmKey := storetypes.NewKVStoreKey(types.StoreKey)
	paramsKey := storetypes.NewKVStoreKey("params")

	one := sdkmath.OneInt()
	v4Params := v4types.V4Params{
		EvmDenom:            "basetcro",
		EnableCreate:        true,
		AllowUnprotectedTxs: true,
		ExtraEIPs:           v4types.ExtraEIPs{EIPs: []int64{2929}},
		ChainConfig: v0types.V0ChainConfig{
			HomesteadBlock: &one,
			LondonBlock:    &one,
			EIP150Hash:     "0x0000000000000000000000000000000000000000000000000000000000000000",
		},
	}
	currentParams := types.DefaultParams()
	setCurrent := func(ctx sdk.Context) {
		ctx.KVStore(evmKey).Set(types.KeyPrefixParams, cdc.MustMarshal(&currentParams))
	}
	setV4Keys := func(ctx sdk.Context) {
		store := ctx.KVStore(evmKey)
		store.Set(v0types.ParamStoreKeyChainConfig, cdc.MustMarshal(&v4Params.ChainConfig))
		store.Set(v0types.ParamStoreKeyExtraEIPs, cdc.MustMarshal(&v4Params.ExtraEIPs))
		store.Set(v0types.ParamStoreKeyEVMDenom, []byte(v4Params.EvmDenom))
		// v4 stored bools as key presence: EnableCall is false, so its key is absent
		store.Set(v0types.ParamStoreKeyEnableCreate, []byte{0x01})
		store.Set(v0types.ParamStoreKeyAllowUnprotectedTxs, []byte{0x01})
	}
	setLegacy := func(ctx sdk.Context) {
		store := ctx.KVStore(paramsKey)
		for key, value := range map[string]any{
			string(v0types.ParamStoreKeyEVMDenom):            v4Params.EvmDenom,
			string(v0types.ParamStoreKeyEnableCreate):        v4Params.EnableCreate,
			string(v0types.ParamStoreKeyEnableCall):          v4Params.EnableCall,
			string(v0types.ParamStoreKeyChainConfig):         v4Params.ChainConfig,
			string(v0types.ParamStoreKeyAllowUnprotectedTxs): v4Params.AllowUnprotectedTxs,
		} {
			store.Set([]byte("evm/"+key), amino.MustMarshalJSON(value))
		}
		// x/params amino JSON encodes int64 as strings
		store.Set([]byte("evm/"+string(v0types.ParamStoreKeyExtraEIPs)), []byte(`["2929"]`))
	}
	// x/params still held EnableCall=true after a v4-era proposal turned it off
	setLegacyEnableCall := func(ctx sdk.Context) {
		ctx.KVStore(paramsKey).Set([]byte("evm/"+string(v0types.ParamStoreKeyEnableCall)), amino.MustMarshalJSON(true))
	}
	v4WithLegacyEnableCall := v4Params.ToParams()
	v4WithLegacyEnableCall.EnableCall = true

	testCases := []struct {
		name         string
		legacyParams legacyparams.Subspace
		malleate     []func(sdk.Context)
		expected     types.Params
	}{
		{
			"current key wins over older layouts",
			legacyparams.NewSubspace(paramsKey, types.ModuleName),
			[]func(sdk.Context){setCurrent, setV4Keys, setLegacy},
			currentParams,
		},
		{
			"v4 separate keys win over the legacy subspace",
			legacyparams.NewSubspace(paramsKey, types.ModuleName),
			[]func(sdk.Context){setV4Keys, setLegacy},
			v4Params.ToParams(),
		},
		{
			"v4 keys absent fall back to the legacy subspace per key",
			legacyparams.NewSubspace(paramsKey, types.ModuleName),
			[]func(sdk.Context){setV4Keys, setLegacy, setLegacyEnableCall},
			v4WithLegacyEnableCall,
		},
		{
			"v4 keys absent read as false without a legacy subspace",
			legacyparams.Subspace{},
			[]func(sdk.Context){setV4Keys, setLegacy, setLegacyEnableCall},
			v4Params.ToParams(),
		},
		{
			"legacy x/params subspace",
			legacyparams.NewSubspace(paramsKey, types.ModuleName),
			[]func(sdk.Context){setLegacy},
			v4Params.ToParams(),
		},
		{
			"legacy data ignored without a legacy subspace",
			legacyparams.Subspace{},
			[]func(sdk.Context){setLegacy},
			types.Params{},
		},
		{
			"nothing stored",
			legacyparams.NewSubspace(paramsKey, types.ModuleName),
			nil,
			types.Params{},
		},
	}
	for _, tc := range testCases {
		suite.Run(tc.name, func() {
			ctx := sdktestutil.DefaultContextWithKeys(
				map[string]*storetypes.KVStoreKey{evmKey.Name(): evmKey, paramsKey.Name(): paramsKey}, nil, nil,
			)
			k := keeper.NewKeeper(
				cdc, evmKey, nil, authtypes.NewModuleAddress(govtypes.ModuleName),
				suite.App.AccountKeeper, nil, nil, nil, "", tc.legacyParams, nil, keeper.GasNoLimit,
			)
			for _, set := range tc.malleate {
				set(ctx)
			}
			suite.Require().Equal(tc.expected, k.GetParams(ctx))
		})
	}
}

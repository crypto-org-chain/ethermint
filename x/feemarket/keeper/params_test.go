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
	"github.com/evmos/ethermint/x/feemarket/keeper"
	"github.com/evmos/ethermint/x/feemarket/types"
	"github.com/stretchr/testify/suite"
)

type ParamsTestSuite struct {
	testutil.BaseTestSuite
}

func TestParamsTestSuite(t *testing.T) {
	suite.Run(t, new(ParamsTestSuite))
}

func (suite *ParamsTestSuite) TestSetGetParams() {
	params := types.DefaultParams()
	suite.App.FeeMarketKeeper.SetParams(suite.Ctx, params)
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
				return suite.App.FeeMarketKeeper.GetParams(suite.Ctx)
			},
			true,
		},
		{
			"success - Check ElasticityMultiplier is set to 3 and can be retrieved correctly",
			func() interface{} {
				params.ElasticityMultiplier = 3
				suite.App.FeeMarketKeeper.SetParams(suite.Ctx, params)
				return params.ElasticityMultiplier
			},
			func() interface{} {
				return suite.App.FeeMarketKeeper.GetParams(suite.Ctx).ElasticityMultiplier
			},
			true,
		},
		{
			"success - Check BaseFeeEnabled is computed with its default params and can be retrieved correctly",
			func() interface{} {
				suite.App.FeeMarketKeeper.SetParams(suite.Ctx, types.DefaultParams())
				return true
			},
			func() interface{} {
				return suite.App.FeeMarketKeeper.GetParams(suite.Ctx).IsBaseFeeEnabled(suite.Ctx.BlockHeight())
			},
			true,
		},
		{
			"success - Check BaseFeeEnabled is computed with alternate params and can be retrieved correctly",
			func() interface{} {
				params.NoBaseFee = true
				params.EnableHeight = 5
				suite.App.FeeMarketKeeper.SetParams(suite.Ctx, params)
				return true
			},
			func() interface{} {
				return suite.App.FeeMarketKeeper.GetParams(suite.Ctx).IsBaseFeeEnabled(suite.Ctx.BlockHeight())
			},
			false,
		},
		{
			"success - Check GetParams returns zero-value params when none are stored",
			func() interface{} {
				suite.Ctx.KVStore(suite.App.GetKey(types.StoreKey)).Delete(types.ParamsKey)
				return types.Params{}
			},
			func() interface{} {
				return suite.App.FeeMarketKeeper.GetParams(suite.Ctx)
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

func (suite *ParamsTestSuite) TestGetParamsLegacySubspace() {
	cdc := suite.App.AppCodec()
	amino := codec.NewLegacyAmino()
	feemarketKey := storetypes.NewKVStoreKey(types.StoreKey)
	paramsKey := storetypes.NewKVStoreKey("params")

	legacyParams := types.Params{
		BaseFeeChangeDenominator: 8,
		ElasticityMultiplier:     2,
		BaseFee:                  sdkmath.NewInt(5_000_000_000_000),
		EnableHeight:             3,
		MinGasPrice:              sdkmath.LegacyNewDec(1_000),
		MinGasMultiplier:         sdkmath.LegacyNewDecWithPrec(5, 1),
	}
	currentParams := types.DefaultParams()
	setCurrent := func(ctx sdk.Context) {
		ctx.KVStore(feemarketKey).Set(types.ParamsKey, cdc.MustMarshal(&currentParams))
	}
	setLegacy := func(ctx sdk.Context) {
		store := ctx.KVStore(paramsKey)
		for key, value := range map[string]any{
			"NoBaseFee":                legacyParams.NoBaseFee,
			"BaseFeeChangeDenominator": legacyParams.BaseFeeChangeDenominator,
			"ElasticityMultiplier":     legacyParams.ElasticityMultiplier,
			"BaseFee":                  legacyParams.BaseFee,
			"MinGasPrice":              legacyParams.MinGasPrice,
			"MinGasMultiplier":         legacyParams.MinGasMultiplier,
		} {
			store.Set([]byte("feemarket/"+key), amino.MustMarshalJSON(value))
		}
		// x/params amino JSON encodes int64 as strings
		store.Set([]byte("feemarket/EnableHeight"), []byte(`"3"`))
	}

	testCases := []struct {
		name         string
		legacyParams legacyparams.Subspace
		malleate     []func(sdk.Context)
		expected     types.Params
	}{
		{
			"current key wins over the legacy subspace",
			legacyparams.NewSubspace(paramsKey, types.ModuleName),
			[]func(sdk.Context){setCurrent, setLegacy},
			currentParams,
		},
		{
			"legacy x/params subspace",
			legacyparams.NewSubspace(paramsKey, types.ModuleName),
			[]func(sdk.Context){setLegacy},
			legacyParams,
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
				map[string]*storetypes.KVStoreKey{feemarketKey.Name(): feemarketKey, paramsKey.Name(): paramsKey}, nil, nil,
			)
			k := keeper.NewKeeper(cdc, authtypes.NewModuleAddress(govtypes.ModuleName), feemarketKey, tc.legacyParams)
			for _, set := range tc.malleate {
				set(ctx)
			}
			expected, got := tc.expected, k.GetParams(ctx)
			suite.Require().Equal(cdc.MustMarshal(&expected), cdc.MustMarshal(&got))
		})
	}
}

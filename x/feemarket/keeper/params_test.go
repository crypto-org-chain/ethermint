package keeper_test

import (
	"reflect"
	"testing"

	"github.com/evmos/ethermint/store/cachemulti"
	"github.com/evmos/ethermint/testutil"
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
	params := suite.App.FeeMarketKeeper.GetParams(suite.Ctx)
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
				return suite.App.FeeMarketKeeper.GetBaseFeeEnabled(suite.Ctx)
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
				return suite.App.FeeMarketKeeper.GetBaseFeeEnabled(suite.Ctx)
			},
			false,
		},
		{
			"success - Recover from panic in GetParams",
			func() interface{} {
				return types.DefaultParams()
			},
			func() interface{} {
				ctx := suite.Ctx.WithMultiStore(cachemulti.NewFromKVStore(nil, nil, nil))
				return suite.App.FeeMarketKeeper.GetParams(ctx)
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

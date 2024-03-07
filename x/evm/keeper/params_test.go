package keeper_test

import (
	"reflect"
	"testing"

	"github.com/evmos/ethermint/testutil"
	"github.com/evmos/ethermint/x/evm/types"
	"github.com/stretchr/testify/suite"
)

type ParamsTestSuite struct {
	testutil.EVMTestSuite
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
	}
	for _, tc := range testCases {
		suite.Run(tc.name, func() {
			outcome := reflect.DeepEqual(tc.paramsFun(), tc.getFun())
			suite.Require().Equal(tc.expected, outcome)
		})
	}
}

package keeper_test

import (
	"testing"

	sdkmath "cosmossdk.io/math"
	ethparams "github.com/ethereum/go-ethereum/params"
	"github.com/evmos/ethermint/testutil"
	"github.com/evmos/ethermint/x/feemarket/types"
	"github.com/stretchr/testify/suite"
)

type GRPCServerTestSuite struct {
	testutil.BaseTestSuiteWithFeeMarketQueryClient
}

func TestEGRPCServerTestSuite(t *testing.T) {
	suite.Run(t, new(EIP1559TestSuite))
}

func (suite *GRPCServerTestSuite) TestQueryParams() {
	testCases := []struct {
		name    string
		expPass bool
	}{
		{
			"pass",
			true,
		},
	}
	for _, tc := range testCases {
		params := suite.App.FeeMarketKeeper.GetParams(suite.Ctx)
		exp := &types.QueryParamsResponse{Params: params}

		res, err := suite.FeeMarketQueryClient.Params(suite.Ctx.Context(), &types.QueryParamsRequest{})
		if tc.expPass {
			suite.Require().Equal(exp, res, tc.name)
			suite.Require().NoError(err)
		} else {
			suite.Require().Error(err)
		}
	}
}

func (suite *GRPCServerTestSuite) TestQueryBaseFee() {
	var (
		aux    sdkmath.Int
		expRes *types.QueryBaseFeeResponse
	)

	testCases := []struct {
		name     string
		malleate func()
		expPass  bool
	}{
		{
			"pass - default Base Fee",
			func() {
				initialBaseFee := sdkmath.NewInt(ethparams.InitialBaseFee)
				expRes = &types.QueryBaseFeeResponse{BaseFee: &initialBaseFee}
			},
			true,
		},
		{
			"pass - non-nil Base Fee",
			func() {
				baseFee := sdkmath.OneInt().BigInt()
				suite.App.FeeMarketKeeper.SetBaseFee(suite.Ctx, baseFee)

				aux = sdkmath.NewIntFromBigInt(baseFee)
				expRes = &types.QueryBaseFeeResponse{BaseFee: &aux}
			},
			true,
		},
	}
	for _, tc := range testCases {
		tc.malleate()

		res, err := suite.FeeMarketQueryClient.BaseFee(suite.Ctx.Context(), &types.QueryBaseFeeRequest{})
		if tc.expPass {
			suite.Require().NotNil(res)
			suite.Require().Equal(expRes, res, tc.name)
			suite.Require().NoError(err)
		} else {
			suite.Require().Error(err)
		}
	}
}

func (suite *GRPCServerTestSuite) TestQueryBlockGas() {
	testCases := []struct {
		name    string
		expPass bool
	}{
		{
			"pass",
			true,
		},
	}
	for _, tc := range testCases {
		gas := suite.App.FeeMarketKeeper.GetBlockGasWanted(suite.Ctx)
		exp := &types.QueryBlockGasResponse{Gas: int64(gas)}

		res, err := suite.FeeMarketQueryClient.BlockGas(suite.Ctx.Context(), &types.QueryBlockGasRequest{})
		if tc.expPass {
			suite.Require().Equal(exp, res, tc.name)
			suite.Require().NoError(err)
		} else {
			suite.Require().Error(err)
		}
	}
}

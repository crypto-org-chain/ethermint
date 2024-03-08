package keeper_test

import (
	"fmt"
	"math/big"
	"testing"

	tmproto "github.com/cometbft/cometbft/proto/tendermint/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/evmos/ethermint/testutil"
	"github.com/stretchr/testify/suite"
)

type EIP1559TestSuite struct {
	testutil.BaseTestSuite
}

func TestEIP1559TestSuite(t *testing.T) {
	suite.Run(t, new(EIP1559TestSuite))
}

func (suite *EIP1559TestSuite) TestCalculateBaseFee() {
	testCases := []struct {
		name                 string
		NoBaseFee            bool
		blockHeight          int64
		parentBlockGasWanted uint64
		minGasPrice          sdk.Dec
		expFee               *big.Int
	}{
		{
			"without BaseFee",
			true,
			0,
			0,
			sdk.ZeroDec(),
			nil,
		},
		{
			"with BaseFee - initial EIP-1559 block",
			false,
			0,
			0,
			sdk.ZeroDec(),
			suite.App.FeeMarketKeeper.GetParams(suite.Ctx).BaseFee.BigInt(),
		},
		{
			"with BaseFee - parent block wanted the same gas as its target (ElasticityMultiplier = 2)",
			false,
			1,
			50,
			sdk.ZeroDec(),
			suite.App.FeeMarketKeeper.GetParams(suite.Ctx).BaseFee.BigInt(),
		},
		{
			"with BaseFee - parent block wanted the same gas as its target, with higher min gas price (ElasticityMultiplier = 2)",
			false,
			1,
			50,
			sdk.NewDec(1500000000),
			suite.App.FeeMarketKeeper.GetParams(suite.Ctx).BaseFee.BigInt(),
		},
		{
			"with BaseFee - parent block wanted more gas than its target (ElasticityMultiplier = 2)",
			false,
			1,
			100,
			sdk.ZeroDec(),
			big.NewInt(1125000000),
		},
		{
			"with BaseFee - parent block wanted more gas than its target, with higher min gas price (ElasticityMultiplier = 2)",
			false,
			1,
			100,
			sdk.NewDec(1500000000),
			big.NewInt(1125000000),
		},
		{
			"with BaseFee - Parent gas wanted smaller than parent gas target (ElasticityMultiplier = 2)",
			false,
			1,
			25,
			sdk.ZeroDec(),
			big.NewInt(937500000),
		},
		{
			"with BaseFee - Parent gas wanted smaller than parent gas target, with higher min gas price (ElasticityMultiplier = 2)",
			false,
			1,
			25,
			sdk.NewDec(1500000000),
			big.NewInt(1500000000),
		},
	}
	for _, tc := range testCases {
		suite.Run(fmt.Sprintf("Case %s", tc.name), func() {
			suite.SetupTest() // reset

			params := suite.App.FeeMarketKeeper.GetParams(suite.Ctx)
			params.NoBaseFee = tc.NoBaseFee
			params.MinGasPrice = tc.minGasPrice
			suite.App.FeeMarketKeeper.SetParams(suite.Ctx, params)

			// Set block height
			suite.Ctx = suite.Ctx.WithBlockHeight(tc.blockHeight)

			// Set parent block gas
			suite.App.FeeMarketKeeper.SetBlockGasWanted(suite.Ctx, tc.parentBlockGasWanted)

			// Set next block target/gasLimit through Consensus Param MaxGas
			blockParams := tmproto.BlockParams{
				MaxGas:   100,
				MaxBytes: 10,
			}
			consParams := tmproto.ConsensusParams{Block: &blockParams}
			suite.Ctx = suite.Ctx.WithConsensusParams(&consParams)

			fee := suite.App.FeeMarketKeeper.CalculateBaseFee(suite.Ctx)
			if tc.NoBaseFee {
				suite.Require().Nil(fee, tc.name)
			} else {
				suite.Require().Equal(tc.expFee, fee, tc.name)
			}
		})
	}
}

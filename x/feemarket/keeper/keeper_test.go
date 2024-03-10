package keeper_test

import (
	_ "embed"
	"math/big"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"

	"github.com/cosmos/cosmos-sdk/baseapp"
	"github.com/cosmos/cosmos-sdk/client"
	sdk "github.com/cosmos/cosmos-sdk/types"

	"github.com/evmos/ethermint/app"
	"github.com/evmos/ethermint/encoding"
	"github.com/evmos/ethermint/testutil"
	evmtypes "github.com/evmos/ethermint/x/evm/types"
	"github.com/evmos/ethermint/x/feemarket/types"

	ethtypes "github.com/ethereum/go-ethereum/core/types"

	abci "github.com/cometbft/cometbft/abci/types"
)

type KeeperTestSuite struct {
	testutil.FeeMarketTestSuiteWithAccountAndQueryClient

	// for generate test tx
	clientCtx client.Context
	ethSigner ethtypes.Signer
	denom     string
}

var s *KeeperTestSuite

func TestKeeperTestSuite(t *testing.T) {
	s = new(KeeperTestSuite)
	suite.Run(t, s)
}

// SetupTest setup test environment, it uses`require.TestingT` to support both `testing.T` and `testing.B`.
func (suite *KeeperTestSuite) SetupTest() {
	suite.FeeMarketTestSuiteWithAccountAndQueryClient.SetupTest(suite.T())
	encodingConfig := encoding.MakeConfig(app.ModuleBasics)
	suite.clientCtx = client.Context{}.WithTxConfig(encodingConfig.TxConfig)
	suite.ethSigner = ethtypes.LatestSignerForChainID(suite.App.EvmKeeper.ChainID())
	suite.denom = evmtypes.DefaultEVMDenom
}

// Commit commits and starts a new block with an updated context.
func (suite *KeeperTestSuite) Commit() {
	jumpTime := time.Second * 0
	header := suite.Ctx.BlockHeader()
	suite.App.EndBlock(abci.RequestEndBlock{Height: header.Height})
	_ = suite.App.Commit()

	header.Height += 1
	header.Time = header.Time.Add(jumpTime)
	suite.App.BeginBlock(abci.RequestBeginBlock{
		Header: header,
	})

	// update ctx
	suite.Ctx = suite.App.BaseApp.NewContext(false, header)

	queryHelper := baseapp.NewQueryServerTestHelper(suite.Ctx, suite.App.InterfaceRegistry())
	types.RegisterQueryServer(queryHelper, suite.App.FeeMarketKeeper)
	suite.FeeMarketQueryClient = types.NewQueryClient(queryHelper)
}

func (suite *KeeperTestSuite) TestSetGetBlockGasWanted() {
	testCases := []struct {
		name     string
		malleate func()
		expGas   uint64
	}{
		{
			"with last block given",
			func() {
				suite.App.FeeMarketKeeper.SetBlockGasWanted(suite.Ctx, uint64(1000000))
			},
			uint64(1000000),
		},
	}
	for _, tc := range testCases {
		suite.Run(tc.name, func() {
			suite.SetupTest()
			tc.malleate()
			gas := suite.App.FeeMarketKeeper.GetBlockGasWanted(suite.Ctx)
			suite.Require().Equal(tc.expGas, gas, tc.name)
		})
	}
}

func (suite *KeeperTestSuite) TestSetGetGasFee() {
	testCases := []struct {
		name     string
		malleate func()
		expFee   *big.Int
	}{
		{
			"with last block given",
			func() {
				suite.App.FeeMarketKeeper.SetBaseFee(suite.Ctx, sdk.OneDec().BigInt())
			},
			sdk.OneDec().BigInt(),
		},
	}
	for _, tc := range testCases {
		suite.Run(tc.name, func() {
			suite.SetupTest()
			tc.malleate()
			fee := suite.App.FeeMarketKeeper.GetBaseFee(suite.Ctx)
			suite.Require().Equal(tc.expFee, fee, tc.name)
		})
	}
}

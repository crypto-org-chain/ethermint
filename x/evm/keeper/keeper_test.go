package keeper_test

import (
	_ "embed"
	"math"
	"math/big"
	"testing"

	sdkmath "cosmossdk.io/math"
	"github.com/stretchr/testify/suite"

	"github.com/cosmos/cosmos-sdk/client"
	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"
	"github.com/evmos/ethermint/testutil"
	feemarkettypes "github.com/evmos/ethermint/x/feemarket/types"

	"github.com/evmos/ethermint/app"
	"github.com/evmos/ethermint/encoding"
	ethermint "github.com/evmos/ethermint/types"
	"github.com/evmos/ethermint/x/evm/statedb"
	"github.com/evmos/ethermint/x/evm/types"

	"github.com/ethereum/go-ethereum/common"
	ethtypes "github.com/ethereum/go-ethereum/core/types"

	abci "github.com/cometbft/cometbft/abci/types"
)

type KeeperTestSuite struct {
	testutil.EVMTestSuiteWithAccountAndQueryClient
	// for generate test tx
	clientCtx client.Context
	ethSigner ethtypes.Signer

	enableFeemarket bool
	enableLondonHF  bool
	denom           string
}

func TestKeeperTestSuite(t *testing.T) {
	s := new(KeeperTestSuite)
	s.enableFeemarket = false
	s.enableLondonHF = true
	suite.Run(t, s)
}

func (suite *KeeperTestSuite) SetupTest() {
	suite.EVMTestSuiteWithAccountAndQueryClient.SetupTestWithCb(suite.T(), func(app *app.EthermintApp, genesis app.GenesisState) app.GenesisState {
		feemarketGenesis := feemarkettypes.DefaultGenesisState()
		if suite.enableFeemarket {
			feemarketGenesis.Params.EnableHeight = 1
			feemarketGenesis.Params.NoBaseFee = false
		} else {
			feemarketGenesis.Params.NoBaseFee = true
		}
		genesis[feemarkettypes.ModuleName] = app.AppCodec().MustMarshalJSON(feemarketGenesis)
		if !suite.enableLondonHF {
			evmGenesis := types.DefaultGenesisState()
			maxInt := sdkmath.NewInt(math.MaxInt64)
			evmGenesis.Params.ChainConfig.LondonBlock = &maxInt
			evmGenesis.Params.ChainConfig.ArrowGlacierBlock = &maxInt
			evmGenesis.Params.ChainConfig.GrayGlacierBlock = &maxInt
			evmGenesis.Params.ChainConfig.MergeNetsplitBlock = &maxInt
			evmGenesis.Params.ChainConfig.ShanghaiTime = &maxInt
			genesis[types.ModuleName] = app.AppCodec().MustMarshalJSON(evmGenesis)
		}
		return genesis
	})

	encodingConfig := encoding.MakeConfig(app.ModuleBasics)
	suite.clientCtx = client.Context{}.WithTxConfig(encodingConfig.TxConfig)
	suite.ethSigner = ethtypes.LatestSignerForChainID(suite.App.EvmKeeper.ChainID())
	suite.denom = types.DefaultEVMDenom
}

func (suite *KeeperTestSuite) TestBaseFee() {
	testCases := []struct {
		name            string
		enableLondonHF  bool
		enableFeemarket bool
		expectBaseFee   *big.Int
	}{
		{"not enable london HF, not enable feemarket", false, false, nil},
		{"enable london HF, not enable feemarket", true, false, big.NewInt(0)},
		{"enable london HF, enable feemarket", true, true, big.NewInt(1000000000)},
		{"not enable london HF, enable feemarket", false, true, nil},
	}

	for _, tc := range testCases {
		suite.Run(tc.name, func() {
			suite.enableFeemarket = tc.enableFeemarket
			suite.enableLondonHF = tc.enableLondonHF
			suite.SetupTest()
			suite.App.EvmKeeper.BeginBlock(suite.Ctx, abci.RequestBeginBlock{})
			params := suite.App.EvmKeeper.GetParams(suite.Ctx)
			ethCfg := params.ChainConfig.EthereumConfig(suite.App.EvmKeeper.ChainID())
			baseFee := suite.App.EvmKeeper.GetBaseFee(suite.Ctx, ethCfg)
			suite.Require().Equal(tc.expectBaseFee, baseFee)
		})
	}
	suite.enableFeemarket = false
	suite.enableLondonHF = true
}

func (suite *KeeperTestSuite) TestGetAccountStorage() {
	testCases := []struct {
		name     string
		malleate func()
		expRes   []int
	}{
		{
			"Only one account that's not a contract (no storage)",
			func() {},
			[]int{0},
		},
		{
			"Two accounts - one contract (with storage), one wallet",
			func() {
				supply := big.NewInt(100)
				suite.DeployTestContract(suite.T(), suite.Address, supply, suite.enableFeemarket)
			},
			[]int{2, 0},
		},
	}

	for _, tc := range testCases {
		suite.Run(tc.name, func() {
			suite.SetupTest()
			tc.malleate()
			i := 0
			suite.App.AccountKeeper.IterateAccounts(suite.Ctx, func(account authtypes.AccountI) bool {
				ethAccount, ok := account.(ethermint.EthAccountI)
				if !ok {
					// ignore non EthAccounts
					return false
				}

				addr := ethAccount.EthAddress()
				storage := suite.App.EvmKeeper.GetAccountStorage(suite.Ctx, addr)

				suite.Require().Equal(tc.expRes[i], len(storage))
				i++
				return false
			})
		})
	}
}

func (suite *KeeperTestSuite) TestGetAccountOrEmpty() {
	empty := statedb.Account{
		CodeHash: types.EmptyCodeHash,
	}

	supply := big.NewInt(100)

	testCases := []struct {
		name     string
		addr     func() common.Address
		expEmpty bool
	}{
		{
			"unexisting account - get empty",
			func() common.Address {
				return common.Address{}
			},
			true,
		},
		{
			"existing contract account",
			func() common.Address {
				return suite.DeployTestContract(suite.T(), suite.Address, supply, suite.enableFeemarket)
			},
			false,
		},
	}

	for _, tc := range testCases {
		suite.Run(tc.name, func() {
			suite.SetupTest()
			res := suite.App.EvmKeeper.GetAccountOrEmpty(suite.Ctx, tc.addr())
			if tc.expEmpty {
				suite.Require().Equal(empty, res)
			} else {
				suite.Require().NotEqual(empty, res)
			}
		})
	}
}

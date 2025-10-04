package keeper_test

import (
	_ "embed"
	"math"
	"math/big"
	"strings"
	"testing"

	sdkmath "cosmossdk.io/math"
	"github.com/stretchr/testify/suite"

	"github.com/cosmos/cosmos-sdk/client"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/evmos/ethermint/testutil"
	feemarkettypes "github.com/evmos/ethermint/x/feemarket/types"

	"github.com/evmos/ethermint/encoding"
	"github.com/evmos/ethermint/evmd"
	ethermint "github.com/evmos/ethermint/types"
	"github.com/evmos/ethermint/x/evm/statedb"
	"github.com/evmos/ethermint/x/evm/types"

	"github.com/ethereum/go-ethereum/common"
	ethtypes "github.com/ethereum/go-ethereum/core/types"
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
	suite.EVMTestSuiteWithAccountAndQueryClient.SetupTestWithCb(suite.T(), func(app *evmd.EthermintApp, genesis evmd.GenesisState) evmd.GenesisState {
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
			evmGenesis.Params.ChainConfig.CancunTime = &maxInt
			evmGenesis.Params.ChainConfig.PragueTime = &maxInt
			genesis[types.ModuleName] = app.AppCodec().MustMarshalJSON(evmGenesis)
		}
		return genesis
	})
	encodingConfig := encoding.MakeConfig()
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
			suite.App.EvmKeeper.BeginBlock(suite.Ctx)
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
		malleate func() common.Address
	}{
		{
			"Only one account that's not a contract (no storage)",
			nil,
		},
		{
			"Two accounts - one contract (with storage), one wallet",
			func() common.Address {
				supply := big.NewInt(100)
				contractAddr := suite.DeployTestContract(suite.T(), suite.Address, supply, suite.enableFeemarket)
				return contractAddr
			},
		},
	}

	for _, tc := range testCases {
		suite.Run(tc.name, func() {
			suite.SetupTest()
			var contractAddr common.Address
			if tc.malleate != nil {
				contractAddr = tc.malleate()
			}

			var results []struct {
				addr    common.Address
				storage types.Storage
			}
			suite.App.AccountKeeper.IterateAccounts(suite.Ctx, func(account sdk.AccountI) bool {
				ethAccount, ok := account.(ethermint.EthAccountI)
				if !ok {
					// ignore non EthAccounts
					return false
				}

				addr := ethAccount.EthAddress()
				storage := suite.App.EvmKeeper.GetAccountStorage(suite.Ctx, addr)
				results = append(results, struct {
					addr    common.Address
					storage types.Storage
				}{addr, storage})
				return false
			})

			isPreinstall := func(addr common.Address) bool {
				for _, p := range types.DefaultPreinstalls {
					if strings.EqualFold(addr.Hex(), p.Address) {
						return true
					}
				}
				return false
			}

			for _, r := range results {
				if isPreinstall(r.addr) {
					// skip preinstall
					continue
				}
				if r.addr == contractAddr {
					suite.Require().NotEqual(0, len(r.storage),
						"expected account address %s to have non-zero amount of storage slots, got %d",
						r.addr.Hex(), len(r.storage),
					)
				} else {
					suite.Require().Len(r.storage, 0,
						"expected account address %s to have %d storage slots, got %d",
						r.addr.Hex(), 0, len(r.storage),
					)
				}
			}
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

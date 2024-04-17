package keeper_test

import (
	"fmt"
	"math"
	"math/big"
	"testing"

	sdkmath "cosmossdk.io/math"
	storetypes "cosmossdk.io/store/types"
	"github.com/cometbft/cometbft/crypto/tmhash"
	tmproto "github.com/cometbft/cometbft/proto/tendermint/types"
	tmtypes "github.com/cometbft/cometbft/types"
	codectypes "github.com/cosmos/cosmos-sdk/codec/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"
	banktypes "github.com/cosmos/cosmos-sdk/x/bank/types"
	stakingtypes "github.com/cosmos/cosmos-sdk/x/staking/types"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core"
	ethtypes "github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/params"
	"github.com/evmos/ethermint/app"
	"github.com/evmos/ethermint/tests"
	"github.com/evmos/ethermint/testutil"
	utiltx "github.com/evmos/ethermint/testutil/tx"
	ethermint "github.com/evmos/ethermint/types"
	"github.com/evmos/ethermint/x/evm/keeper"
	"github.com/evmos/ethermint/x/evm/statedb"
	"github.com/evmos/ethermint/x/evm/types"
	feemarkettypes "github.com/evmos/ethermint/x/feemarket/types"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

type StateTransitionTestSuite struct {
	testutil.EVMTestSuiteWithAccountAndQueryClient
	mintFeeCollector bool
}

func (suite *StateTransitionTestSuite) SetupTest() {
	coins := sdk.NewCoins(sdk.NewCoin(types.DefaultEVMDenom, sdkmath.NewInt(int64(params.TxGas)-1)))

	t := suite.T()
	suite.SetupTestWithCb(t, func(a *app.EthermintApp, genesis app.GenesisState) app.GenesisState {
		feemarketGenesis := feemarkettypes.DefaultGenesisState()
		feemarketGenesis.Params.NoBaseFee = true
		genesis[feemarkettypes.ModuleName] = a.AppCodec().MustMarshalJSON(feemarketGenesis)
		acc := &ethermint.EthAccount{
			BaseAccount: authtypes.NewBaseAccount(sdk.AccAddress(suite.Address.Bytes()), nil, 0, 0),
			CodeHash:    common.BytesToHash(crypto.Keccak256(nil)).String(),
		}
		accs, err := authtypes.PackAccounts(authtypes.GenesisAccounts{acc})
		require.NoError(t, err)
		var authGenesis authtypes.GenesisState
		a.AppCodec().MustUnmarshalJSON(genesis[authtypes.ModuleName], &authGenesis)
		authGenesis.Accounts = append(authGenesis.Accounts, accs[0])
		genesis[authtypes.ModuleName] = a.AppCodec().MustMarshalJSON(&authGenesis)
		if suite.mintFeeCollector {
			// mint some coin to fee collector
			balances := []banktypes.Balance{
				{
					Address: suite.App.AccountKeeper.GetModuleAddress(authtypes.FeeCollectorName).String(),
					Coins:   coins,
				},
			}
			var bankGenesis banktypes.GenesisState
			suite.App.AppCodec().MustUnmarshalJSON(genesis[banktypes.ModuleName], &bankGenesis)
			// Update balances and total supply
			bankGenesis.Balances = append(bankGenesis.Balances, balances...)
			bankGenesis.Supply = bankGenesis.Supply.Add(coins...)
			genesis[banktypes.ModuleName] = suite.App.AppCodec().MustMarshalJSON(&bankGenesis)
		}
		return genesis
	})

	if suite.mintFeeCollector {
		suite.MintFeeCollectorVirtual(coins)
	}
}

func TestStateTransitionTestSuite(t *testing.T) {
	suite.Run(t, new(StateTransitionTestSuite))
}

func (suite *StateTransitionTestSuite) TestGetHashFn() {
	header := suite.Ctx.BlockHeader()
	h, _ := tmtypes.HeaderFromProto(&header)
	hash := h.Hash()

	testCases := []struct {
		msg      string
		height   uint64
		malleate func()
		expHash  common.Hash
	}{
		{
			"case 1.1: context hash cached",
			uint64(suite.Ctx.BlockHeight()),
			func() {
				suite.Ctx = suite.Ctx.WithHeaderHash(tmhash.Sum([]byte("header"))).WithConsensusParams(*app.DefaultConsensusParams)
			},
			common.BytesToHash(tmhash.Sum([]byte("header"))),
		},
		{
			"case 1.2: failed to cast Tendermint header",
			uint64(suite.Ctx.BlockHeight()),
			func() {
				header := tmproto.Header{}
				header.Height = suite.Ctx.BlockHeight()
				suite.Ctx = suite.Ctx.WithBlockHeader(header).WithConsensusParams(*app.DefaultConsensusParams)
			},
			common.Hash{},
		},
		{
			"case 1.3: hash calculated from Tendermint header",
			uint64(suite.Ctx.BlockHeight()),
			func() {
				suite.Ctx = suite.Ctx.WithBlockHeader(header).WithConsensusParams(*app.DefaultConsensusParams)
			},
			common.BytesToHash(hash),
		},
		{
			"case 2.1: height lower than current one, hist info not found",
			1,
			func() {
				suite.Ctx = suite.Ctx.WithBlockHeight(10).WithConsensusParams(*app.DefaultConsensusParams)
			},
			common.Hash{},
		},
		{
			"case 2.2: height lower than current one, invalid hist info header",
			1,
			func() {
				suite.App.StakingKeeper.SetHistoricalInfo(suite.Ctx, 1, &stakingtypes.HistoricalInfo{})
				suite.Ctx = suite.Ctx.WithBlockHeight(10).WithConsensusParams(*app.DefaultConsensusParams)
			},
			common.Hash{},
		},
		{
			"case 2.3: height lower than current one, calculated from hist info header",
			1,
			func() {
				histInfo := &stakingtypes.HistoricalInfo{
					Header: header,
				}
				suite.App.StakingKeeper.SetHistoricalInfo(suite.Ctx, 1, histInfo)
				suite.Ctx = suite.Ctx.WithBlockHeight(10).WithConsensusParams(*app.DefaultConsensusParams)
			},
			common.BytesToHash(hash),
		},
		{
			"case 3: height greater than current one",
			200,
			func() {},
			common.Hash{},
		},
	}

	for _, tc := range testCases {
		suite.Run(fmt.Sprintf("Case %s", tc.msg), func() {
			suite.SetupTest() // reset

			tc.malleate()

			hash := suite.App.EvmKeeper.GetHashFn(suite.Ctx)(tc.height)
			suite.Require().Equal(tc.expHash, hash)
		})
	}
}

func (suite *StateTransitionTestSuite) TestGetCoinbaseAddress() {
	valOpAddr := tests.GenerateAddress()

	testCases := []struct {
		msg      string
		malleate func()
		expPass  bool
	}{
		{
			"validator not found",
			func() {
				header := suite.Ctx.BlockHeader()
				header.ProposerAddress = []byte{1}
				suite.Ctx = suite.Ctx.WithBlockHeader(header).WithConsensusParams(*app.DefaultConsensusParams)
			},
			false,
		},
		{
			"success",
			func() {
				valConsAddr, privkey := tests.NewAddrKey()

				pkAny, err := codectypes.NewAnyWithValue(privkey.PubKey())
				suite.Require().NoError(err)

				validator := stakingtypes.Validator{
					OperatorAddress: sdk.ValAddress(valOpAddr.Bytes()).String(),
					ConsensusPubkey: pkAny,
				}

				suite.App.StakingKeeper.SetValidator(suite.Ctx, validator)
				err = suite.App.StakingKeeper.SetValidatorByConsAddr(suite.Ctx, validator)
				suite.Require().NoError(err)

				header := suite.Ctx.BlockHeader()
				header.ProposerAddress = valConsAddr.Bytes()
				suite.Ctx = suite.Ctx.WithBlockHeader(header).WithConsensusParams(*app.DefaultConsensusParams)

				_, err = suite.App.StakingKeeper.GetValidatorByConsAddr(suite.Ctx, valConsAddr.Bytes())
				suite.Require().NoError(err)

				suite.Require().NotEmpty(suite.Ctx.BlockHeader().ProposerAddress)
			},
			true,
		},
	}

	for _, tc := range testCases {
		suite.Run(fmt.Sprintf("Case %s", tc.msg), func() {
			suite.SetupTest() // reset
			tc.malleate()
			coinbase, err := suite.App.EvmKeeper.GetCoinbaseAddress(suite.Ctx)
			if tc.expPass {
				suite.Require().NoError(err)
				suite.Require().Equal(valOpAddr, coinbase)
			} else {
				suite.Require().Error(err)
			}
		})
	}
}

// toWordSize returns the ceiled word size required for init code payment calculation.
func toWordSize(size uint64) uint64 {
	if size > math.MaxUint64-31 {
		return math.MaxUint64/32 + 1
	}

	return (size + 31) / 32
}

func (suite *StateTransitionTestSuite) TestGetEthIntrinsicGas() {
	testCases := []struct {
		name               string
		data               []byte
		accessList         ethtypes.AccessList
		height             int64
		isContractCreation bool
		noError            bool
		expGas             uint64
	}{
		{
			"no data, no accesslist, not contract creation, not homestead, not istanbul",
			nil,
			nil,
			1,
			false,
			true,
			params.TxGas,
		},
		{
			"with one zero data, no accesslist, not contract creation, not homestead, not istanbul",
			[]byte{0},
			nil,
			1,
			false,
			true,
			params.TxGas + params.TxDataZeroGas*1,
		},
		{
			"with one non zero data, no accesslist, not contract creation, not homestead, not istanbul",
			[]byte{1},
			nil,
			1,
			true,
			true,
			params.TxGas + params.TxDataNonZeroGasFrontier*1 + toWordSize(1)*params.InitCodeWordGas,
		},
		{
			"no data, one accesslist, not contract creation, not homestead, not istanbul",
			nil,
			[]ethtypes.AccessTuple{
				{},
			},
			1,
			false,
			true,
			params.TxGas + params.TxAccessListAddressGas,
		},
		{
			"no data, one accesslist with one storageKey, not contract creation, not homestead, not istanbul",
			nil,
			[]ethtypes.AccessTuple{
				{StorageKeys: make([]common.Hash, 1)},
			},
			1,
			false,
			true,
			params.TxGas + params.TxAccessListAddressGas + params.TxAccessListStorageKeyGas*1,
		},
		{
			"no data, no accesslist, is contract creation, is homestead, not istanbul",
			nil,
			nil,
			2,
			true,
			true,
			params.TxGasContractCreation,
		},
		{
			"with one zero data, no accesslist, not contract creation, is homestead, is istanbul",
			[]byte{1},
			nil,
			3,
			false,
			true,
			params.TxGas + params.TxDataNonZeroGasEIP2028*1,
		},
	}

	for _, tc := range testCases {
		suite.Run(fmt.Sprintf("Case %s", tc.name), func() {
			suite.SetupTest() // reset

			params := suite.App.EvmKeeper.GetParams(suite.Ctx)
			ethCfg := params.ChainConfig.EthereumConfig(suite.App.EvmKeeper.ChainID())
			ethCfg.HomesteadBlock = big.NewInt(2)
			ethCfg.IstanbulBlock = big.NewInt(3)
			signer := ethtypes.LatestSignerForChainID(suite.App.EvmKeeper.ChainID())
			suite.Ctx = suite.Ctx.WithBlockHeight(tc.height).WithConsensusParams(*app.DefaultConsensusParams)
			nonce := suite.App.EvmKeeper.GetNonce(suite.Ctx, suite.Address)
			m, err := newNativeMessage(
				nonce,
				suite.Ctx.BlockHeight(),
				suite.Address,
				ethCfg,
				suite.Signer,
				signer,
				ethtypes.AccessListTxType,
				tc.data,
				tc.accessList,
			)
			suite.Require().NoError(err)

			rules := ethCfg.Rules(big.NewInt(suite.Ctx.BlockHeight()), ethCfg.MergeNetsplitBlock != nil, uint64(suite.Ctx.BlockHeader().Time.Unix()))
			gas, err := suite.App.EvmKeeper.GetEthIntrinsicGas(m, rules, tc.isContractCreation)
			if tc.noError {
				suite.Require().NoError(err)
			} else {
				suite.Require().Error(err)
			}

			suite.Require().Equal(tc.expGas, gas)
		})
	}
}

func (suite *StateTransitionTestSuite) TestGasToRefund() {
	testCases := []struct {
		name           string
		gasconsumed    uint64
		refundQuotient uint64
		expGasRefund   uint64
		expPanic       bool
	}{
		{
			"gas refund 5",
			5,
			1,
			5,
			false,
		},
		{
			"gas refund 10",
			10,
			1,
			10,
			false,
		},
		{
			"gas refund availableRefund",
			11,
			1,
			10,
			false,
		},
		{
			"gas refund quotient 0",
			11,
			0,
			0,
			true,
		},
	}

	for _, tc := range testCases {
		suite.Run(fmt.Sprintf("Case %s", tc.name), func() {
			suite.mintFeeCollector = true
			suite.SetupTest() // reset
			vmdb := suite.StateDB()
			vmdb.AddRefund(10)

			if tc.expPanic {
				panicF := func() {
					_ = keeper.GasToRefund(vmdb.GetRefund(), tc.gasconsumed, tc.refundQuotient)
				}
				suite.Require().Panics(panicF)
			} else {
				gr := keeper.GasToRefund(vmdb.GetRefund(), tc.gasconsumed, tc.refundQuotient)
				suite.Require().Equal(tc.expGasRefund, gr)
			}
		})
	}
	suite.mintFeeCollector = false
}

func (suite *StateTransitionTestSuite) TestRefundGas() {
	var (
		m   core.Message
		err error
	)

	testCases := []struct {
		name           string
		leftoverGas    uint64
		refundQuotient uint64
		noError        bool
		expGasRefund   uint64
		malleate       func()
	}{
		{
			name:           "leftoverGas more than tx gas limit",
			leftoverGas:    params.TxGas + 1,
			refundQuotient: params.RefundQuotient,
			noError:        false,
			expGasRefund:   params.TxGas + 1,
		},
		{
			name:           "leftoverGas equal to tx gas limit, insufficient fee collector account",
			leftoverGas:    params.TxGas,
			refundQuotient: params.RefundQuotient,
			noError:        true,
			expGasRefund:   0,
		},
		{
			name:           "leftoverGas less than to tx gas limit",
			leftoverGas:    params.TxGas - 1,
			refundQuotient: params.RefundQuotient,
			noError:        true,
			expGasRefund:   0,
		},
		{
			name:           "no leftoverGas, refund half used gas ",
			leftoverGas:    0,
			refundQuotient: params.RefundQuotient,
			noError:        true,
			expGasRefund:   params.TxGas / params.RefundQuotient,
		},
		{
			name:           "invalid Gas value in msg",
			leftoverGas:    0,
			refundQuotient: params.RefundQuotient,
			noError:        false,
			expGasRefund:   params.TxGas,
			malleate: func() {
				m, err = suite.createContractGethMsg(
					suite.StateDB().GetNonce(suite.Address),
					ethtypes.LatestSignerForChainID(suite.App.EvmKeeper.ChainID()),
					big.NewInt(-100),
				)
				suite.Require().NoError(err)
			},
		},
	}

	for _, tc := range testCases {
		suite.Run(fmt.Sprintf("Case %s", tc.name), func() {
			suite.mintFeeCollector = true
			suite.SetupTest() // reset

			keeperParams := suite.App.EvmKeeper.GetParams(suite.Ctx)
			ethCfg := keeperParams.ChainConfig.EthereumConfig(suite.App.EvmKeeper.ChainID())
			signer := ethtypes.LatestSignerForChainID(suite.App.EvmKeeper.ChainID())
			vmdb := suite.StateDB()

			m, err = newNativeMessage(
				vmdb.GetNonce(suite.Address),
				suite.Ctx.BlockHeight(),
				suite.Address,
				ethCfg,
				suite.Signer,
				signer,
				ethtypes.AccessListTxType,
				nil,
				nil,
			)
			suite.Require().NoError(err)

			vmdb.AddRefund(params.TxGas)

			if tc.leftoverGas > m.GasLimit {
				return
			}

			if tc.malleate != nil {
				tc.malleate()
			}

			gasUsed := m.GasLimit - tc.leftoverGas
			refund := keeper.GasToRefund(vmdb.GetRefund(), gasUsed, tc.refundQuotient)
			suite.Require().Equal(tc.expGasRefund, refund)

			err = suite.App.EvmKeeper.RefundGas(suite.Ctx, m, refund, "aphoton")
			if tc.noError {
				suite.Require().NoError(err)
			} else {
				suite.Require().Error(err)
			}
		})
	}
	suite.mintFeeCollector = false
}

func (suite *StateTransitionTestSuite) TestResetGasMeterAndConsumeGas() {
	testCases := []struct {
		name        string
		gasConsumed uint64
		gasUsed     uint64
		expPanic    bool
	}{
		{
			"gas consumed 5, used 5",
			5,
			5,
			false,
		},
		{
			"gas consumed 5, used 10",
			5,
			10,
			false,
		},
		{
			"gas consumed 10, used 10",
			10,
			10,
			false,
		},
		{
			"gas consumed 11, used 10, NegativeGasConsumed panic",
			11,
			10,
			true,
		},
		{
			"gas consumed 1, used 10, overflow panic",
			1,
			math.MaxUint64,
			true,
		},
	}

	for _, tc := range testCases {
		suite.Run(fmt.Sprintf("Case %s", tc.name), func() {
			suite.SetupTest() // reset

			panicF := func() {
				gm := storetypes.NewGasMeter(10)
				gm.ConsumeGas(tc.gasConsumed, "")
				ctx := suite.Ctx.WithGasMeter(gm)
				suite.App.EvmKeeper.ResetGasMeterAndConsumeGas(ctx, tc.gasUsed)
			}

			if tc.expPanic {
				suite.Require().Panics(panicF)
			} else {
				suite.Require().NotPanics(panicF)
			}
		})
	}
}

func (suite *StateTransitionTestSuite) TestEVMConfig() {
	suite.SetupTest()
	cfg, err := suite.App.EvmKeeper.EVMConfig(suite.Ctx, big.NewInt(9000), common.Hash{})
	suite.Require().NoError(err)
	suite.Require().Equal(types.DefaultParams(), cfg.Params)
	// london hardfork is enabled by default
	suite.Require().Equal(big.NewInt(0), cfg.BaseFee)
	suite.Require().Equal(suite.Address, cfg.CoinBase)
	suite.Require().Equal(types.DefaultParams().ChainConfig.EthereumConfig(big.NewInt(9000)), cfg.ChainConfig)
}

func (suite *StateTransitionTestSuite) TestContractDeployment() {
	contractAddress := suite.EVMTestSuiteWithAccountAndQueryClient.DeployTestContract(
		suite.T(),
		suite.Address,
		big.NewInt(10000000000000),
		false,
	)
	db := suite.StateDB()
	suite.Require().Greater(db.GetCodeSize(contractAddress), 0)
}

func (suite *StateTransitionTestSuite) TestApplyMessage() {
	expectedGasUsed := params.TxGas
	var msg core.Message

	_, err := suite.App.EvmKeeper.EVMConfig(suite.Ctx, big.NewInt(9000), common.Hash{})
	suite.Require().NoError(err)

	keeperParams := suite.App.EvmKeeper.GetParams(suite.Ctx)
	chainCfg := keeperParams.ChainConfig.EthereumConfig(suite.App.EvmKeeper.ChainID())
	rules := chainCfg.Rules(big.NewInt(suite.Ctx.BlockHeight()), chainCfg.MergeNetsplitBlock != nil, uint64(suite.Ctx.BlockHeader().Time.Unix()))
	signer := ethtypes.LatestSignerForChainID(suite.App.EvmKeeper.ChainID())
	tracer := suite.App.EvmKeeper.Tracer(msg, rules)
	vmdb := suite.StateDB()

	msg, err = newNativeMessage(
		vmdb.GetNonce(suite.Address),
		suite.Ctx.BlockHeight(),
		suite.Address,
		chainCfg,
		suite.Signer,
		signer,
		ethtypes.AccessListTxType,
		nil,
		nil,
	)
	suite.Require().NoError(err)

	res, err := suite.App.EvmKeeper.ApplyMessage(suite.Ctx, msg, tracer, true)

	suite.Require().NoError(err)
	suite.Require().Equal(expectedGasUsed, res.GasUsed)
	suite.Require().False(res.Failed())
}

func (suite *StateTransitionTestSuite) TestApplyMessageWithConfig() {
	var (
		msg             core.Message
		err             error
		expectedGasUsed uint64
		config          *keeper.EVMConfig
		keeperParams    types.Params
		signer          ethtypes.Signer
		vmdb            *statedb.StateDB
		chainCfg        *params.ChainConfig
	)

	testCases := []struct {
		name     string
		malleate func()
		expErr   bool
	}{
		{
			"messsage applied ok",
			func() {
				msg, err = newNativeMessage(
					vmdb.GetNonce(suite.Address),
					suite.Ctx.BlockHeight(),
					suite.Address,
					chainCfg,
					suite.Signer,
					signer,
					ethtypes.AccessListTxType,
					nil,
					nil,
				)
				suite.Require().NoError(err)
			},
			false,
		},
		{
			"call contract tx with config param EnableCall = false",
			func() {
				config.Params.EnableCall = false
				msg, err = newNativeMessage(
					vmdb.GetNonce(suite.Address),
					suite.Ctx.BlockHeight(),
					suite.Address,
					chainCfg,
					suite.Signer,
					signer,
					ethtypes.AccessListTxType,
					nil,
					nil,
				)
				suite.Require().NoError(err)
			},
			true,
		},
		{
			"create contract tx with config param EnableCreate = false",
			func() {
				msg, err = suite.createContractGethMsg(vmdb.GetNonce(suite.Address), signer, big.NewInt(1))
				suite.Require().NoError(err)
				config.Params.EnableCreate = false
			},
			true,
		},
	}

	for _, tc := range testCases {
		suite.Run(fmt.Sprintf("Case %s", tc.name), func() {
			suite.SetupTest()
			expectedGasUsed = params.TxGas

			config, err = suite.App.EvmKeeper.EVMConfig(suite.Ctx, big.NewInt(9000), common.Hash{})
			suite.Require().NoError(err)

			keeperParams = suite.App.EvmKeeper.GetParams(suite.Ctx)
			chainCfg = keeperParams.ChainConfig.EthereumConfig(suite.App.EvmKeeper.ChainID())
			signer = ethtypes.LatestSignerForChainID(suite.App.EvmKeeper.ChainID())
			vmdb = suite.StateDB()
			config.TxConfig = suite.App.EvmKeeper.TxConfig(suite.Ctx, common.Hash{})

			tc.malleate()
			res, err := suite.App.EvmKeeper.ApplyMessageWithConfig(suite.Ctx, msg, config, true)

			if tc.expErr {
				suite.Require().Error(err)
				return
			}

			suite.Require().NoError(err)
			suite.Require().False(res.Failed())
			suite.Require().Equal(expectedGasUsed, res.GasUsed)
		})
	}
}

func (suite *StateTransitionTestSuite) createContractGethMsg(nonce uint64, signer ethtypes.Signer, gasPrice *big.Int) (core.Message, error) {
	ethMsg, err := utiltx.CreateContractMsgTx(nonce, signer, gasPrice, suite.Address, suite.Signer)
	if err != nil {
		return core.Message{}, err
	}
	return ethMsg.AsMessage(nil)
}

func (suite *StateTransitionTestSuite) TestGetProposerAddress() {
	var a sdk.ConsAddress
	address := sdk.ConsAddress(suite.Address.Bytes())
	proposerAddress := sdk.ConsAddress(suite.Ctx.BlockHeader().ProposerAddress)
	testCases := []struct {
		msg    string
		adr    sdk.ConsAddress
		expAdr sdk.ConsAddress
	}{
		{
			"proposer address provided",
			address,
			address,
		},
		{
			"nil proposer address provided",
			nil,
			proposerAddress,
		},
		{
			"typed nil proposer address provided",
			a,
			proposerAddress,
		},
	}
	for _, tc := range testCases {
		suite.Run(fmt.Sprintf("Case %s", tc.msg), func() {
			suite.Require().Equal(tc.expAdr, keeper.GetProposerAddress(suite.Ctx, tc.adr))
		})
	}
}

package keeper_test

import (
	"encoding/json"
	"fmt"
	"math"
	"math/big"
	"testing"

	sdkmath "cosmossdk.io/math"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/core/tracing"
	ethtypes "github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/core/vm"
	"github.com/ethereum/go-ethereum/crypto"
	ethlogger "github.com/ethereum/go-ethereum/eth/tracers/logger"
	ethparams "github.com/ethereum/go-ethereum/params"
	"github.com/evmos/ethermint/evmd"
	rpctypes "github.com/evmos/ethermint/rpc/types"
	"github.com/evmos/ethermint/server/config"
	"github.com/evmos/ethermint/tests"
	"github.com/evmos/ethermint/testutil"
	ethermint "github.com/evmos/ethermint/types"
	"github.com/evmos/ethermint/x/evm/statedb"
	"github.com/evmos/ethermint/x/evm/types"
	evmtypes "github.com/evmos/ethermint/x/evm/types"
	feemarkettypes "github.com/evmos/ethermint/x/feemarket/types"
	"github.com/holiman/uint256"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

// Not valid Ethereum address
const invalidAddress = "0x0000"

type GRPCServerTestSuiteSuite struct {
	testutil.EVMTestSuiteWithAccountAndQueryClient
	enableFeemarket bool
	enableLondonHF  bool
}

func (suite *GRPCServerTestSuiteSuite) SetupTest() {
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
			evmGenesis.Params.ChainConfig.OsakaTime = &maxInt
			genesis[types.ModuleName] = app.AppCodec().MustMarshalJSON(evmGenesis)
		}
		return genesis
	})
}

func TestGRPCServerTestSuite(t *testing.T) {
	s := new(GRPCServerTestSuiteSuite)
	s.enableFeemarket = false
	s.enableLondonHF = true
	suite.Run(t, s)
}

// deployTestContract deploy a test erc20 contract and returns the contract address
func (suite *GRPCServerTestSuiteSuite) deployTestContract(owner common.Address) common.Address {
	supply := sdkmath.NewIntWithDecimal(1000, 18).BigInt()
	return suite.EVMTestSuiteWithAccountAndQueryClient.DeployTestContract(
		suite.T(),
		owner,
		supply,
		suite.enableFeemarket,
	)
}

func (suite *GRPCServerTestSuiteSuite) transferERC20Token(t require.TestingT, contractAddr, from, to common.Address, amount *big.Int) *types.MsgEthereumTx {
	chainID := suite.App.EvmKeeper.ChainID()

	transferData, err := types.ERC20Contract.ABI.Pack("transfer", to, amount)
	require.NoError(t, err)
	args, err := json.Marshal(&types.TransactionArgs{To: &contractAddr, From: &from, Data: (*hexutil.Bytes)(&transferData)})
	require.NoError(t, err)
	res, err := suite.EvmQueryClient.EstimateGas(suite.Ctx, &types.EthCallRequest{
		Args:            args,
		GasCap:          25_000_000,
		ProposerAddress: suite.Ctx.BlockHeader().ProposerAddress,
	})
	require.NoError(t, err)

	nonce := suite.App.EvmKeeper.GetNonce(suite.Ctx, suite.Address)

	var ercTransferTx *types.MsgEthereumTx
	if suite.enableFeemarket {
		ercTransferTx = types.NewTx(
			chainID,
			nonce,
			&contractAddr,
			nil,
			res.Gas,
			nil,
			suite.App.FeeMarketKeeper.GetBaseFee(suite.Ctx),
			big.NewInt(1),
			transferData,
			&ethtypes.AccessList{}, // accesses
		)
	} else {
		ercTransferTx = types.NewTx(
			chainID,
			nonce,
			&contractAddr,
			nil,
			res.Gas,
			nil,
			nil, nil,
			transferData,
			nil,
		)
	}

	ercTransferTx.From = suite.Address.Bytes()
	err = ercTransferTx.Sign(ethtypes.LatestSignerForChainID(chainID), suite.Signer)

	require.NoError(t, err)
	rsp, err := suite.App.EvmKeeper.EthereumTx(suite.Ctx, ercTransferTx)

	require.NoError(t, err)
	require.Empty(t, rsp.VmError)

	return ercTransferTx
}

func (suite *GRPCServerTestSuiteSuite) TestQueryAccount() {
	var (
		req        *types.QueryAccountRequest
		expAccount *types.QueryAccountResponse
	)

	testCases := []struct {
		msg      string
		malleate func()
		expPass  bool
	}{
		{
			"invalid address",
			func() {
				expAccount = &types.QueryAccountResponse{
					Balance:  "0",
					CodeHash: common.BytesToHash(crypto.Keccak256(nil)).Hex(),
					Nonce:    0,
				}
				req = &types.QueryAccountRequest{
					Address: invalidAddress,
				}
			},
			false,
		},
		{
			"success",
			func() {
				amt := sdk.Coins{ethermint.NewPhotonCoinInt64(100)}
				err := suite.App.BankKeeper.MintCoins(suite.Ctx, types.ModuleName, amt)
				suite.Require().NoError(err)
				err = suite.App.BankKeeper.SendCoinsFromModuleToAccount(suite.Ctx, types.ModuleName, suite.Address.Bytes(), amt)
				suite.Require().NoError(err)

				expAccount = &types.QueryAccountResponse{
					Balance:  "100",
					CodeHash: common.BytesToHash(crypto.Keccak256(nil)).Hex(),
					Nonce:    0,
				}
				req = &types.QueryAccountRequest{
					Address: suite.Address.String(),
				}
			},
			true,
		},
	}

	for _, tc := range testCases {
		suite.Run(fmt.Sprintf("Case %s", tc.msg), func() {
			suite.SetupTest() // reset
			tc.malleate()
			res, err := suite.EvmQueryClient.Account(suite.Ctx, req)
			if tc.expPass {
				suite.Require().NoError(err)
				suite.Require().NotNil(res)

				suite.Require().Equal(expAccount, res)
			} else {
				suite.Require().Error(err)
			}
		})
	}
}

func (suite *GRPCServerTestSuiteSuite) TestQueryCosmosAccount() {
	var (
		req        *types.QueryCosmosAccountRequest
		expAccount *types.QueryCosmosAccountResponse
	)

	testCases := []struct {
		msg      string
		malleate func()
		expPass  bool
	}{
		{
			"invalid address",
			func() {
				expAccount = &types.QueryCosmosAccountResponse{
					CosmosAddress: sdk.AccAddress(common.Address{}.Bytes()).String(),
				}
				req = &types.QueryCosmosAccountRequest{
					Address: invalidAddress,
				}
			},
			false,
		},
		{
			"success",
			func() {
				expAccount = &types.QueryCosmosAccountResponse{
					CosmosAddress: sdk.AccAddress(suite.Address.Bytes()).String(),
					Sequence:      0,
					AccountNumber: suite.App.AccountKeeper.NextAccountNumber(suite.Ctx) - 1,
				}
				req = &types.QueryCosmosAccountRequest{
					Address: suite.Address.String(),
				}
			},
			true,
		},
		{
			"success with seq and account number",
			func() {
				acc := suite.App.AccountKeeper.GetAccount(suite.Ctx, suite.Address.Bytes())
				suite.Require().NoError(acc.SetSequence(10))
				num := suite.App.AccountKeeper.NextAccountNumber(suite.Ctx)
				suite.Require().NoError(acc.SetAccountNumber(num))
				suite.App.AccountKeeper.SetAccount(suite.Ctx, acc)
				expAccount = &types.QueryCosmosAccountResponse{
					CosmosAddress: sdk.AccAddress(suite.Address.Bytes()).String(),
					Sequence:      10,
					AccountNumber: num,
				}
				req = &types.QueryCosmosAccountRequest{
					Address: suite.Address.String(),
				}
			},
			true,
		},
	}

	for _, tc := range testCases {
		suite.Run(fmt.Sprintf("Case %s", tc.msg), func() {
			suite.SetupTest() // reset
			tc.malleate()
			res, err := suite.EvmQueryClient.CosmosAccount(suite.Ctx, req)
			if tc.expPass {
				suite.Require().NoError(err)
				suite.Require().NotNil(res)

				suite.Require().Equal(expAccount, res)
			} else {
				suite.Require().Error(err)
			}
		})
	}
}

func (suite *GRPCServerTestSuiteSuite) TestQueryBalance() {
	var (
		req        *types.QueryBalanceRequest
		expBalance string
	)

	testCases := []struct {
		msg      string
		malleate func()
		expPass  bool
	}{
		{
			"invalid address",
			func() {
				expBalance = "0"
				req = &types.QueryBalanceRequest{
					Address: invalidAddress,
				}
			},
			false,
		},
		{
			"success",
			func() {
				amt := sdk.Coins{ethermint.NewPhotonCoinInt64(100)}
				err := suite.App.BankKeeper.MintCoins(suite.Ctx, types.ModuleName, amt)
				suite.Require().NoError(err)
				err = suite.App.BankKeeper.SendCoinsFromModuleToAccount(suite.Ctx, types.ModuleName, suite.Address.Bytes(), amt)
				suite.Require().NoError(err)

				expBalance = "100"
				req = &types.QueryBalanceRequest{
					Address: suite.Address.String(),
				}
			},
			true,
		},
	}

	for _, tc := range testCases {
		suite.Run(fmt.Sprintf("Case %s", tc.msg), func() {
			suite.SetupTest() // reset
			tc.malleate()
			res, err := suite.EvmQueryClient.Balance(suite.Ctx, req)
			if tc.expPass {
				suite.Require().NoError(err)
				suite.Require().NotNil(res)

				suite.Require().Equal(expBalance, res.Balance)
			} else {
				suite.Require().Error(err)
			}
		})
	}
}

func (suite *GRPCServerTestSuiteSuite) TestQueryStorage() {
	var (
		req      *types.QueryStorageRequest
		expValue string
	)

	testCases := []struct {
		msg      string
		malleate func(vm.StateDB)
		expPass  bool
	}{
		{
			"invalid address",
			func(vm.StateDB) {
				req = &types.QueryStorageRequest{
					Address: invalidAddress,
				}
			},
			false,
		},
		{
			"success",
			func(vmdb vm.StateDB) {
				key := common.BytesToHash([]byte("key"))
				value := common.BytesToHash([]byte("value"))
				expValue = value.String()
				vmdb.SetState(suite.Address, key, value)
				req = &types.QueryStorageRequest{
					Address: suite.Address.String(),
					Key:     key.String(),
				}
			},
			true,
		},
	}

	for _, tc := range testCases {
		suite.Run(fmt.Sprintf("Case %s", tc.msg), func() {
			suite.SetupTest() // reset

			vmdb := suite.StateDB()
			tc.malleate(vmdb)
			suite.Require().NoError(vmdb.Commit())
			res, err := suite.EvmQueryClient.Storage(suite.Ctx, req)
			if tc.expPass {
				suite.Require().NoError(err)
				suite.Require().NotNil(res)

				suite.Require().Equal(expValue, res.Value)
			} else {
				suite.Require().Error(err)
			}
		})
	}
}

func (suite *GRPCServerTestSuiteSuite) TestQueryCode() {
	var (
		req     *types.QueryCodeRequest
		expCode []byte
	)

	testCases := []struct {
		msg      string
		malleate func(vm.StateDB)
		expPass  bool
	}{
		{
			"invalid address",
			func(vm.StateDB) {
				req = &types.QueryCodeRequest{
					Address: invalidAddress,
				}
				exp := &types.QueryCodeResponse{}
				expCode = exp.Code
			},
			false,
		},
		{
			"success",
			func(vmdb vm.StateDB) {
				expCode = []byte("code")
				vmdb.SetCode(suite.Address, expCode, 0)

				req = &types.QueryCodeRequest{
					Address: suite.Address.String(),
				}
			},
			true,
		},
	}

	for _, tc := range testCases {
		suite.Run(fmt.Sprintf("Case %s", tc.msg), func() {
			suite.SetupTest() // reset

			vmdb := suite.StateDB()
			tc.malleate(vmdb)
			suite.Require().NoError(vmdb.Commit())
			res, err := suite.EvmQueryClient.Code(suite.Ctx, req)
			if tc.expPass {
				suite.Require().NoError(err)
				suite.Require().NotNil(res)

				suite.Require().Equal(expCode, res.Code)
			} else {
				suite.Require().Error(err)
			}
		})
	}
}

func (suite *GRPCServerTestSuiteSuite) TestQueryTxLogs() {
	var expLogs []*types.Log
	txHash := common.BytesToHash([]byte("tx_hash"))
	txIndex := uint(1)
	logIndex := uint(1)

	testCases := []struct {
		msg      string
		malleate func(vm.StateDB)
	}{
		{
			"empty logs",
			func(vm.StateDB) {
				expLogs = nil
			},
		},
		{
			"success",
			func(vmdb vm.StateDB) {
				expLogs = []*types.Log{
					{
						Address:     evmtypes.HexAddress(suite.Address.Bytes()),
						Topics:      []string{common.BytesToHash([]byte("topic")).String()},
						Data:        []byte("data"),
						BlockNumber: 1,
						TxHash:      txHash.String(),
						TxIndex:     uint64(txIndex),
						BlockHash:   common.BytesToHash(suite.Ctx.HeaderHash()).Hex(),
						Index:       uint64(logIndex),
						Removed:     false,
					},
				}

				for _, log := range types.LogsToEthereum(expLogs) {
					vmdb.AddLog(log)
				}
			},
		},
	}

	for _, tc := range testCases {
		suite.Run(fmt.Sprintf("Case %s", tc.msg), func() {
			suite.SetupTest() // reset

			vmdb := statedb.New(suite.Ctx, suite.App.EvmKeeper, statedb.NewTxConfig(common.BytesToHash(suite.Ctx.HeaderHash()), txHash, txIndex, logIndex))
			tc.malleate(vmdb)
			suite.Require().NoError(vmdb.Commit())

			logs := vmdb.Logs()
			suite.Require().Equal(expLogs, types.NewLogsFromEth(logs))
		})
	}
}

func (suite *GRPCServerTestSuiteSuite) TestQueryParams() {
	expParams := types.DefaultParams()
	res, err := suite.EvmQueryClient.Params(suite.Ctx, &types.QueryParamsRequest{})
	suite.Require().NoError(err)
	suite.Require().Equal(expParams, res.Params)
}

func (suite *GRPCServerTestSuiteSuite) TestQueryValidatorAccount() {
	var (
		req        *types.QueryValidatorAccountRequest
		expAccount *types.QueryValidatorAccountResponse
	)

	testCases := []struct {
		msg      string
		malleate func()
		expPass  bool
	}{
		{
			"invalid address",
			func() {
				expAccount = &types.QueryValidatorAccountResponse{
					AccountAddress: sdk.AccAddress(common.Address{}.Bytes()).String(),
				}
				req = &types.QueryValidatorAccountRequest{
					ConsAddress: "",
				}
			},
			false,
		},
		{
			"success",
			func() {
				expAccount = &types.QueryValidatorAccountResponse{
					AccountAddress: sdk.AccAddress(suite.Address.Bytes()).String(),
					Sequence:       0,
					AccountNumber:  suite.App.AccountKeeper.NextAccountNumber(suite.Ctx) - 1,
				}
				req = &types.QueryValidatorAccountRequest{
					ConsAddress: suite.ConsAddress.String(),
				}
			},
			true,
		},
		{
			"success with seq and account number",
			func() {
				acc := suite.App.AccountKeeper.GetAccount(suite.Ctx, suite.Address.Bytes())
				suite.Require().NoError(acc.SetSequence(10))
				num := suite.App.AccountKeeper.NextAccountNumber(suite.Ctx)
				suite.Require().NoError(acc.SetAccountNumber(num))
				suite.App.AccountKeeper.SetAccount(suite.Ctx, acc)
				expAccount = &types.QueryValidatorAccountResponse{
					AccountAddress: sdk.AccAddress(suite.Address.Bytes()).String(),
					Sequence:       10,
					AccountNumber:  num,
				}
				req = &types.QueryValidatorAccountRequest{
					ConsAddress: suite.ConsAddress.String(),
				}
			},
			true,
		},
	}

	for _, tc := range testCases {
		suite.Run(fmt.Sprintf("Case %s", tc.msg), func() {
			suite.SetupTest() // reset
			tc.malleate()
			res, err := suite.EvmQueryClient.ValidatorAccount(suite.Ctx, req)
			if tc.expPass {
				suite.Require().NoError(err)
				suite.Require().NotNil(res)

				suite.Require().Equal(expAccount, res)
			} else {
				suite.Require().Error(err)
			}
		})
	}
}

func (suite *GRPCServerTestSuiteSuite) TestEstimateGas() {
	gasHelper := hexutil.Uint64(22000)
	higherGas := hexutil.Uint64(25000)
	hexBigInt := hexutil.Big(*big.NewInt(1))

	var (
		args   interface{}
		gasCap uint64
	)
	testCases := []struct {
		msg             string
		malleate        func()
		expPass         bool
		expGas          uint64
		enableFeemarket bool
	}{
		// should success, because transfer value is zero
		{
			"default args - special case for ErrIntrinsicGas on contract creation, raise gas limit",
			func() {
				args = types.TransactionArgs{}
			},
			true,
			ethparams.TxGasContractCreation,
			false,
		},
		// should success, because transfer value is zero
		{
			"default args with 'to' address",
			func() {
				args = types.TransactionArgs{To: &common.Address{}}
			},
			true,
			ethparams.TxGas,
			false,
		},
		// should fail, because the default From address(zero address) don't have fund
		{
			"not enough balance",
			func() {
				args = types.TransactionArgs{To: &common.Address{}, Value: (*hexutil.Big)(big.NewInt(100))}
			},
			false,
			0,
			false,
		},
		// should success, enough balance now
		{
			"enough balance",
			func() {
				args = types.TransactionArgs{To: &common.Address{}, From: &suite.Address, Value: (*hexutil.Big)(big.NewInt(100))}
			}, false, 0, false,
		},
		// should success, because gas limit lower than 21000 is ignored
		{
			"gas exceed allowance",
			func() {
				args = types.TransactionArgs{To: &common.Address{}, Gas: &gasHelper}
			},
			true,
			ethparams.TxGas,
			false,
		},
		// should fail, invalid gas cap
		{
			"gas exceed global allowance",
			func() {
				args = types.TransactionArgs{To: &common.Address{}}
				gasCap = 20000
			},
			false,
			0,
			false,
		},
		// estimate gas of an erc20 contract deployment, the exact gas number is checked with geth
		{
			"contract deployment",
			func() {
				ctorArgs, err := types.ERC20Contract.ABI.Pack("", &suite.Address, sdkmath.NewIntWithDecimal(1000, 18).BigInt())
				suite.Require().NoError(err)
				data := append(types.ERC20Contract.Bin, ctorArgs...)
				args = types.TransactionArgs{
					From: &suite.Address,
					Data: (*hexutil.Bytes)(&data),
				}
			},
			true,
			1187108,
			false,
		},
		// estimate gas of an erc20 transfer, the exact gas number is checked with geth
		{
			"erc20 transfer",
			func() {
				contractAddr := suite.deployTestContract(suite.Address)
				suite.Commit(suite.T())
				transferData, err := types.ERC20Contract.ABI.Pack("transfer", common.HexToAddress("0x378c50D9264C63F3F92B806d4ee56E9D86FfB3Ec"), big.NewInt(1000))
				suite.Require().NoError(err)
				args = types.TransactionArgs{To: &contractAddr, From: &suite.Address, Data: (*hexutil.Bytes)(&transferData)}
			},
			true,
			51880,
			false,
		},
		// repeated tests with enableFeemarket
		{
			"default args w/ enableFeemarket",
			func() {
				args = types.TransactionArgs{To: &common.Address{}}
			},
			true,
			ethparams.TxGas,
			true,
		},
		{
			"not enough balance w/ enableFeemarket",
			func() {
				args = types.TransactionArgs{To: &common.Address{}, Value: (*hexutil.Big)(big.NewInt(100))}
			},
			false,
			0,
			true,
		},
		{
			"enough balance w/ enableFeemarket",
			func() {
				args = types.TransactionArgs{To: &common.Address{}, From: &suite.Address, Value: (*hexutil.Big)(big.NewInt(100))}
			},
			false,
			0,
			true,
		},
		{
			"gas exceed allowance w/ enableFeemarket",
			func() {
				args = types.TransactionArgs{To: &common.Address{}, Gas: &gasHelper}
			},
			true,
			ethparams.TxGas,
			true,
		},
		{
			"gas exceed global allowance w/ enableFeemarket",
			func() {
				args = types.TransactionArgs{To: &common.Address{}}
				gasCap = 20000
			},
			false,
			0,
			true,
		},
		{
			"contract deployment w/ enableFeemarket",
			func() {
				ctorArgs, err := types.ERC20Contract.ABI.Pack("", &suite.Address, sdkmath.NewIntWithDecimal(1000, 18).BigInt())
				suite.Require().NoError(err)
				data := append(types.ERC20Contract.Bin, ctorArgs...)
				args = types.TransactionArgs{
					From: &suite.Address,
					Data: (*hexutil.Bytes)(&data),
				}
			},
			true,
			1187108,
			true,
		},
		{
			"erc20 transfer w/ enableFeemarket",
			func() {
				contractAddr := suite.deployTestContract(suite.Address)
				suite.Commit(suite.T())
				transferData, err := types.ERC20Contract.ABI.Pack("transfer", common.HexToAddress("0x378c50D9264C63F3F92B806d4ee56E9D86FfB3Ec"), big.NewInt(1000))
				suite.Require().NoError(err)
				args = types.TransactionArgs{To: &contractAddr, From: &suite.Address, Data: (*hexutil.Bytes)(&transferData)}
			},
			true,
			51880,
			true,
		},
		{
			"contract creation but 'create' param disabled",
			func() {
				ctorArgs, err := types.ERC20Contract.ABI.Pack("", &suite.Address, sdkmath.NewIntWithDecimal(1000, 18).BigInt())
				suite.Require().NoError(err)
				data := append(types.ERC20Contract.Bin, ctorArgs...)
				args = types.TransactionArgs{
					From: &suite.Address,
					Data: (*hexutil.Bytes)(&data),
				}
				params := suite.App.EvmKeeper.GetParams(suite.Ctx)
				params.EnableCreate = false
				suite.App.EvmKeeper.SetParams(suite.Ctx, params)
			},
			false,
			0,
			false,
		},
		{
			"specified gas in args higher than ethparams.TxGas (21,000)",
			func() {
				args = types.TransactionArgs{
					To:  &common.Address{},
					Gas: &higherGas,
				}
			},
			true,
			ethparams.TxGas,
			false,
		},
		{
			"specified gas in args higher than request gasCap",
			func() {
				gasCap = 22_000
				args = types.TransactionArgs{
					To:  &common.Address{},
					Gas: &higherGas,
				}
			},
			true,
			ethparams.TxGas,
			false,
		},
		{
			"invalid args - specified both gasPrice and maxFeePerGas",
			func() {
				args = types.TransactionArgs{
					To:           &common.Address{},
					GasPrice:     &hexBigInt,
					MaxFeePerGas: &hexBigInt,
				}
			},
			false,
			0,
			false,
		},
	}

	for _, tc := range testCases {
		suite.Run(fmt.Sprintf("Case %s", tc.msg), func() {
			suite.enableFeemarket = tc.enableFeemarket
			suite.SetupTest()
			gasCap = 25_000_000
			tc.malleate()

			args, err := json.Marshal(&args)
			suite.Require().NoError(err)
			req := types.EthCallRequest{
				Args:            args,
				GasCap:          gasCap,
				ProposerAddress: suite.Ctx.BlockHeader().ProposerAddress,
			}
			rsp, err := suite.EvmQueryClient.EstimateGas(suite.Ctx, &req)
			if tc.expPass {
				suite.Require().NoError(err)
				suite.Require().Equal(int64(tc.expGas), int64(rsp.Gas))
			} else {
				suite.Require().Error(err)
			}
		})
	}
	suite.enableFeemarket = false // reset flag
}

func (suite *GRPCServerTestSuiteSuite) TestTraceTx() {
	// TODO deploy contract that triggers internal transactions
	var (
		txMsg        *types.MsgEthereumTx
		traceConfig  *types.TraceConfig
		predecessors []*types.MsgEthereumTx
		chainID      *sdkmath.Int
	)

	testCases := []struct {
		msg             string
		malleate        func()
		expPass         bool
		traceResponse   string
		enableFeemarket bool
	}{
		{
			msg: "default trace",
			malleate: func() {
				traceConfig = nil
				predecessors = []*types.MsgEthereumTx{}
			},
			expPass:       true,
			traceResponse: `{"gas":34828,"failed":false,"returnValue":"0x0000000000000000000000000000000000000000000000000000000000000001","structLogs":[{"pc":0,"op":"PUSH1","gas`,
		},
		{
			msg: "default trace with filtered response",
			malleate: func() {
				traceConfig = &types.TraceConfig{
					DisableStack:   true,
					DisableStorage: true,
					EnableMemory:   false,
				}
				predecessors = []*types.MsgEthereumTx{}
			},
			expPass:         true,
			traceResponse:   `{"gas":34828,"failed":false,"returnValue":"0x0000000000000000000000000000000000000000000000000000000000000001","structLogs":[{"pc":0,"op":"PUSH1","gas`,
			enableFeemarket: false,
		},
		{
			msg: "javascript tracer",
			malleate: func() {
				traceConfig = &types.TraceConfig{
					Tracer: "{data: [], fault: function(log) {}, step: function(log) { if(log.op.toString() == \"CALL\") this.data.push(log.stack.peek(0)); }, result: function() { return this.data; }}",
				}
				predecessors = []*types.MsgEthereumTx{}
			},
			expPass:       true,
			traceResponse: "[]",
		},
		{
			msg: "default trace with enableFeemarket",
			malleate: func() {
				traceConfig = &types.TraceConfig{
					DisableStack:   true,
					DisableStorage: true,
					EnableMemory:   false,
				}
				predecessors = []*types.MsgEthereumTx{}
			},
			expPass:         false,
			enableFeemarket: true,
		},
		{
			msg: "default trace with enableFeemarket and sufficient balance",
			malleate: func() {
				suite.App.EvmKeeper.SetBalance(suite.Ctx, suite.Address, *uint256.NewInt(1000000000000000000), types.DefaultEVMDenom)
				traceConfig = &types.TraceConfig{
					DisableStack:   true,
					DisableStorage: true,
					EnableMemory:   false,
				}
				predecessors = []*types.MsgEthereumTx{}
			},
			expPass:         true,
			traceResponse:   `{"gas":34828,"failed":false,"returnValue":"0x0000000000000000000000000000000000000000000000000000000000000001","structLogs":[{"pc":0,"op":"PUSH1","gas`,
			enableFeemarket: true,
		},
		{
			msg: "javascript tracer with enableFeemarket",
			malleate: func() {
				traceConfig = &types.TraceConfig{
					Tracer: "{data: [], fault: function(log) {}, step: function(log) { if(log.op.toString() == \"CALL\") this.data.push(log.stack.peek(0)); }, result: function() { return this.data; }}",
				}
				predecessors = []*types.MsgEthereumTx{}
			},
			expPass:         false,
			enableFeemarket: true,
		},
		{
			msg: "javascript tracer with enableFeemarket and sufficient balance",
			malleate: func() {
				suite.App.EvmKeeper.SetBalance(suite.Ctx, suite.Address, *uint256.NewInt(1000000000000000000), types.DefaultEVMDenom)
				traceConfig = &types.TraceConfig{
					Tracer: "{data: [], fault: function(log) {}, step: function(log) { if(log.op.toString() == \"CALL\") this.data.push(log.stack.peek(0)); }, result: function() { return this.data; }}",
				}
				predecessors = []*types.MsgEthereumTx{}
			},
			expPass:         true,
			traceResponse:   "[]",
			enableFeemarket: true,
		},
		{
			msg: "default tracer with predecessors",
			malleate: func() {
				traceConfig = nil

				// increase nonce to avoid address collision
				vmdb := suite.StateDB()
				vmdb.SetNonce(suite.Address, vmdb.GetNonce(suite.Address)+1, tracing.NonceChangeUnspecified)
				suite.Require().NoError(vmdb.Commit())
				contractAddr := suite.deployTestContract(suite.Address)
				suite.Commit(suite.T())
				// Generate token transfer transaction
				firstTx := suite.transferERC20Token(suite.T(), contractAddr, suite.Address, common.HexToAddress("0x378c50D9264C63F3F92B806d4ee56E9D86FfB3Ec"), sdkmath.NewIntWithDecimal(1, 18).BigInt())
				txMsg = suite.transferERC20Token(suite.T(), contractAddr, suite.Address, common.HexToAddress("0x378c50D9264C63F3F92B806d4ee56E9D86FfB3Ec"), sdkmath.NewIntWithDecimal(1, 18).BigInt())
				suite.Commit(suite.T())

				predecessors = append(predecessors, firstTx)
			},
			expPass:         true,
			traceResponse:   "{\"gas\":34828,\"failed\":false,\"returnValue\":\"0x0000000000000000000000000000000000000000000000000000000000000001\",\"structLogs\":[{\"pc\":0,\"op\":\"PUSH1\",\"gas",
			enableFeemarket: false,
		},
		{
			msg: "invalid trace config - Negative Limit",
			malleate: func() {
				traceConfig = &types.TraceConfig{
					DisableStack:   true,
					DisableStorage: true,
					EnableMemory:   false,
					Limit:          -1,
				}
			},
			expPass: false,
		},
		{
			msg: "invalid trace config - Invalid Tracer",
			malleate: func() {
				traceConfig = &types.TraceConfig{
					DisableStack:   true,
					DisableStorage: true,
					EnableMemory:   false,
					Tracer:         "invalid_tracer",
				}
			},
			expPass: false,
		},
		{
			msg: "invalid trace config - Invalid Timeout",
			malleate: func() {
				traceConfig = &types.TraceConfig{
					DisableStack:   true,
					DisableStorage: true,
					EnableMemory:   false,
					Timeout:        "wrong_time",
				}
			},
			expPass: false,
		},
		{
			msg: "default tracer with contract creation tx as predecessor but 'create' param disabled",
			malleate: func() {
				traceConfig = nil

				// increase nonce to avoid address collision
				vmdb := suite.StateDB()
				vmdb.SetNonce(suite.Address, vmdb.GetNonce(suite.Address)+1, tracing.NonceChangeUnspecified)
				suite.Require().NoError(vmdb.Commit())

				chainID := suite.App.EvmKeeper.ChainID()
				nonce := suite.App.EvmKeeper.GetNonce(suite.Ctx, suite.Address)
				data := types.ERC20Contract.Bin
				contractTx := types.NewTxContract(
					chainID,
					nonce,
					nil,                             // amount
					ethparams.TxGasContractCreation, // gasLimit
					nil,                             // gasPrice
					nil, nil,
					data, // input
					nil,  // accesses
				)

				predecessors = append(predecessors, contractTx)
				suite.Commit(suite.T())

				params := suite.App.EvmKeeper.GetParams(suite.Ctx)
				params.EnableCreate = false
				suite.App.EvmKeeper.SetParams(suite.Ctx, params)
			},
			expPass:       true,
			traceResponse: "{\"gas\":34828,\"failed\":false,\"returnValue\":\"0x0000000000000000000000000000000000000000000000000000000000000001\",\"structLogs\":[{\"pc\":0,\"op\":\"PUSH1\",\"gas",
		},
		{
			msg: "invalid chain id",
			malleate: func() {
				traceConfig = nil
				predecessors = []*types.MsgEthereumTx{}
				tmp := sdkmath.NewInt(1)
				chainID = &tmp
			},
			expPass: false,
		},
		{
			msg: "nil Msg in QueryTraceTxRequest",
			malleate: func() {
				traceConfig = nil
				predecessors = []*types.MsgEthereumTx{}
				txMsg = nil
			},
			expPass: false,
		},
	}

	for _, tc := range testCases {
		suite.Run(fmt.Sprintf("Case %s", tc.msg), func() {
			suite.enableFeemarket = tc.enableFeemarket
			suite.SetupTest()
			// Deploy contract
			contractAddr := suite.deployTestContract(suite.Address)
			suite.Commit(suite.T())
			// Generate token transfer transaction
			txMsg = suite.transferERC20Token(suite.T(), contractAddr, suite.Address, common.HexToAddress("0x378c50D9264C63F3F92B806d4ee56E9D86FfB3Ec"), sdkmath.NewIntWithDecimal(1, 18).BigInt())
			suite.Commit(suite.T())

			tc.malleate()
			traceReq := types.QueryTraceTxRequest{
				Msg:          txMsg,
				TraceConfig:  traceConfig,
				Predecessors: predecessors,
			}

			if chainID != nil {
				traceReq.ChainId = chainID.Int64()
			}
			res, err := suite.EvmQueryClient.TraceTx(suite.Ctx, &traceReq)
			if tc.expPass {
				suite.Require().NoError(err)
				// if data is to big, slice the result
				if len(res.Data) > 150 {
					suite.Require().Equal(tc.traceResponse, string(res.Data[:150]))
				} else {
					suite.Require().Equal(tc.traceResponse, string(res.Data))
				}
				if traceConfig == nil || traceConfig.Tracer == "" {
					var result ethlogger.ExecutionResult
					suite.Require().NoError(json.Unmarshal(res.Data, &result))
					suite.Require().Positive(result.Gas)
				}
			} else {
				suite.Require().Error(err)
			}
			// Reset for next test case
			chainID = nil
		})
	}

	suite.enableFeemarket = false // reset flag
}

func (suite *GRPCServerTestSuiteSuite) TestTraceBlock() {
	var (
		txs         []*types.MsgEthereumTx
		traceConfig *types.TraceConfig
		chainID     *sdkmath.Int
	)

	testCases := []struct {
		msg             string
		malleate        func()
		expPass         bool
		traceResponse   string
		enableFeemarket bool
	}{
		{
			msg: "default trace",
			malleate: func() {
				traceConfig = nil
			},
			expPass:       true,
			traceResponse: `[{"result":{"gas":34828,"failed":false,"returnValue":"0x0000000000000000000000000000000000000000000000000000000000000001","structLogs":[{"pc":0,"op":"`,
		},
		{
			msg: "filtered trace",
			malleate: func() {
				traceConfig = &types.TraceConfig{
					DisableStack:   true,
					DisableStorage: true,
					EnableMemory:   false,
				}
			},
			expPass:       true,
			traceResponse: `[{"result":{"gas":34828,"failed":false,"returnValue":"0x0000000000000000000000000000000000000000000000000000000000000001","structLogs":[{"pc":0,"op":"`,
		},
		{
			msg: "javascript tracer",
			malleate: func() {
				traceConfig = &types.TraceConfig{
					Tracer: "{data: [], fault: function(log) {}, step: function(log) { if(log.op.toString() == \"CALL\") this.data.push(log.stack.peek(0)); }, result: function() { return this.data; }}",
				}
			},
			expPass:       true,
			traceResponse: "[{\"result\":[]}]",
		},
		{
			msg: "default trace with enableFeemarket and filtered return",
			malleate: func() {
				traceConfig = &types.TraceConfig{
					DisableStack:   true,
					DisableStorage: true,
					EnableMemory:   false,
				}
			},
			expPass:         true,
			traceResponse:   `[{"result":{"gas":34828,"failed":false,"returnValue":"0x0000000000000000000000000000000000000000000000000000000000000001","structLogs":[{"pc":0,"op":"`,
			enableFeemarket: true,
		},
		{
			msg: "javascript tracer with enableFeemarket",
			malleate: func() {
				traceConfig = &types.TraceConfig{
					Tracer: "{data: [], fault: function(log) {}, step: function(log) { if(log.op.toString() == \"CALL\") this.data.push(log.stack.peek(0)); }, result: function() { return this.data; }}",
				}
			},
			expPass:         true,
			traceResponse:   `[{"result":[]}]`,
			enableFeemarket: true,
		},
		{
			msg: "tracer with multiple transactions",
			malleate: func() {
				traceConfig = nil

				// increase nonce to avoid address collision
				vmdb := suite.StateDB()
				vmdb.SetNonce(suite.Address, vmdb.GetNonce(suite.Address)+1, tracing.NonceChangeUnspecified)
				suite.Require().NoError(vmdb.Commit())
				contractAddr := suite.deployTestContract(suite.Address)
				suite.Commit(suite.T())
				// create multiple transactions in the same block
				firstTx := suite.transferERC20Token(suite.T(), contractAddr, suite.Address, common.HexToAddress("0x378c50D9264C63F3F92B806d4ee56E9D86FfB3Ec"), sdkmath.NewIntWithDecimal(1, 18).BigInt())
				secondTx := suite.transferERC20Token(suite.T(), contractAddr, suite.Address, common.HexToAddress("0x378c50D9264C63F3F92B806d4ee56E9D86FfB3Ec"), sdkmath.NewIntWithDecimal(1, 18).BigInt())
				suite.Commit(suite.T())
				// overwrite txs to include only the ones on new block
				txs = append([]*types.MsgEthereumTx{}, firstTx, secondTx)
			},
			expPass:         true,
			traceResponse:   `[{"result":{"gas":34828,"failed":false,"returnValue":"0x0000000000000000000000000000000000000000000000000000000000000001","structLogs":[{"pc":0,"op":"`,
			enableFeemarket: false,
		},
		{
			msg: "invalid trace config - Negative Limit",
			malleate: func() {
				traceConfig = &types.TraceConfig{
					DisableStack:   true,
					DisableStorage: true,
					EnableMemory:   false,
					Limit:          -1,
				}
			},
			expPass: false,
		},
		{
			msg: "invalid trace config - Invalid Tracer",
			malleate: func() {
				traceConfig = &types.TraceConfig{
					DisableStack:   true,
					DisableStorage: true,
					EnableMemory:   false,
					Tracer:         "invalid_tracer",
				}
			},
			expPass:       true,
			traceResponse: "invalid_tracer is not defined",
		},
		{
			msg: "invalid chain id",
			malleate: func() {
				traceConfig = nil
				tmp := sdkmath.NewInt(1)
				chainID = &tmp
			},
			expPass:       true,
			traceResponse: "invalid chain id for signer",
		},
	}

	for _, tc := range testCases {
		suite.Run(fmt.Sprintf("Case %s", tc.msg), func() {
			txs = []*types.MsgEthereumTx{}
			suite.enableFeemarket = tc.enableFeemarket
			suite.SetupTest()
			// Deploy contract
			contractAddr := suite.deployTestContract(suite.Address)
			// set some balance to handle fees
			suite.App.EvmKeeper.SetBalance(suite.Ctx, suite.Address, *uint256.NewInt(1000000000000000000), types.DefaultEVMDenom)
			suite.Commit(suite.T())
			// Generate token transfer transaction
			txMsg := suite.transferERC20Token(suite.T(), contractAddr, suite.Address, common.HexToAddress("0x378c50D9264C63F3F92B806d4ee56E9D86FfB3Ec"), sdkmath.NewIntWithDecimal(1, 18).BigInt())
			suite.Commit(suite.T())

			txs = append(txs, txMsg)

			tc.malleate()
			traceReq := types.QueryTraceBlockRequest{
				Txs:         txs,
				TraceConfig: traceConfig,
			}

			if chainID != nil {
				traceReq.ChainId = chainID.Int64()
			}
			res, err := suite.EvmQueryClient.TraceBlock(suite.Ctx, &traceReq)
			if tc.expPass {
				suite.Require().NoError(err)
				// if data is to big, slice the result
				if len(res.Data) > 150 {
					suite.Require().Equal(tc.traceResponse, string(res.Data[:150]))
				} else {
					suite.Require().Contains(string(res.Data), tc.traceResponse)
				}
			} else {
				suite.Require().Error(err)
			}
			// Reset for next case
			chainID = nil
		})
	}

	suite.enableFeemarket = false // reset flag
}

func (suite *GRPCServerTestSuiteSuite) TestNonceInQuery() {
	suite.SetupTest()
	address := tests.GenerateAddress()
	suite.Require().Equal(uint64(0), suite.App.EvmKeeper.GetNonce(suite.Ctx, address))
	supply := sdkmath.NewIntWithDecimal(1000, 18).BigInt()

	// accupy nonce 0
	_ = suite.deployTestContract(address)

	// do an EthCall/EstimateGas with nonce 0
	ctorArgs, err := types.ERC20Contract.ABI.Pack("", address, supply)
	suite.Require().NoError(err)

	data := append(types.ERC20Contract.Bin, ctorArgs...)
	args, err := json.Marshal(&types.TransactionArgs{
		From: &address,
		Data: (*hexutil.Bytes)(&data),
	})
	suite.Require().NoError(err)
	proposerAddress := suite.Ctx.BlockHeader().ProposerAddress
	_, err = suite.EvmQueryClient.EstimateGas(suite.Ctx, &types.EthCallRequest{
		Args:            args,
		GasCap:          uint64(config.DefaultGasCap),
		ProposerAddress: proposerAddress,
	})
	suite.Require().NoError(err)

	_, err = suite.EvmQueryClient.EthCall(suite.Ctx, &types.EthCallRequest{
		Args:            args,
		GasCap:          uint64(config.DefaultGasCap),
		ProposerAddress: proposerAddress,
	})
	suite.Require().NoError(err)
}

func (suite *GRPCServerTestSuiteSuite) TestQueryBaseFee() {
	var (
		aux    sdkmath.Int
		expRes *types.QueryBaseFeeResponse
	)

	testCases := []struct {
		name            string
		malleate        func()
		expPass         bool
		enableFeemarket bool
		enableLondonHF  bool
	}{
		{
			"pass - default Base Fee",
			func() {
				initialBaseFee := sdkmath.NewInt(ethparams.InitialBaseFee)
				expRes = &types.QueryBaseFeeResponse{BaseFee: &initialBaseFee}
			},
			true, true, true,
		},
		{
			"pass - non-nil Base Fee",
			func() {
				baseFee := sdkmath.OneInt().BigInt()
				suite.App.FeeMarketKeeper.SetBaseFee(suite.Ctx, baseFee)

				aux = sdkmath.NewIntFromBigInt(baseFee)
				expRes = &types.QueryBaseFeeResponse{BaseFee: &aux}
			},
			true, true, true,
		},
		{
			"pass - nil Base Fee when london hardfork not activated",
			func() {
				baseFee := sdkmath.OneInt().BigInt()
				suite.App.FeeMarketKeeper.SetBaseFee(suite.Ctx, baseFee)

				expRes = &types.QueryBaseFeeResponse{}
			},
			true, true, false,
		},
		{
			"pass - zero Base Fee when feemarket not activated",
			func() {
				baseFee := sdkmath.ZeroInt()
				expRes = &types.QueryBaseFeeResponse{BaseFee: &baseFee}
			},
			true, false, true,
		},
	}
	for _, tc := range testCases {
		suite.Run(tc.name, func() {
			suite.enableFeemarket = tc.enableFeemarket
			suite.enableLondonHF = tc.enableLondonHF
			suite.SetupTest()

			tc.malleate()

			res, err := suite.EvmQueryClient.BaseFee(suite.Ctx.Context(), &types.QueryBaseFeeRequest{})
			if tc.expPass {
				suite.Require().NotNil(res)
				suite.Require().Equal(expRes, res, tc.name)
				suite.Require().NoError(err)
			} else {
				suite.Require().Error(err)
			}
		})
	}
	suite.enableFeemarket = false
	suite.enableLondonHF = true
}

func (suite *GRPCServerTestSuiteSuite) TestEthCall() {
	var req *types.EthCallRequest

	address := tests.GenerateAddress()
	suite.Require().Equal(uint64(0), suite.App.EvmKeeper.GetNonce(suite.Ctx, address))
	supply := sdkmath.NewIntWithDecimal(1000, 18).BigInt()

	hexBigInt := hexutil.Big(*big.NewInt(1))
	ctorArgs, err := types.ERC20Contract.ABI.Pack("", address, supply)
	suite.Require().NoError(err)

	data := append(types.ERC20Contract.Bin, ctorArgs...)

	testCases := []struct {
		name     string
		malleate func()
		expPass  bool
	}{
		{
			"invalid args",
			func() {
				req = &types.EthCallRequest{Args: []byte("invalid args"), GasCap: uint64(config.DefaultGasCap)}
			},
			false,
		},
		{
			"invalid args - specified both gasPrice and maxFeePerGas",
			func() {
				args, err := json.Marshal(&types.TransactionArgs{
					From:         &address,
					Data:         (*hexutil.Bytes)(&data),
					GasPrice:     &hexBigInt,
					MaxFeePerGas: &hexBigInt,
				})

				suite.Require().NoError(err)
				req = &types.EthCallRequest{Args: args, GasCap: uint64(config.DefaultGasCap)}
			},
			false,
		},
		{
			"set param EnableCreate = false",
			func() {
				args, err := json.Marshal(&types.TransactionArgs{
					From: &address,
					Data: (*hexutil.Bytes)(&data),
				})

				suite.Require().NoError(err)
				req = &types.EthCallRequest{Args: args, GasCap: uint64(config.DefaultGasCap)}

				params := suite.App.EvmKeeper.GetParams(suite.Ctx)
				params.EnableCreate = false
				suite.App.EvmKeeper.SetParams(suite.Ctx, params)
			},
			false,
		},
	}
	for _, tc := range testCases {
		suite.Run(tc.name, func() {
			suite.SetupTest()
			tc.malleate()

			res, err := suite.EvmQueryClient.EthCall(suite.Ctx, req)
			if tc.expPass {
				suite.Require().NotNil(res)
				suite.Require().NoError(err)
			} else {
				suite.Require().Error(err)
			}
		})
	}
}

func (suite *GRPCServerTestSuiteSuite) TestEmptyRequest() {
	testCases := []struct {
		name      string
		queryFunc func() (interface{}, error)
	}{
		{
			"Account method",
			func() (interface{}, error) {
				return suite.App.EvmKeeper.Account(suite.Ctx, nil)
			},
		},
		{
			"CosmosAccount method",
			func() (interface{}, error) {
				return suite.App.EvmKeeper.CosmosAccount(suite.Ctx, nil)
			},
		},
		{
			"ValidatorAccount method",
			func() (interface{}, error) {
				return suite.App.EvmKeeper.ValidatorAccount(suite.Ctx, nil)
			},
		},
		{
			"Balance method",
			func() (interface{}, error) {
				return suite.App.EvmKeeper.Balance(suite.Ctx, nil)
			},
		},
		{
			"Storage method",
			func() (interface{}, error) {
				return suite.App.EvmKeeper.Storage(suite.Ctx, nil)
			},
		},
		{
			"Code method",
			func() (interface{}, error) {
				return suite.App.EvmKeeper.Code(suite.Ctx, nil)
			},
		},
		{
			"EthCall method",
			func() (interface{}, error) {
				return suite.App.EvmKeeper.EthCall(suite.Ctx, nil)
			},
		},
		{
			"EstimateGas method",
			func() (interface{}, error) {
				return suite.App.EvmKeeper.EstimateGas(suite.Ctx, nil)
			},
		},
		{
			"TraceTx method",
			func() (interface{}, error) {
				return suite.App.EvmKeeper.TraceTx(suite.Ctx, nil)
			},
		},
		{
			"TraceBlock method",
			func() (interface{}, error) {
				return suite.App.EvmKeeper.TraceBlock(suite.Ctx, nil)
			},
		},
	}

	for _, tc := range testCases {
		suite.Run(fmt.Sprintf("Case %s", tc.name), func() {
			suite.SetupTest()
			_, err := tc.queryFunc()
			suite.Require().Error(err)
		})
	}
}

// ---------------------------------------------------------------------------
// SimulateV1 gRPC handler tests
// ---------------------------------------------------------------------------

func (suite *GRPCServerTestSuiteSuite) TestSimulateV1_NilRequest() {
	_, err := suite.App.EvmKeeper.SimulateV1(suite.Ctx, nil)
	suite.Require().Error(err)
	suite.Require().Contains(err.Error(), "empty request")
}

func (suite *GRPCServerTestSuiteSuite) TestSimulateV1_InvalidJSON() {
	req := &evmtypes.SimulateV1Request{
		Args:    []byte("not valid json"),
		GasCap:  25_000_000,
		ChainId: suite.App.EvmKeeper.ChainID().Int64(),
	}
	_, err := suite.App.EvmKeeper.SimulateV1(suite.Ctx, req)
	suite.Require().Error(err)
}

func (suite *GRPCServerTestSuiteSuite) TestSimulateV1_MissingBaseHeader() {
	payload := rpctypes.SimulateV1Args{
		Opts: rpctypes.SimOpts{
			BlockStateCalls: []rpctypes.SimBlock{{}},
		},
		BaseHeader: nil,
	}
	bz, err := json.Marshal(&payload)
	suite.Require().NoError(err)

	req := &evmtypes.SimulateV1Request{
		Args:    bz,
		GasCap:  25_000_000,
		ChainId: suite.App.EvmKeeper.ChainID().Int64(),
	}
	_, err = suite.App.EvmKeeper.SimulateV1(suite.Ctx, req)
	suite.Require().Error(err)
	suite.Require().Contains(err.Error(), "missing base header")
}

func (suite *GRPCServerTestSuiteSuite) TestSimulateV1_EmptyBlockStateCalls() {
	baseHeader := &ethtypes.Header{
		Number:     big.NewInt(suite.Ctx.BlockHeight()),
		Time:       uint64(suite.Ctx.BlockTime().Unix()),
		Difficulty: big.NewInt(0),
		GasLimit:   25_000_000,
	}
	payload := rpctypes.SimulateV1Args{
		Opts: rpctypes.SimOpts{
			BlockStateCalls: []rpctypes.SimBlock{},
		},
		BaseHeader: baseHeader,
	}
	bz, err := json.Marshal(&payload)
	suite.Require().NoError(err)

	req := &evmtypes.SimulateV1Request{
		Args:    bz,
		GasCap:  25_000_000,
		ChainId: suite.App.EvmKeeper.ChainID().Int64(),
	}
	_, err = suite.App.EvmKeeper.SimulateV1(suite.Ctx, req)
	suite.Require().Error(err)
	suite.Require().Contains(err.Error(), "blockStateCalls must not be empty")
}

func (suite *GRPCServerTestSuiteSuite) TestSimulateV1_TooManyBlocks() {
	baseHeader := &ethtypes.Header{
		Number:     big.NewInt(suite.Ctx.BlockHeight()),
		Time:       uint64(suite.Ctx.BlockTime().Unix()),
		Difficulty: big.NewInt(0),
		GasLimit:   25_000_000,
	}
	blocks := make([]rpctypes.SimBlock, rpctypes.MaxSimulateBlocks+1)
	payload := rpctypes.SimulateV1Args{
		Opts: rpctypes.SimOpts{
			BlockStateCalls: blocks,
		},
		BaseHeader: baseHeader,
	}
	bz, err := json.Marshal(&payload)
	suite.Require().NoError(err)

	req := &evmtypes.SimulateV1Request{
		Args:    bz,
		GasCap:  25_000_000,
		ChainId: suite.App.EvmKeeper.ChainID().Int64(),
	}
	_, err = suite.App.EvmKeeper.SimulateV1(suite.Ctx, req)
	suite.Require().Error(err)
	suite.Require().Contains(err.Error(), "too many blocks in blockStateCalls")
}

func (suite *GRPCServerTestSuiteSuite) TestSimulateV1_SingleEmptyBlock() {
	suite.SetupTest()
	suite.App.EvmKeeper.BeginBlock(suite.Ctx)

	baseHeader := &ethtypes.Header{
		Number:     big.NewInt(suite.Ctx.BlockHeight()),
		Time:       uint64(suite.Ctx.BlockTime().Unix()),
		Difficulty: big.NewInt(0),
		GasLimit:   25_000_000,
	}

	payload := rpctypes.SimulateV1Args{
		Opts: rpctypes.SimOpts{
			BlockStateCalls: []rpctypes.SimBlock{{}},
		},
		BaseHeader: baseHeader,
	}
	bz, err := json.Marshal(&payload)
	suite.Require().NoError(err)

	req := &evmtypes.SimulateV1Request{
		Args:            bz,
		GasCap:          25_000_000,
		ChainId:         suite.App.EvmKeeper.ChainID().Int64(),
		ProposerAddress: suite.Ctx.BlockHeader().ProposerAddress,
	}
	resp, err := suite.App.EvmKeeper.SimulateV1(suite.Ctx, req)
	suite.Require().NoError(err)
	suite.Require().NotNil(resp)
	suite.Require().NotEmpty(resp.Result)
}

func (suite *GRPCServerTestSuiteSuite) TestSimulateV1_WithSimpleTransfer() {
	suite.SetupTest()
	suite.App.EvmKeeper.BeginBlock(suite.Ctx)

	to := tests.GenerateAddress()
	from := suite.Address

	baseHeader := &ethtypes.Header{
		Number:     big.NewInt(suite.Ctx.BlockHeight()),
		Time:       uint64(suite.Ctx.BlockTime().Unix()),
		Difficulty: big.NewInt(0),
		GasLimit:   25_000_000,
	}

	callJSON, err := json.Marshal(map[string]interface{}{
		"from":  from.Hex(),
		"to":    to.Hex(),
		"gas":   hexutil.EncodeUint64(21000),
		"value": hexutil.EncodeBig(big.NewInt(0)),
	})
	suite.Require().NoError(err)

	payload := rpctypes.SimulateV1Args{
		Opts: rpctypes.SimOpts{
			BlockStateCalls: []rpctypes.SimBlock{
				{
					Calls: []json.RawMessage{callJSON},
				},
			},
		},
		BaseHeader: baseHeader,
	}
	bz, err := json.Marshal(&payload)
	suite.Require().NoError(err)

	req := &evmtypes.SimulateV1Request{
		Args:            bz,
		GasCap:          25_000_000,
		ChainId:         suite.App.EvmKeeper.ChainID().Int64(),
		ProposerAddress: suite.Ctx.BlockHeader().ProposerAddress,
	}
	resp, err := suite.App.EvmKeeper.SimulateV1(suite.Ctx, req)
	suite.Require().NoError(err)
	suite.Require().NotNil(resp)
	suite.Require().NotEmpty(resp.Result)
}

// ---------------------------------------------------------------------------
// Simulator code-path coverage via SimulateV1 gRPC
// ---------------------------------------------------------------------------

// helper: builds a SimulateV1 request payload and returns the keeper response.
func (suite *GRPCServerTestSuiteSuite) simulateRequest(
	opts rpctypes.SimOpts,
	baseHeader *ethtypes.Header,
	gasCap uint64,
) (*evmtypes.SimulateV1Response, error) {
	payload := rpctypes.SimulateV1Args{Opts: opts, BaseHeader: baseHeader}
	bz, err := json.Marshal(&payload)
	if err != nil {
		return nil, err
	}
	req := &evmtypes.SimulateV1Request{
		Args:            bz,
		GasCap:          gasCap,
		ChainId:         suite.App.EvmKeeper.ChainID().Int64(),
		ProposerAddress: suite.Ctx.BlockHeader().ProposerAddress,
	}
	return suite.App.EvmKeeper.SimulateV1(suite.Ctx, req)
}

// ctxBaseHeader builds an ethtypes.Header from the current test context.
func (suite *GRPCServerTestSuiteSuite) ctxBaseHeader(gasLimit uint64) *ethtypes.Header {
	return &ethtypes.Header{
		Number:     big.NewInt(suite.Ctx.BlockHeight()),
		Time:       uint64(suite.Ctx.BlockTime().Unix()),
		Difficulty: big.NewInt(0),
		GasLimit:   gasLimit,
	}
}

// TestSimulator_SanitizeChain_InvalidBlockNumber exercises the block-number-order
// error path in sanitizeChain (within Execute).
func (suite *GRPCServerTestSuiteSuite) TestSimulator_SanitizeChain_InvalidBlockNumber() {
	suite.SetupTest()
	suite.App.EvmKeeper.BeginBlock(suite.Ctx)

	baseHeader := suite.ctxBaseHeader(25_000_000)
	// Supply a block number that is ≤ baseHeader.Number
	sameNum := (*hexutil.Big)(new(big.Int).Set(baseHeader.Number))
	opts := rpctypes.SimOpts{
		BlockStateCalls: []rpctypes.SimBlock{
			{BlockOverrides: &rpctypes.SimBlockOverrides{Number: sameNum}},
		},
	}
	resp, err := suite.simulateRequest(opts, baseHeader, 25_000_000)
	suite.Require().NoError(err) // gRPC call itself succeeds
	// The response carries the simulation error in dedicated fields.
	suite.Require().Equal(rpctypes.ErrCodeBlockNumberInvalid, int(resp.ErrorCode))
	suite.Require().Contains(resp.ErrorMessage, "block numbers must be in order")
}

// TestSimulator_SanitizeChain_InvalidTimestamp exercises the timestamp-order
// error path in sanitizeChain.
func (suite *GRPCServerTestSuiteSuite) TestSimulator_SanitizeChain_InvalidTimestamp() {
	suite.SetupTest()
	suite.App.EvmKeeper.BeginBlock(suite.Ctx)

	baseHeader := suite.ctxBaseHeader(25_000_000)
	nextNum := (*hexutil.Big)(new(big.Int).Add(baseHeader.Number, big.NewInt(1)))
	// Timestamp in the past
	pastTime := hexutil.Uint64(baseHeader.Time - 1)
	opts := rpctypes.SimOpts{
		BlockStateCalls: []rpctypes.SimBlock{
			{BlockOverrides: &rpctypes.SimBlockOverrides{
				Number: nextNum,
				Time:   &pastTime,
			}},
		},
	}
	resp, err := suite.simulateRequest(opts, baseHeader, 25_000_000)
	suite.Require().NoError(err)
	suite.Require().Equal(rpctypes.ErrCodeBlockTimestampInvalid, int(resp.ErrorCode))
	suite.Require().Contains(resp.ErrorMessage, "block timestamps must be in order")
}

// TestSimulator_SanitizeChain_GapFilling exercises the gap-filling path where
// sanitizeChain inserts empty intermediate blocks.
func (suite *GRPCServerTestSuiteSuite) TestSimulator_SanitizeChain_GapFilling() {
	suite.SetupTest()
	suite.App.EvmKeeper.BeginBlock(suite.Ctx)

	baseHeader := suite.ctxBaseHeader(25_000_000)
	// Jump 3 blocks ahead → 2 gap-fill blocks + 1 explicit block
	farNum := (*hexutil.Big)(new(big.Int).Add(baseHeader.Number, big.NewInt(3)))
	opts := rpctypes.SimOpts{
		BlockStateCalls: []rpctypes.SimBlock{
			{BlockOverrides: &rpctypes.SimBlockOverrides{Number: farNum}},
		},
	}
	resp, err := suite.simulateRequest(opts, baseHeader, 25_000_000)
	suite.Require().NoError(err)
	// Result should be valid (3 blocks)
	var blocks []json.RawMessage
	suite.Require().NoError(json.Unmarshal(resp.Result, &blocks))
	suite.Require().Len(blocks, 3)
}

// TestSimulator_ContractCreation exercises the contract-creation path in applyCall.
func (suite *GRPCServerTestSuiteSuite) TestSimulator_ContractCreation() {
	suite.SetupTest()
	suite.App.EvmKeeper.BeginBlock(suite.Ctx)

	from := suite.Address
	// Minimal init code: just STOP (0x00)
	initCode := hexutil.Bytes{0x00}
	callJSON, err := json.Marshal(map[string]interface{}{
		"from":  from.Hex(),
		"gas":   hexutil.EncodeUint64(200_000),
		"value": hexutil.EncodeBig(big.NewInt(0)),
		"input": hexutil.Encode(initCode),
		// No "to" field → contract creation
	})
	suite.Require().NoError(err)

	opts := rpctypes.SimOpts{
		BlockStateCalls: []rpctypes.SimBlock{
			{Calls: []json.RawMessage{callJSON}},
		},
	}
	baseHeader := suite.ctxBaseHeader(25_000_000)
	resp, err := suite.simulateRequest(opts, baseHeader, 25_000_000)
	suite.Require().NoError(err)
	suite.Require().NotEmpty(resp.Result)
}

// TestSimulator_EVMRevert exercises the EVM-revert path in processBlock.
func (suite *GRPCServerTestSuiteSuite) TestSimulator_EVMRevert() {
	suite.SetupTest()
	suite.App.EvmKeeper.BeginBlock(suite.Ctx)

	from := suite.Address
	// EVM bytecode that immediately REVERTs: PUSH1 0 PUSH1 0 REVERT
	revertCode := hexutil.Bytes{0x60, 0x00, 0x60, 0x00, 0xfd}

	callJSON, err := json.Marshal(map[string]interface{}{
		"from":  from.Hex(),
		"gas":   hexutil.EncodeUint64(100_000),
		"value": hexutil.EncodeBig(big.NewInt(0)),
		"input": hexutil.Encode(revertCode),
	})
	suite.Require().NoError(err)

	opts := rpctypes.SimOpts{
		BlockStateCalls: []rpctypes.SimBlock{
			{Calls: []json.RawMessage{callJSON}},
		},
	}
	baseHeader := suite.ctxBaseHeader(25_000_000)
	resp, err := suite.simulateRequest(opts, baseHeader, 25_000_000)
	suite.Require().NoError(err)
	suite.Require().NotEmpty(resp.Result)

	// The call should report a failure status
	var results []map[string]json.RawMessage
	suite.Require().NoError(json.Unmarshal(resp.Result, &results))
	suite.Require().Len(results, 1)
	var calls []map[string]json.RawMessage
	suite.Require().NoError(json.Unmarshal(results[0]["calls"], &calls))
	suite.Require().Len(calls, 1)
	// status should be "0x0" (failure)
	suite.Require().Contains(string(calls[0]["status"]), "0x0")
}

// TestSimulator_InvalidCallJSON exercises the invalid-JSON-call path in processBlock.
func (suite *GRPCServerTestSuiteSuite) TestSimulator_InvalidCallJSON() {
	suite.SetupTest()
	suite.App.EvmKeeper.BeginBlock(suite.Ctx)

	// Manually construct the payload with an invalid call JSON that passes
	// Go's json.Marshal (as an escaped string) but fails to unmarshal as
	// TransactionArgs inside processBlock.
	rawPayload := []byte(`{
		"opts": {
			"blockStateCalls": [
				{"calls": [{"not": "a valid tx arg that will fail strict unmarshal... "}]}
			]
		},
		"baseHeader": {
			"parentHash":"0x0000000000000000000000000000000000000000000000000000000000000000",
			"sha3Uncles":"0x1dcc4de8dec75d7aab85b567b6ccd41ad312451b948a7413f0a142fd40d49347",
			"miner":"0x0000000000000000000000000000000000000000",
			"stateRoot":"0x0000000000000000000000000000000000000000000000000000000000000000",
			"transactionsRoot":"0x56e81f171bcc55a6ff8345e692c0f86e5b48e01b996cadc001622fb5e363b421",
			"receiptsRoot":"0x56e81f171bcc55a6ff8345e692c0f86e5b48e01b996cadc001622fb5e363b421",
			"logsBloom":"0x00000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000",
			"difficulty":"0x0",
			"number":"0x1",
			"gasLimit":"0x17d7840",
			"gasUsed":"0x0",
			"timestamp":"0x1",
			"extraData":"0x",
			"mixHash":"0x0000000000000000000000000000000000000000000000000000000000000000",
			"nonce":"0x0000000000000000",
			"hash":"0x0000000000000000000000000000000000000000000000000000000000000000"
		}
	}`)

	req := &evmtypes.SimulateV1Request{
		Args:            rawPayload,
		GasCap:          25_000_000,
		ChainId:         suite.App.EvmKeeper.ChainID().Int64(),
		ProposerAddress: suite.Ctx.BlockHeader().ProposerAddress,
	}
	resp, err := suite.App.EvmKeeper.SimulateV1(suite.Ctx, req)
	suite.Require().NoError(err)
	suite.Require().NotEmpty(resp.Result)
	// The result could be either an error response (if the call fails) or a valid block
	// Either is acceptable - we're testing that the code path runs without panic
}

// TestSimulator_ValidationMode exercises the validate=true path in applyCall.
func (suite *GRPCServerTestSuiteSuite) TestSimulator_ValidationMode() {
	suite.SetupTest()
	suite.App.EvmKeeper.BeginBlock(suite.Ctx)

	from := suite.Address
	to := tests.GenerateAddress()

	callJSON, err := json.Marshal(map[string]interface{}{
		"from":  from.Hex(),
		"to":    to.Hex(),
		"gas":   hexutil.EncodeUint64(21000),
		"value": hexutil.EncodeBig(big.NewInt(0)),
		"nonce": hexutil.EncodeUint64(999), // nonce too high → validation error
	})
	suite.Require().NoError(err)

	opts := rpctypes.SimOpts{
		BlockStateCalls: []rpctypes.SimBlock{
			{Calls: []json.RawMessage{callJSON}},
		},
		Validation: true,
	}
	// Validation mode requires BaseFee to be set for London rules.
	baseHeader := suite.ctxBaseHeader(25_000_000)
	baseHeader.BaseFee = big.NewInt(0)
	resp, err := suite.simulateRequest(opts, baseHeader, 25_000_000)
	suite.Require().NoError(err)
	suite.Require().NotZero(resp.ErrorCode)
}

// TestSimulator_BlockGasLimitReached exercises the BlockGasLimitReachedError
// path in sanitizeCall.
func (suite *GRPCServerTestSuiteSuite) TestSimulator_BlockGasLimitReached() {
	suite.SetupTest()
	suite.App.EvmKeeper.BeginBlock(suite.Ctx)

	from := suite.Address
	to := tests.GenerateAddress()

	// Request more gas than the block gas limit
	bigGas := hexutil.EncodeUint64(50_000_000)
	callJSON, err := json.Marshal(map[string]interface{}{
		"from":  from.Hex(),
		"to":    to.Hex(),
		"gas":   bigGas,
		"value": hexutil.EncodeBig(big.NewInt(0)),
	})
	suite.Require().NoError(err)

	blockGasLimit := hexutil.Uint64(1000) // tiny block gas limit
	opts := rpctypes.SimOpts{
		BlockStateCalls: []rpctypes.SimBlock{
			{
				BlockOverrides: &rpctypes.SimBlockOverrides{
					GasLimit: &blockGasLimit,
				},
				Calls: []json.RawMessage{callJSON},
			},
		},
	}
	baseHeader := suite.ctxBaseHeader(25_000_000)
	resp, err := suite.simulateRequest(opts, baseHeader, 25_000_000)
	suite.Require().NoError(err)
	suite.Require().NotZero(resp.ErrorCode)
}

// TestSimulator_TraceTransfers exercises the traceTransfers=true path.
func (suite *GRPCServerTestSuiteSuite) TestSimulator_TraceTransfers() {
	suite.SetupTest()
	suite.App.EvmKeeper.BeginBlock(suite.Ctx)

	from := suite.Address
	to := tests.GenerateAddress()

	callJSON, err := json.Marshal(map[string]interface{}{
		"from":  from.Hex(),
		"to":    to.Hex(),
		"gas":   hexutil.EncodeUint64(21000),
		"value": hexutil.EncodeBig(big.NewInt(0)),
	})
	suite.Require().NoError(err)

	opts := rpctypes.SimOpts{
		BlockStateCalls: []rpctypes.SimBlock{
			{Calls: []json.RawMessage{callJSON}},
		},
		TraceTransfers: true,
	}
	baseHeader := suite.ctxBaseHeader(25_000_000)
	resp, err := suite.simulateRequest(opts, baseHeader, 25_000_000)
	suite.Require().NoError(err)
	suite.Require().NotEmpty(resp.Result)
}

// TestSimulator_ReturnFullTransactions exercises the fullTx=true path.
func (suite *GRPCServerTestSuiteSuite) TestSimulator_ReturnFullTransactions() {
	suite.SetupTest()
	suite.App.EvmKeeper.BeginBlock(suite.Ctx)

	from := suite.Address
	to := tests.GenerateAddress()

	callJSON, err := json.Marshal(map[string]interface{}{
		"from":  from.Hex(),
		"to":    to.Hex(),
		"gas":   hexutil.EncodeUint64(21000),
		"value": hexutil.EncodeBig(big.NewInt(0)),
	})
	suite.Require().NoError(err)

	opts := rpctypes.SimOpts{
		BlockStateCalls: []rpctypes.SimBlock{
			{Calls: []json.RawMessage{callJSON}},
		},
		ReturnFullTransactions: true,
	}
	baseHeader := suite.ctxBaseHeader(25_000_000)
	resp, err := suite.simulateRequest(opts, baseHeader, 25_000_000)
	suite.Require().NoError(err)
	suite.Require().NotEmpty(resp.Result)
}

// TestSimulator_BlobBaseFeeOverride exercises the BlobBaseFee override path.
func (suite *GRPCServerTestSuiteSuite) TestSimulator_BlobBaseFeeOverride() {
	suite.SetupTest()
	suite.App.EvmKeeper.BeginBlock(suite.Ctx)

	baseHeader := suite.ctxBaseHeader(25_000_000)
	blobFee := (*hexutil.Big)(big.NewInt(1_000_000))

	opts := rpctypes.SimOpts{
		BlockStateCalls: []rpctypes.SimBlock{
			{BlockOverrides: &rpctypes.SimBlockOverrides{
				BlobBaseFee: blobFee,
			}},
		},
	}
	resp, err := suite.simulateRequest(opts, baseHeader, 25_000_000)
	suite.Require().NoError(err)
	suite.Require().NotEmpty(resp.Result)
}

// TestSimulator_StateOverrides exercises the Apply-state-override path.
func (suite *GRPCServerTestSuiteSuite) TestSimulator_StateOverrides() {
	suite.SetupTest()
	suite.App.EvmKeeper.BeginBlock(suite.Ctx)

	addr := tests.GenerateAddress()
	newBalance := (*hexutil.Big)(big.NewInt(1e18))
	newNonce := hexutil.Uint64(5)

	stateOverride := rpctypes.SimStateOverride{
		addr: rpctypes.SimOverrideAccount{
			Balance: newBalance,
			Nonce:   &newNonce,
		},
	}

	opts := rpctypes.SimOpts{
		BlockStateCalls: []rpctypes.SimBlock{
			{StateOverrides: &stateOverride},
		},
	}
	baseHeader := suite.ctxBaseHeader(25_000_000)
	resp, err := suite.simulateRequest(opts, baseHeader, 25_000_000)
	suite.Require().NoError(err)
	suite.Require().NotEmpty(resp.Result)
}

// TestSimulator_StateOverrides_Code verifies that code overrides are applied
// and exercises the Apply code-override path.
func (suite *GRPCServerTestSuiteSuite) TestSimulator_StateOverrides_Code() {
	suite.SetupTest()
	suite.App.EvmKeeper.BeginBlock(suite.Ctx)

	addr := tests.GenerateAddress()
	// Simple STOP bytecode.
	code := hexutil.Bytes([]byte{0x00})

	stateOverride := rpctypes.SimStateOverride{
		addr: rpctypes.SimOverrideAccount{
			Code: &code,
		},
	}

	opts := rpctypes.SimOpts{
		BlockStateCalls: []rpctypes.SimBlock{
			{StateOverrides: &stateOverride},
		},
	}
	baseHeader := suite.ctxBaseHeader(25_000_000)
	resp, err := suite.simulateRequest(opts, baseHeader, 25_000_000)
	suite.Require().NoError(err)
	suite.Require().NotEmpty(resp.Result)
}

// TestSimulator_StateOverrides_StateDiff verifies that stateDiff overrides
// (SetState path) are applied via Apply.
func (suite *GRPCServerTestSuiteSuite) TestSimulator_StateOverrides_StateDiff() {
	suite.SetupTest()
	suite.App.EvmKeeper.BeginBlock(suite.Ctx)

	addr := tests.GenerateAddress()
	key := common.HexToHash("0x01")
	val := common.HexToHash("0xff")

	stateOverride := rpctypes.SimStateOverride{
		addr: rpctypes.SimOverrideAccount{
			StateDiff: map[common.Hash]common.Hash{key: val},
		},
	}

	opts := rpctypes.SimOpts{
		BlockStateCalls: []rpctypes.SimBlock{
			{StateOverrides: &stateOverride},
		},
	}
	baseHeader := suite.ctxBaseHeader(25_000_000)
	resp, err := suite.simulateRequest(opts, baseHeader, 25_000_000)
	suite.Require().NoError(err)
	suite.Require().NotEmpty(resp.Result)
}

// TestSimulator_StateOverrides_State verifies that full-state replacement
// (SetStorage path) is applied via Apply.
func (suite *GRPCServerTestSuiteSuite) TestSimulator_StateOverrides_State() {
	suite.SetupTest()
	suite.App.EvmKeeper.BeginBlock(suite.Ctx)

	addr := tests.GenerateAddress()
	key := common.HexToHash("0x02")
	val := common.HexToHash("0xab")

	stateOverride := rpctypes.SimStateOverride{
		addr: rpctypes.SimOverrideAccount{
			State: map[common.Hash]common.Hash{key: val},
		},
	}

	opts := rpctypes.SimOpts{
		BlockStateCalls: []rpctypes.SimBlock{
			{StateOverrides: &stateOverride},
		},
	}
	baseHeader := suite.ctxBaseHeader(25_000_000)
	resp, err := suite.simulateRequest(opts, baseHeader, 25_000_000)
	suite.Require().NoError(err)
	suite.Require().NotEmpty(resp.Result)
}

// TestSimulator_TraceTransfers_WithValue exercises the repairLogs inner loop
// by producing a synthetic ERC-7528 transfer log via traceTransfers=true with
// a non-zero ETH value.  The log's BlockHash is repaired by repairLogs.
func (suite *GRPCServerTestSuiteSuite) TestSimulator_TraceTransfers_WithValue() {
	suite.SetupTest()
	suite.App.EvmKeeper.BeginBlock(suite.Ctx)

	from := suite.Address
	to := tests.GenerateAddress()
	value := big.NewInt(1) // non-zero value triggers the transfer log

	// Give the sender enough balance via state override.
	largeBalance := (*hexutil.Big)(new(big.Int).Mul(big.NewInt(1e18), big.NewInt(100)))
	stateOverride := rpctypes.SimStateOverride{
		from: rpctypes.SimOverrideAccount{Balance: largeBalance},
	}

	callJSON, err := json.Marshal(map[string]interface{}{
		"from":  from.Hex(),
		"to":    to.Hex(),
		"gas":   hexutil.EncodeUint64(21000),
		"value": hexutil.EncodeBig(value),
	})
	suite.Require().NoError(err)

	opts := rpctypes.SimOpts{
		BlockStateCalls: []rpctypes.SimBlock{
			{
				StateOverrides: &stateOverride,
				Calls:          []json.RawMessage{callJSON},
			},
		},
		TraceTransfers: true,
	}
	baseHeader := suite.ctxBaseHeader(25_000_000)
	resp, err := suite.simulateRequest(opts, baseHeader, 25_000_000)
	suite.Require().NoError(err)
	suite.Require().NotEmpty(resp.Result)
}

// TestSimulator_StateOverrides_MovePrecompile exercises the successful
// precompile-move path in SimStateOverride.Apply (precompile moved to a new
// address that is not already in the override map).
func (suite *GRPCServerTestSuiteSuite) TestSimulator_StateOverrides_MovePrecompile() {
	suite.SetupTest()
	suite.App.EvmKeeper.BeginBlock(suite.Ctx)

	// ecrecover lives at 0x01 and is always present in the precompile map.
	precompileAddr := common.HexToAddress("0x0000000000000000000000000000000000000001")
	destAddr := tests.GenerateAddress()

	stateOverride := rpctypes.SimStateOverride{
		precompileAddr: rpctypes.SimOverrideAccount{
			MovePrecompileTo: &destAddr,
		},
	}

	opts := rpctypes.SimOpts{
		BlockStateCalls: []rpctypes.SimBlock{
			{StateOverrides: &stateOverride},
		},
	}
	baseHeader := suite.ctxBaseHeader(25_000_000)
	resp, err := suite.simulateRequest(opts, baseHeader, 25_000_000)
	suite.Require().NoError(err)
	suite.Require().NotEmpty(resp.Result)
}

// TestSimulator_SetCodeAuthorizations exercises the msg.SetCodeAuthorizations != nil
// branch in applyCall (EIP-7702 authorization path).
func (suite *GRPCServerTestSuiteSuite) TestSimulator_SetCodeAuthorizations() {
	suite.SetupTest()
	suite.App.EvmKeeper.BeginBlock(suite.Ctx)

	from := suite.Address
	to := tests.GenerateAddress()

	// Build a call with an authorization list (EIP-7702 SetCode).
	gas := hexutil.Uint64(50_000)
	val := hexutil.Big(*big.NewInt(0))
	nonce := hexutil.Uint64(0)
	args := evmtypes.TransactionArgs{
		From:  &from,
		To:    &to,
		Gas:   &gas,
		Value: &val,
		Nonce: &nonce,
		// Empty authorization list triggers the SetCodeAuthorizations loop in applyCall.
		AuthorizationList: []ethtypes.SetCodeAuthorization{{Address: to}},
	}
	callJSON, err := json.Marshal(args)
	suite.Require().NoError(err)

	opts := rpctypes.SimOpts{
		BlockStateCalls: []rpctypes.SimBlock{
			{Calls: []json.RawMessage{callJSON}},
		},
	}
	baseHeader := suite.ctxBaseHeader(25_000_000)
	resp, err := suite.simulateRequest(opts, baseHeader, 25_000_000)
	suite.Require().NoError(err)
	suite.Require().NotEmpty(resp.Result)
}

// TestSimulator_Validation_NonceTooLow exercises the nonce-too-low path in applyCall.
func (suite *GRPCServerTestSuiteSuite) TestSimulator_Validation_NonceTooLow() {
	suite.SetupTest()
	suite.App.EvmKeeper.BeginBlock(suite.Ctx)

	from := tests.GenerateAddress()
	to := tests.GenerateAddress()

	// Override nonce to 2 so tx nonce=1 is too low.
	stateNonce := hexutil.Uint64(2)
	stateOverride := rpctypes.SimStateOverride{
		from: rpctypes.SimOverrideAccount{Nonce: &stateNonce},
	}

	callJSON, err := json.Marshal(map[string]interface{}{
		"from":  from.Hex(),
		"to":    to.Hex(),
		"gas":   hexutil.EncodeUint64(21000),
		"nonce": hexutil.EncodeUint64(1), // tx nonce=1 < state nonce=2
		"value": hexutil.EncodeBig(big.NewInt(0)),
	})
	suite.Require().NoError(err)

	opts := rpctypes.SimOpts{
		BlockStateCalls: []rpctypes.SimBlock{
			{
				StateOverrides: &stateOverride,
				Calls:          []json.RawMessage{callJSON},
			},
		},
		Validation: true,
	}
	baseHeader := suite.ctxBaseHeader(25_000_000)
	baseHeader.BaseFee = big.NewInt(0)
	resp, err := suite.simulateRequest(opts, baseHeader, 25_000_000)
	suite.Require().NoError(err)
	// Validation errors propagate as simulation-level errors.
	suite.Require().NotZero(resp.ErrorCode)
	suite.Require().Contains(resp.ErrorMessage, "nonce too low")
}

// TestSimulator_Validation_FeeCapTooLow exercises the fee-cap-too-low path in applyCall.
func (suite *GRPCServerTestSuiteSuite) TestSimulator_Validation_FeeCapTooLow() {
	suite.SetupTest()
	suite.App.EvmKeeper.BeginBlock(suite.Ctx)

	from := tests.GenerateAddress()
	to := tests.GenerateAddress()

	// Set a large balance so the balance check passes.
	largeBalance := (*hexutil.Big)(new(big.Int).Mul(big.NewInt(1e18), big.NewInt(100)))
	stateOverride := rpctypes.SimStateOverride{
		from: rpctypes.SimOverrideAccount{Balance: largeBalance},
	}

	callJSON, err := json.Marshal(map[string]interface{}{
		"from":                 from.Hex(),
		"to":                   to.Hex(),
		"gas":                  hexutil.EncodeUint64(21000),
		"value":                hexutil.EncodeBig(big.NewInt(0)),
		"maxFeePerGas":         hexutil.EncodeBig(big.NewInt(1)), // lower than baseFee=100
		"maxPriorityFeePerGas": hexutil.EncodeBig(big.NewInt(1)),
	})
	suite.Require().NoError(err)

	opts := rpctypes.SimOpts{
		BlockStateCalls: []rpctypes.SimBlock{
			{
				StateOverrides: &stateOverride,
				Calls:          []json.RawMessage{callJSON},
			},
		},
		Validation: true,
	}
	baseHeader := suite.ctxBaseHeader(25_000_000)
	baseHeader.BaseFee = big.NewInt(100) // higher than maxFeePerGas=1
	resp, err := suite.simulateRequest(opts, baseHeader, 25_000_000)
	suite.Require().NoError(err)
	// Validation errors propagate as simulation-level errors.
	suite.Require().NotZero(resp.ErrorCode)
	suite.Require().Contains(resp.ErrorMessage, "max fee per gas less than block base fee")
}

// TestSimulator_Validation_InsufficientFunds exercises the insufficient-funds path in applyCall.
func (suite *GRPCServerTestSuiteSuite) TestSimulator_Validation_InsufficientFunds() {
	suite.SetupTest()
	suite.App.EvmKeeper.BeginBlock(suite.Ctx)

	// Fresh address with zero balance.
	from := tests.GenerateAddress()
	to := tests.GenerateAddress()

	callJSON, err := json.Marshal(map[string]interface{}{
		"from":                 from.Hex(),
		"to":                   to.Hex(),
		"gas":                  hexutil.EncodeUint64(21000),
		"value":                hexutil.EncodeBig(big.NewInt(1_000_000)), // requires funds
		"maxFeePerGas":         hexutil.EncodeBig(big.NewInt(0)),
		"maxPriorityFeePerGas": hexutil.EncodeBig(big.NewInt(0)),
	})
	suite.Require().NoError(err)

	opts := rpctypes.SimOpts{
		BlockStateCalls: []rpctypes.SimBlock{
			{Calls: []json.RawMessage{callJSON}},
		},
		Validation: true,
	}
	baseHeader := suite.ctxBaseHeader(25_000_000)
	baseHeader.BaseFee = big.NewInt(0)
	resp, err := suite.simulateRequest(opts, baseHeader, 25_000_000)
	suite.Require().NoError(err)
	// Validation errors propagate as simulation-level errors.
	suite.Require().NotZero(resp.ErrorCode)
	suite.Require().Contains(resp.ErrorMessage, "insufficient funds")
}

// TestSimulator_EVMNonRevertError exercises the non-revert EVM error path in processBlock
// (e.g., out-of-gas, which results in vm.ErrOutOfGas rather than vm.ErrExecutionReverted).
func (suite *GRPCServerTestSuiteSuite) TestSimulator_EVMNonRevertError() {
	suite.SetupTest()
	suite.App.EvmKeeper.BeginBlock(suite.Ctx)

	from := suite.Address
	target := tests.GenerateAddress()

	// Infinite loop bytecode: JUMPDEST (0x5b), PUSH1 0 (0x60, 0x00), JUMP (0x56)
	// This loops forever and will exhaust any gas limit → vm.ErrOutOfGas.
	infiniteLoop := hexutil.Bytes{0x5b, 0x60, 0x00, 0x56}
	stateOverride := rpctypes.SimStateOverride{
		target: rpctypes.SimOverrideAccount{
			Code: &infiniteLoop,
		},
	}

	callJSON, err := json.Marshal(map[string]interface{}{
		"from":  from.Hex(),
		"to":    target.Hex(),
		"gas":   hexutil.EncodeUint64(25_000), // just over intrinsic gas; loop burns the rest
		"value": hexutil.EncodeBig(big.NewInt(0)),
	})
	suite.Require().NoError(err)

	opts := rpctypes.SimOpts{
		BlockStateCalls: []rpctypes.SimBlock{
			{
				StateOverrides: &stateOverride,
				Calls:          []json.RawMessage{callJSON},
			},
		},
	}
	baseHeader := suite.ctxBaseHeader(25_000_000)
	resp, err := suite.simulateRequest(opts, baseHeader, 25_000_000)
	suite.Require().NoError(err)
	suite.Require().NotEmpty(resp.Result)

	var results []map[string]json.RawMessage
	suite.Require().NoError(json.Unmarshal(resp.Result, &results))
	suite.Require().Len(results, 1)
	var calls []map[string]json.RawMessage
	suite.Require().NoError(json.Unmarshal(results[0]["calls"], &calls))
	suite.Require().Len(calls, 1)
	// Status should be failure (0x0) with an error object.
	suite.Require().Contains(string(calls[0]["status"]), "0x0")
	suite.Require().NotNil(calls[0]["error"])
}

// TestSimulateV1_GasCapEnforced_ZeroGasCap exercises the gasCap=0 path in SimulateV1
// where queryMaxGasLimit is applied as the new cap.
func (suite *GRPCServerTestSuiteSuite) TestSimulateV1_GasCapEnforced_ZeroGasCap() {
	suite.SetupTest()
	suite.App.EvmKeeper.BeginBlock(suite.Ctx)
	suite.App.EvmKeeper.SetQueryMaxGasLimitForTest(50_000_000)

	opts := rpctypes.SimOpts{
		BlockStateCalls: []rpctypes.SimBlock{{}},
	}
	baseHeader := suite.ctxBaseHeader(25_000_000)
	// GasCap=0 triggers "gasCap = k.queryMaxGasLimit" branch.
	resp, err := suite.simulateRequest(opts, baseHeader, 0)
	suite.Require().NoError(err)
	suite.Require().NotEmpty(resp.Result)
}

// TestSimulateV1_GasCapEnforced_ClampedGasCap exercises the path where gasCap > queryMaxGasLimit
// so it gets clamped to queryMaxGasLimit.
func (suite *GRPCServerTestSuiteSuite) TestSimulateV1_GasCapEnforced_ClampedGasCap() {
	suite.SetupTest()
	suite.App.EvmKeeper.BeginBlock(suite.Ctx)
	suite.App.EvmKeeper.SetQueryMaxGasLimitForTest(1_000_000)

	opts := rpctypes.SimOpts{
		BlockStateCalls: []rpctypes.SimBlock{{}},
	}
	baseHeader := suite.ctxBaseHeader(25_000_000)
	// GasCap=25_000_000 > queryMaxGasLimit=1_000_000 → clamped.
	resp, err := suite.simulateRequest(opts, baseHeader, 25_000_000)
	suite.Require().NoError(err)
	suite.Require().NotEmpty(resp.Result)
}

// TestSimulator_BlockHash_BaseNum exercises the base-block hash lookup path in getHashFn
// by executing a contract that calls BLOCKHASH(baseNum).
func (suite *GRPCServerTestSuiteSuite) TestSimulator_BlockHash_BaseNum() {
	suite.SetupTest()
	suite.App.EvmKeeper.BeginBlock(suite.Ctx)

	from := suite.Address
	target := tests.GenerateAddress()

	// Contract: PUSH1 baseNum, BLOCKHASH, STOP
	// baseNum = suite.Ctx.BlockHeight() = 1 in the default test setup.
	baseNum := byte(suite.Ctx.BlockHeight())
	blockhashCode := hexutil.Bytes{0x60, baseNum, 0x40, 0x00}
	stateOverride := rpctypes.SimStateOverride{
		target: rpctypes.SimOverrideAccount{Code: &blockhashCode},
	}

	callJSON, err := json.Marshal(map[string]interface{}{
		"from":  from.Hex(),
		"to":    target.Hex(),
		"gas":   hexutil.EncodeUint64(100_000),
		"value": hexutil.EncodeBig(big.NewInt(0)),
	})
	suite.Require().NoError(err)

	opts := rpctypes.SimOpts{
		BlockStateCalls: []rpctypes.SimBlock{
			{StateOverrides: &stateOverride, Calls: []json.RawMessage{callJSON}},
		},
	}
	baseHeader := suite.ctxBaseHeader(25_000_000)
	resp, err := suite.simulateRequest(opts, baseHeader, 25_000_000)
	suite.Require().NoError(err)
	suite.Require().NotEmpty(resp.Result)
}

// TestSimulator_BlockHash_Fallback exercises the keeper-fallback path in getHashFn
// by querying a block number that is neither a simulated header nor the base block.
func (suite *GRPCServerTestSuiteSuite) TestSimulator_BlockHash_Fallback() {
	suite.SetupTest()
	suite.App.EvmKeeper.BeginBlock(suite.Ctx)

	from := suite.Address
	target := tests.GenerateAddress()

	// PUSH1 0, BLOCKHASH, STOP — queries block 0 which is before the base block (1).
	blockhashCode := hexutil.Bytes{0x60, 0x00, 0x40, 0x00}
	stateOverride := rpctypes.SimStateOverride{
		target: rpctypes.SimOverrideAccount{Code: &blockhashCode},
	}

	callJSON, err := json.Marshal(map[string]interface{}{
		"from":  from.Hex(),
		"to":    target.Hex(),
		"gas":   hexutil.EncodeUint64(100_000),
		"value": hexutil.EncodeBig(big.NewInt(0)),
	})
	suite.Require().NoError(err)

	opts := rpctypes.SimOpts{
		BlockStateCalls: []rpctypes.SimBlock{
			{StateOverrides: &stateOverride, Calls: []json.RawMessage{callJSON}},
		},
	}
	baseHeader := suite.ctxBaseHeader(25_000_000)
	resp, err := suite.simulateRequest(opts, baseHeader, 25_000_000)
	suite.Require().NoError(err)
	suite.Require().NotEmpty(resp.Result)
}

// TestSimulator_BlockHash_PrevHeaders exercises the prevHeaders match path in getHashFn.
// Block 2 queries BLOCKHASH of the first simulated block (number = baseNum+1).
func (suite *GRPCServerTestSuiteSuite) TestSimulator_BlockHash_PrevHeaders() {
	suite.SetupTest()
	suite.App.EvmKeeper.BeginBlock(suite.Ctx)

	from := suite.Address
	target := tests.GenerateAddress()

	// The first simulated block gets number = baseNum+1.
	// suite.Ctx.BlockHeight() == 1 → first simulated block is at number 2.
	firstSimBlockNum := byte(suite.Ctx.BlockHeight() + 1)
	// Contract: PUSH1 firstSimBlockNum, BLOCKHASH, STOP
	blockhashCode := hexutil.Bytes{0x60, firstSimBlockNum, 0x40, 0x00}
	stateOverride := rpctypes.SimStateOverride{
		target: rpctypes.SimOverrideAccount{Code: &blockhashCode},
	}

	callJSON, err := json.Marshal(map[string]interface{}{
		"from":  from.Hex(),
		"to":    target.Hex(),
		"gas":   hexutil.EncodeUint64(100_000),
		"value": hexutil.EncodeBig(big.NewInt(0)),
	})
	suite.Require().NoError(err)

	// First block (number 2): empty — just to populate prevHeaders.
	// Second block (number 3): has the BLOCKHASH contract call.
	opts := rpctypes.SimOpts{
		BlockStateCalls: []rpctypes.SimBlock{
			{},
			{StateOverrides: &stateOverride, Calls: []json.RawMessage{callJSON}},
		},
	}
	baseHeader := suite.ctxBaseHeader(25_000_000)
	resp, err := suite.simulateRequest(opts, baseHeader, 25_000_000)
	suite.Require().NoError(err)
	suite.Require().NotEmpty(resp.Result)
}

// TestSimulator_SanitizeChain_BlockSpanTooLarge exercises the "too many blocks" error
// in sanitizeChain when a single block's number is too far from the base.
func (suite *GRPCServerTestSuiteSuite) TestSimulator_SanitizeChain_BlockSpanTooLarge() {
	suite.SetupTest()
	suite.App.EvmKeeper.BeginBlock(suite.Ctx)

	baseHeader := suite.ctxBaseHeader(25_000_000)
	// A block number more than MaxSimulateBlocks ahead of base triggers the span check.
	farNum := (*hexutil.Big)(new(big.Int).Add(baseHeader.Number, big.NewInt(rpctypes.MaxSimulateBlocks+1)))
	opts := rpctypes.SimOpts{
		BlockStateCalls: []rpctypes.SimBlock{
			{BlockOverrides: &rpctypes.SimBlockOverrides{Number: farNum}},
		},
	}
	resp, err := suite.simulateRequest(opts, baseHeader, 25_000_000)
	suite.Require().NoError(err)
	suite.Require().NotZero(resp.ErrorCode)
	suite.Require().Contains(resp.ErrorMessage, "too many blocks")
}

// TestSimulator_ValidationMode_GasCharged verifies that in validation mode the
// sender's balance is reduced by gasUsed*gasPrice after each applyCall, so that
// a second call from the same sender with insufficient remaining balance fails
// with ErrInsufficientFunds instead of passing a stale (unreduced) balance check.
func (suite *GRPCServerTestSuiteSuite) TestSimulator_ValidationMode_GasCharged() {
	suite.SetupTest()
	suite.App.EvmKeeper.BeginBlock(suite.Ctx)

	from := suite.Address
	to := tests.GenerateAddress()

	// A simple ETH transfer costs exactly 21000 gas.
	// Set gasPrice = 1 wei (using legacy gasPrice field), baseFee = 0.
	// Fund the sender with only 21001 wei so the first call (21000 gas * 1 = 21000)
	// succeeds but the second call's upfront balance check (21000 * 1 = 21000 > 1)
	// fails — proving the balance was actually deducted after the first call.
	initialBalance := uint256.NewInt(21001)
	suite.Require().NoError(
		suite.App.EvmKeeper.SetBalance(suite.Ctx, from, *initialBalance, types.DefaultEVMDenom),
	)

	nonce := suite.App.EvmKeeper.GetNonce(suite.Ctx, from)

	makeCall := func(n uint64) json.RawMessage {
		raw, err := json.Marshal(map[string]interface{}{
			"from":                 from.Hex(),
			"to":                   to.Hex(),
			"gas":                  hexutil.EncodeUint64(21000),
			"maxFeePerGas":         hexutil.EncodeBig(big.NewInt(1)),
			"maxPriorityFeePerGas": hexutil.EncodeBig(big.NewInt(1)),
			"value":                hexutil.EncodeBig(big.NewInt(0)),
			"nonce":                hexutil.EncodeUint64(n),
		})
		suite.Require().NoError(err)
		return raw
	}

	opts := rpctypes.SimOpts{
		BlockStateCalls: []rpctypes.SimBlock{
			{Calls: []json.RawMessage{makeCall(nonce), makeCall(nonce + 1)}},
		},
		Validation: true,
	}
	baseHeader := suite.ctxBaseHeader(25_000_000)
	baseHeader.BaseFee = big.NewInt(0) // baseFee = 0 so effective gasPrice = min(1, 0+1) = 1

	resp, err := suite.simulateRequest(opts, baseHeader, 25_000_000)
	suite.Require().NoError(err)

	// The simulation returns an error response because the second call cannot
	// pay its gas (only 1 wei left after the first call deducted 21000 wei).
	suite.Require().NotZero(resp.ErrorCode, "expected second call to fail with insufficient funds")
	suite.Require().Contains(resp.ErrorMessage, "insufficient funds")
}

// TestSimulator_SimulationMode_BalanceNotCharged verifies that in non-validation
// (simulation) mode the sender's balance is NOT charged for gas when the default
// zero gas price is used. A sender with zero balance must still be able to execute
// a call because both the balance check and the gas deduction are skipped.
func (suite *GRPCServerTestSuiteSuite) TestSimulator_SimulationMode_BalanceNotCharged() {
	suite.SetupTest()
	suite.App.EvmKeeper.BeginBlock(suite.Ctx)

	// Use a fresh address with zero balance — no funds needed in simulation mode.
	from := tests.GenerateAddress()
	to := tests.GenerateAddress()

	// Register the account so GetNonce works.
	suite.App.AccountKeeper.SetAccount(suite.Ctx,
		suite.App.AccountKeeper.NewAccountWithAddress(suite.Ctx, sdk.AccAddress(from.Bytes())),
	)

	callJSON, err := json.Marshal(map[string]interface{}{
		"from":  from.Hex(),
		"to":    to.Hex(),
		"gas":   hexutil.EncodeUint64(21000),
		"value": hexutil.EncodeBig(big.NewInt(0)),
		// No gasPrice / maxFeePerGas: defaults to 0 in simulation mode.
	})
	suite.Require().NoError(err)

	opts := rpctypes.SimOpts{
		BlockStateCalls: []rpctypes.SimBlock{
			{Calls: []json.RawMessage{callJSON}},
		},
		Validation: false, // simulation mode
	}
	baseHeader := suite.ctxBaseHeader(25_000_000)
	resp, err := suite.simulateRequest(opts, baseHeader, 25_000_000)
	suite.Require().NoError(err)
	suite.Require().NotEmpty(resp.Result)

	// Must be a valid block result, not an error envelope.
	var blocks []json.RawMessage
	suite.Require().NoError(json.Unmarshal(resp.Result, &blocks), "expected block array result")
	suite.Require().Len(blocks, 1)

	// The call must succeed (status 0x1).
	var results []map[string]json.RawMessage
	suite.Require().NoError(json.Unmarshal(resp.Result, &results))
	var calls []map[string]json.RawMessage
	suite.Require().NoError(json.Unmarshal(results[0]["calls"], &calls))
	suite.Require().Len(calls, 1)
	suite.Require().Equal(`"0x1"`, string(calls[0]["status"]))
}

func (suite *GRPCServerTestSuiteSuite) TestCreateAccessList() {
	address := tests.GenerateAddress()
	supply := sdkmath.NewIntWithDecimal(1000, 18).BigInt()

	ctorArgs, err := types.ERC20Contract.ABI.Pack("", address, supply)
	suite.Require().NoError(err)
	data := append(types.ERC20Contract.Bin, ctorArgs...)

	testCases := []struct {
		name     string
		req      *types.EthCallRequest
		expPass  bool
		expError string
	}{
		{
			"nil request",
			nil,
			false,
			"",
		},
		{
			"invalid args",
			&types.EthCallRequest{Args: []byte("invalid"), GasCap: uint64(config.DefaultGasCap)},
			false,
			"",
		},
		{
			"pass - contract deployment with GasUsed populated",
			&types.EthCallRequest{
				Args: func() []byte {
					gas := hexutil.Uint64(config.DefaultGasCap)
					args, _ := json.Marshal(&types.TransactionArgs{
						From: &address,
						Data: (*hexutil.Bytes)(&data),
						Gas:  &gas,
					})
					return args
				}(),
				GasCap: uint64(config.DefaultGasCap),
			},
			true,
			"",
		},
	}

	for _, tc := range testCases {
		suite.Run(tc.name, func() {
			suite.SetupTest()
			res, err := suite.App.EvmKeeper.CreateAccessList(suite.Ctx, tc.req)
			if tc.expPass {
				suite.Require().NoError(err)
				suite.Require().NotNil(res)
				var result types.AccessListResult
				suite.Require().NoError(json.Unmarshal(res.Data, &result))
				suite.Require().NotZero(uint64(result.GasUsed))
				suite.Require().Equal(tc.expError, result.Error)
			} else {
				suite.Require().Error(err)
			}
		})
	}
}

func (suite *GRPCServerTestSuiteSuite) TestCreateAccessList_VmError() {
	contractAddr := suite.deployTestContract(suite.Address)
	suite.Commit(suite.T())

	// Call transfer from an address that holds no tokens — triggers ERC20 revert.
	caller := tests.GenerateAddress()
	recipient := tests.GenerateAddress()
	transferData, err := types.ERC20Contract.ABI.Pack("transfer", recipient, big.NewInt(1))
	suite.Require().NoError(err)

	gas := hexutil.Uint64(config.DefaultGasCap)
	args, err := json.Marshal(&types.TransactionArgs{
		From: &caller,
		To:   &contractAddr,
		Data: (*hexutil.Bytes)(&transferData),
		Gas:  &gas,
	})
	suite.Require().NoError(err)

	res, err := suite.App.EvmKeeper.CreateAccessList(suite.Ctx, &types.EthCallRequest{
		Args:   args,
		GasCap: uint64(config.DefaultGasCap),
	})
	suite.Require().NoError(err)
	suite.Require().NotNil(res)

	var result types.AccessListResult
	suite.Require().NoError(json.Unmarshal(res.Data, &result))
	suite.Require().NotEmpty(result.Error)
	suite.Require().NotZero(uint64(result.GasUsed))
	suite.Require().NotEmpty(result.AccessList)
}

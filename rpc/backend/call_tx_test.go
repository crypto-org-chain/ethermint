package backend

import (
	"encoding/json"
	"fmt"
	"math/big"

	sdkmath "cosmossdk.io/math"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	ethtypes "github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/rlp"
	"github.com/evmos/ethermint/rpc/backend/mocks"
	rpctypes "github.com/evmos/ethermint/rpc/types"
	"github.com/evmos/ethermint/tests"
	evmtypes "github.com/evmos/ethermint/x/evm/types"
	"google.golang.org/grpc/metadata"
)

func (suite *BackendTestSuite) TestResend() {
	txNonce := (hexutil.Uint64)(1)
	baseFee := sdkmath.NewInt(1)
	gasPrice := new(hexutil.Big)
	toAddr := tests.GenerateAddress()
	chainID := (*hexutil.Big)(suite.backend.chainID)
	validator := sdk.AccAddress(tests.GenerateAddress().Bytes())
	height := int64(1)
	callArgs := evmtypes.TransactionArgs{
		From:                 nil,
		To:                   &toAddr,
		Gas:                  nil,
		GasPrice:             nil,
		MaxFeePerGas:         gasPrice,
		MaxPriorityFeePerGas: gasPrice,
		Value:                gasPrice,
		Nonce:                &txNonce,
		Input:                nil,
		Data:                 nil,
		AccessList:           nil,
		ChainID:              chainID,
	}

	testCases := []struct {
		name         string
		registerMock func()
		args         evmtypes.TransactionArgs
		gasPrice     *hexutil.Big
		gasLimit     *hexutil.Uint64
		expHash      common.Hash
		expPass      bool
	}{
		{
			"fail - Missing transaction nonce ",
			func() {},
			evmtypes.TransactionArgs{
				Nonce: nil,
			},
			nil,
			nil,
			common.Hash{},
			false,
		},
		{
			"pass - Can't set Tx defaults BaseFee disabled",
			func() {
				var header metadata.MD
				client := suite.backend.clientCtx.Client.(*mocks.Client)
				queryClient := suite.backend.queryClient.QueryClient.(*mocks.EVMQueryClient)
				RegisterParams(queryClient, &header, height)
				RegisterEmptyBlockResults(client, 1)
				RegisterBlock(client, height, nil)
				RegisterBaseFeeDisabled(queryClient)
				RegisterValidatorAccount(queryClient, validator)
			},
			evmtypes.TransactionArgs{
				Nonce:   &txNonce,
				ChainID: callArgs.ChainID,
			},
			nil,
			nil,
			common.Hash{},
			true,
		},
		{
			"pass - Can't set Tx defaults ",
			func() {
				var header metadata.MD
				client := suite.backend.clientCtx.Client.(*mocks.Client)
				queryClient := suite.backend.queryClient.QueryClient.(*mocks.EVMQueryClient)
				feeMarketClient := suite.backend.queryClient.FeeMarket.(*mocks.FeeMarketQueryClient)
				RegisterParams(queryClient, &header, height)
				RegisterFeeMarketParams(feeMarketClient, height)
				RegisterEmptyBlockResults(client, height)
				RegisterBlock(client, height, nil)
				RegisterBaseFee(queryClient, baseFee)
				RegisterValidatorAccount(queryClient, validator)
			},
			evmtypes.TransactionArgs{
				Nonce: &txNonce,
			},
			nil,
			nil,
			common.Hash{},
			true,
		},
		{
			"pass - MaxFeePerGas is nil",
			func() {
				var header metadata.MD
				client := suite.backend.clientCtx.Client.(*mocks.Client)
				queryClient := suite.backend.queryClient.QueryClient.(*mocks.EVMQueryClient)
				RegisterParams(queryClient, &header, height)
				RegisterEmptyBlockResults(client, height)
				RegisterBlock(client, height, nil)
				RegisterBaseFeeDisabled(queryClient)
				RegisterValidatorAccount(queryClient, validator)
			},
			evmtypes.TransactionArgs{
				Nonce:                &txNonce,
				MaxPriorityFeePerGas: nil,
				GasPrice:             nil,
				MaxFeePerGas:         nil,
			},
			nil,
			nil,
			common.Hash{},
			true,
		},
		{
			"fail - GasPrice and (MaxFeePerGas or MaxPriorityPerGas specified",
			func() {},
			evmtypes.TransactionArgs{
				Nonce:                &txNonce,
				MaxPriorityFeePerGas: nil,
				GasPrice:             gasPrice,
				MaxFeePerGas:         gasPrice,
			},
			nil,
			nil,
			common.Hash{},
			false,
		},
		{
			"fail - Block error",
			func() {
				var header metadata.MD
				client := suite.backend.clientCtx.Client.(*mocks.Client)
				queryClient := suite.backend.queryClient.QueryClient.(*mocks.EVMQueryClient)
				RegisterParams(queryClient, &header, height)
				RegisterBlockError(client, height)
			},
			evmtypes.TransactionArgs{
				Nonce: &txNonce,
			},
			nil,
			nil,
			common.Hash{},
			false,
		},
		{
			"pass - MaxFeePerGas is nil",
			func() {
				var header metadata.MD
				client := suite.backend.clientCtx.Client.(*mocks.Client)
				queryClient := suite.backend.queryClient.QueryClient.(*mocks.EVMQueryClient)
				RegisterParams(queryClient, &header, height)
				RegisterEmptyBlockResults(client, height)
				RegisterBlock(client, height, nil)
				RegisterBaseFee(queryClient, baseFee)
				RegisterValidatorAccount(queryClient, validator)
			},
			evmtypes.TransactionArgs{
				Nonce:                &txNonce,
				GasPrice:             nil,
				MaxPriorityFeePerGas: gasPrice,
				MaxFeePerGas:         gasPrice,
				ChainID:              callArgs.ChainID,
			},
			nil,
			nil,
			common.Hash{},
			true,
		},
		{
			"pass - Chain Id is nil",
			func() {
				var header metadata.MD
				client := suite.backend.clientCtx.Client.(*mocks.Client)
				queryClient := suite.backend.queryClient.QueryClient.(*mocks.EVMQueryClient)
				RegisterParams(queryClient, &header, height)
				RegisterEmptyBlockResults(client, height)
				RegisterBlock(client, height, nil)
				RegisterBaseFee(queryClient, baseFee)
				RegisterValidatorAccount(queryClient, validator)
			},
			evmtypes.TransactionArgs{
				Nonce:                &txNonce,
				MaxPriorityFeePerGas: gasPrice,
				ChainID:              nil,
			},
			nil,
			nil,
			common.Hash{},
			true,
		},
		{
			"fail - Pending transactions error",
			func() {
				var header metadata.MD
				client := suite.backend.clientCtx.Client.(*mocks.Client)
				queryClient := suite.backend.queryClient.QueryClient.(*mocks.EVMQueryClient)
				RegisterHeader(client, &height, nil)
				RegisterEmptyBlockResults(client, height)
				RegisterBlock(client, height, nil)
				RegisterBaseFee(queryClient, baseFee)
				RegisterEstimateGas(queryClient, callArgs)
				RegisterParams(queryClient, &header, height)
				RegisterParamsWithoutHeader(queryClient, height)
				RegisterUnconfirmedTxsError(client, nil)
				RegisterValidatorAccount(queryClient, validator)
			},
			evmtypes.TransactionArgs{
				Nonce:                &txNonce,
				To:                   &toAddr,
				MaxFeePerGas:         gasPrice,
				MaxPriorityFeePerGas: gasPrice,
				Value:                gasPrice,
				Gas:                  nil,
				ChainID:              callArgs.ChainID,
			},
			gasPrice,
			nil,
			common.Hash{},
			false,
		},
		{
			"fail - Not Ethereum txs",
			func() {
				var header metadata.MD
				client := suite.backend.clientCtx.Client.(*mocks.Client)
				queryClient := suite.backend.queryClient.QueryClient.(*mocks.EVMQueryClient)
				RegisterHeader(client, &height, nil)
				RegisterEmptyBlockResults(client, height)
				RegisterBlock(client, height, nil)
				RegisterBaseFee(queryClient, baseFee)
				RegisterEstimateGas(queryClient, callArgs)
				RegisterParams(queryClient, &header, height)
				RegisterParamsWithoutHeader(queryClient, height)
				RegisterUnconfirmedTxsEmpty(client, nil)
				RegisterValidatorAccount(queryClient, validator)
			},
			evmtypes.TransactionArgs{
				Nonce:                &txNonce,
				To:                   &toAddr,
				MaxFeePerGas:         gasPrice,
				MaxPriorityFeePerGas: gasPrice,
				Value:                gasPrice,
				Gas:                  nil,
				ChainID:              callArgs.ChainID,
			},
			gasPrice,
			nil,
			common.Hash{},
			false,
		},
	}

	for _, tc := range testCases {
		suite.Run(fmt.Sprintf("case %s", tc.name), func() {
			suite.SetupTest() // reset test and queries
			tc.registerMock()

			hash, err := suite.backend.Resend(tc.args, tc.gasPrice, tc.gasLimit)

			if tc.expPass {
				suite.Require().Equal(tc.expHash, hash)
			} else {
				suite.Require().Error(err)
			}
		})
	}
}

func (suite *BackendTestSuite) TestSendRawTransaction() {
	ethTx, bz := suite.buildEthereumTx()
	rlpEncodedBz, _ := rlp.EncodeToBytes(ethTx.AsTransaction())
	cosmosTx, _ := ethTx.BuildTx(suite.backend.clientCtx.TxConfig.NewTxBuilder(), "aphoton")
	txBytes, _ := suite.backend.clientCtx.TxConfig.TxEncoder()(cosmosTx)

	// Build a tx whose fee exceeds the default 1 ether cap (gasPrice * gas > 1e18).
	highFeeTx := evmtypes.NewTx(
		suite.backend.chainID, 0, &common.Address{}, big.NewInt(0),
		100000, new(big.Int).Mul(big.NewInt(1e10), big.NewInt(1e9)), // 10 gwei * 100k gas = 1e6 gwei > 1 ether... actually 1e10 * 1e9 = 1e19 wei = 10 ether
		nil, nil, nil, nil,
	)
	highFeeTx.From = suite.signerAddress
	_ = highFeeTx.Sign(ethtypes.LatestSignerForChainID(suite.backend.chainID), suite.signer)
	highFeeRlp, _ := rlp.EncodeToBytes(highFeeTx.AsTransaction())

	// Build a tx signed with a different chain ID.
	wrongChainID := new(big.Int).Add(suite.backend.chainID, big.NewInt(1))
	wrongChainTx := evmtypes.NewTx(
		wrongChainID, 0, &common.Address{}, big.NewInt(0),
		100000, big.NewInt(1), nil, nil, nil, nil,
	)
	wrongChainTx.From = suite.signerAddress
	_ = wrongChainTx.Sign(ethtypes.LatestSignerForChainID(wrongChainID), suite.signer)
	wrongChainRlp, _ := rlp.EncodeToBytes(wrongChainTx.AsTransaction())

	testCases := []struct {
		name         string
		registerMock func()
		rawTx        []byte
		expHash      common.Hash
		expPass      bool
	}{
		{
			"fail - empty bytes",
			func() {},
			[]byte{},
			common.Hash{},
			false,
		},
		{
			"fail - no RLP encoded bytes",
			func() {},
			bz,
			common.Hash{},
			false,
		},
		{
			"fail - fee exceeds configured cap",
			func() {
				suite.backend.allowUnprotectedTxs = true
				suite.backend.cfg.JSONRPC.TxFeeCap = 1.0 // 1 ether cap
			},
			highFeeRlp,
			common.Hash{},
			false,
		},
		{
			"fail - wrong chain ID",
			func() { suite.backend.allowUnprotectedTxs = true },
			wrongChainRlp,
			common.Hash{},
			false,
		},
		{
			"fail - unprotected transactions",
			func() {
				suite.backend.allowUnprotectedTxs = false
				queryClient := suite.backend.queryClient.QueryClient.(*mocks.EVMQueryClient)
				RegisterParamsWithoutHeaderError(queryClient, 1)
			},
			rlpEncodedBz,
			common.Hash{},
			false,
		},
		{
			"fail - failed to get evm params",
			func() {
				queryClient := suite.backend.queryClient.QueryClient.(*mocks.EVMQueryClient)
				suite.backend.allowUnprotectedTxs = true
				RegisterParamsWithoutHeaderError(queryClient, 1)
			},
			rlpEncodedBz,
			common.Hash{},
			false,
		},
		{
			"fail - failed to broadcast transaction",
			func() {
				client := suite.backend.clientCtx.Client.(*mocks.Client)
				queryClient := suite.backend.queryClient.QueryClient.(*mocks.EVMQueryClient)
				suite.backend.allowUnprotectedTxs = true
				RegisterParamsWithoutHeader(queryClient, 1)
				RegisterBroadcastTxError(client, txBytes)
			},
			rlpEncodedBz,
			ethTx.Hash(),
			false,
		},
		{
			"pass - Gets the correct transaction hash of the eth transaction",
			func() {
				client := suite.backend.clientCtx.Client.(*mocks.Client)
				queryClient := suite.backend.queryClient.QueryClient.(*mocks.EVMQueryClient)
				suite.backend.allowUnprotectedTxs = true
				RegisterParamsWithoutHeader(queryClient, 1)
				RegisterBroadcastTx(client, txBytes)
			},
			rlpEncodedBz,
			ethTx.Hash(),
			true,
		},
	}

	for _, tc := range testCases {
		suite.Run(fmt.Sprintf("case %s", tc.name), func() {
			suite.SetupTest() // reset test and queries
			tc.registerMock()

			hash, err := suite.backend.SendRawTransaction(tc.rawTx)

			if tc.expPass {
				suite.Require().Equal(tc.expHash, hash)
			} else {
				suite.Require().Error(err)
			}
		})
	}
}

func (suite *BackendTestSuite) TestDoCall() {
	_, bz := suite.buildEthereumTx()
	gasPrice := (*hexutil.Big)(big.NewInt(1))
	toAddr := tests.GenerateAddress()
	chainID := (*hexutil.Big)(suite.backend.chainID)
	callArgs := evmtypes.TransactionArgs{
		From:                 nil,
		To:                   &toAddr,
		Gas:                  nil,
		GasPrice:             nil,
		MaxFeePerGas:         gasPrice,
		MaxPriorityFeePerGas: gasPrice,
		Value:                gasPrice,
		Input:                nil,
		Data:                 nil,
		AccessList:           nil,
		ChainID:              chainID,
	}
	argsBz, err := json.Marshal(callArgs)
	suite.Require().NoError(err)

	testCases := []struct {
		name         string
		registerMock func()
		blockNum     rpctypes.BlockNumber
		callArgs     evmtypes.TransactionArgs
		expEthTx     *evmtypes.EthCallResponse
		expPass      bool
	}{
		{
			"fail - Invalid request",
			func() {
				client := suite.backend.clientCtx.Client.(*mocks.Client)
				queryClient := suite.backend.queryClient.QueryClient.(*mocks.EVMQueryClient)
				height := int64(1)
				RegisterHeader(client, &height, bz)
				RegisterEthCallError(queryClient, &evmtypes.EthCallRequest{Args: argsBz, ChainId: suite.backend.chainID.Int64()})
			},
			rpctypes.BlockNumber(1),
			callArgs,
			&evmtypes.EthCallResponse{},
			false,
		},
		{
			"pass - Returned transaction response",
			func() {
				client := suite.backend.clientCtx.Client.(*mocks.Client)
				queryClient := suite.backend.queryClient.QueryClient.(*mocks.EVMQueryClient)
				height := int64(1)
				RegisterHeader(client, &height, bz)
				RegisterEthCall(queryClient, &evmtypes.EthCallRequest{Args: argsBz, ChainId: suite.backend.chainID.Int64()})
			},
			rpctypes.BlockNumber(1),
			callArgs,
			&evmtypes.EthCallResponse{},
			true,
		},
	}

	for _, tc := range testCases {
		suite.Run(fmt.Sprintf("case %s", tc.name), func() {
			suite.SetupTest() // reset test and queries
			tc.registerMock()

			msgEthTx, err := suite.backend.DoCall(tc.callArgs, tc.blockNum, nil)

			if tc.expPass {
				suite.Require().Equal(tc.expEthTx, msgEthTx)
			} else {
				suite.Require().Error(err)
			}
		})
	}
}

func (suite *BackendTestSuite) TestGasPrice() {
	defaultGasPrice := (*hexutil.Big)(big.NewInt(1))
	validator := sdk.AccAddress(tests.GenerateAddress().Bytes())
	height := int64(1)
	testCases := []struct {
		name         string
		registerMock func()
		expGas       *hexutil.Big
		expPass      bool
	}{
		{
			"pass - get the default gas price",
			func() {
				var header metadata.MD
				client := suite.backend.clientCtx.Client.(*mocks.Client)
				queryClient := suite.backend.queryClient.QueryClient.(*mocks.EVMQueryClient)
				feeMarketClient := suite.backend.queryClient.FeeMarket.(*mocks.FeeMarketQueryClient)
				RegisterFeeMarketParams(feeMarketClient, height)
				RegisterParams(queryClient, &header, height)
				RegisterEmptyBlockResults(client, height)
				RegisterBlock(client, height, nil)
				RegisterBaseFee(queryClient, sdkmath.NewInt(1))
				RegisterValidatorAccount(queryClient, validator)
			},
			defaultGasPrice,
			true,
		},
		{
			"fail - can't get gasFee, FeeMarketParams error",
			func() {
				var header metadata.MD
				client := suite.backend.clientCtx.Client.(*mocks.Client)
				queryClient := suite.backend.queryClient.QueryClient.(*mocks.EVMQueryClient)
				feeMarketClient := suite.backend.queryClient.FeeMarket.(*mocks.FeeMarketQueryClient)
				RegisterFeeMarketParamsError(feeMarketClient, height)
				RegisterParams(queryClient, &header, height)
				RegisterEmptyBlockResults(client, height)
				RegisterBlock(client, height, nil)
				RegisterBaseFee(queryClient, sdkmath.NewInt(1))
				RegisterValidatorAccount(queryClient, validator)
			},
			defaultGasPrice,
			false,
		},
	}

	for _, tc := range testCases {
		suite.Run(fmt.Sprintf("case %s", tc.name), func() {
			suite.SetupTest() // reset test and queries
			tc.registerMock()

			gasPrice, err := suite.backend.GasPrice()
			if tc.expPass {
				suite.Require().Equal(tc.expGas, gasPrice)
			} else {
				suite.Require().Error(err)
			}
		})
	}
}

func (suite *BackendTestSuite) TestCreateAccessListCall() {
	_, bz := suite.buildEthereumTx()
	toAddr := tests.GenerateAddress()
	chainID := (*hexutil.Big)(suite.backend.chainID)
	callArgs := evmtypes.TransactionArgs{
		To:      &toAddr,
		ChainID: chainID,
	}
	argsBz, err := json.Marshal(callArgs)
	suite.Require().NoError(err)

	baseReq := &evmtypes.EthCallRequest{
		Args:    argsBz,
		GasCap:  suite.backend.RPCGasCap(),
		ChainId: suite.backend.chainID.Int64(),
	}

	successData := func(gasUsed uint64, vmErr string) []byte {
		al := evmtypes.AccessListResult{
			AccessList: ethtypes.AccessList{},
			GasUsed:    hexutil.Uint64(gasUsed),
			Error:      vmErr,
		}
		bz, _ := json.Marshal(al)
		return bz
	}

	testCases := []struct {
		name         string
		registerMock func()
		expResult    *evmtypes.AccessListResult
		expPass      bool
	}{
		{
			"fail - header not found",
			func() {
				client := suite.backend.clientCtx.Client.(*mocks.Client)
				height := int64(1)
				RegisterHeaderError(client, &height)
			},
			nil,
			false,
		},
		{
			"fail - grpc returns error",
			func() {
				client := suite.backend.clientCtx.Client.(*mocks.Client)
				queryClient := suite.backend.queryClient.QueryClient.(*mocks.EVMQueryClient)
				height := int64(1)
				RegisterHeader(client, &height, bz)
				RegisterCreateAccessListError(queryClient, baseReq)
			},
			nil,
			false,
		},
		{
			"pass - success with gas used hex-encoded",
			func() {
				client := suite.backend.clientCtx.Client.(*mocks.Client)
				queryClient := suite.backend.queryClient.QueryClient.(*mocks.EVMQueryClient)
				height := int64(1)
				RegisterHeader(client, &height, bz)
				RegisterCreateAccessList(queryClient, baseReq, successData(21000, ""))
			},
			&evmtypes.AccessListResult{
				AccessList: ethtypes.AccessList{},
				GasUsed:    hexutil.Uint64(21000),
				Error:      "",
			},
			true,
		},
		{
			"pass - vm error propagated in Error field",
			func() {
				client := suite.backend.clientCtx.Client.(*mocks.Client)
				queryClient := suite.backend.queryClient.QueryClient.(*mocks.EVMQueryClient)
				height := int64(1)
				RegisterHeader(client, &height, bz)
				RegisterCreateAccessList(queryClient, baseReq, successData(21000, "execution reverted"))
			},
			&evmtypes.AccessListResult{
				AccessList: ethtypes.AccessList{},
				GasUsed:    hexutil.Uint64(21000),
				Error:      "execution reverted",
			},
			true,
		},
	}

	for _, tc := range testCases {
		suite.Run(fmt.Sprintf("case %s", tc.name), func() {
			suite.SetupTest()
			tc.registerMock()

			result, err := suite.backend.CreateAccessListCall(callArgs, rpctypes.BlockNumber(1), nil)
			if tc.expPass {
				suite.Require().NoError(err)
				suite.Require().Equal(tc.expResult, result)
			} else {
				suite.Require().Error(err)
			}
		})
	}
}

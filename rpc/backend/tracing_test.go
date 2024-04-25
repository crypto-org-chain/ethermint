package backend

import (
	"encoding/json"
	"fmt"
	"math/big"

	tmlog "cosmossdk.io/log"
	abci "github.com/cometbft/cometbft/abci/types"
	tmrpctypes "github.com/cometbft/cometbft/rpc/core/types"
	"github.com/cometbft/cometbft/types"
	tmtypes "github.com/cometbft/cometbft/types"
	dbm "github.com/cosmos/cosmos-db"
	"github.com/cosmos/cosmos-sdk/crypto"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	ethtypes "github.com/ethereum/go-ethereum/core/types"
	"github.com/evmos/ethermint/crypto/ethsecp256k1"
	"github.com/evmos/ethermint/indexer"
	"github.com/evmos/ethermint/rpc/backend/mocks"
	rpctypes "github.com/evmos/ethermint/rpc/types"
	"github.com/evmos/ethermint/tests"
	evmtypes "github.com/evmos/ethermint/x/evm/types"
)

func (suite *BackendTestSuite) TestTraceTransaction() {
	msgEthereumTx, _ := suite.buildEthereumTx()
	msgEthereumTx2, _ := suite.buildEthereumTx()

	txHash := msgEthereumTx.AsTransaction().Hash()
	txHash2 := msgEthereumTx2.AsTransaction().Hash()

	priv, _ := ethsecp256k1.GenerateKey()
	from := common.BytesToAddress(priv.PubKey().Address().Bytes())

	queryClient := suite.backend.queryClient.QueryClient.(*mocks.EVMQueryClient)
	RegisterParamsWithoutHeader(queryClient, 1)

	armor := crypto.EncryptArmorPrivKey(priv, "", "eth_secp256k1")
	suite.backend.clientCtx.Keyring.ImportPrivKey("test_key", armor, "")
	ethSigner := ethtypes.LatestSigner(suite.backend.ChainConfig())

	txEncoder := suite.backend.clientCtx.TxConfig.TxEncoder()
	txDecoder := suite.backend.clientCtx.TxConfig.TxDecoder()

	msgEthereumTx.From = from.Bytes()
	msgEthereumTx.Sign(ethSigner, suite.signer)
	tx, err := msgEthereumTx.BuildTx(suite.backend.clientCtx.TxConfig.NewTxBuilder(), "aphoton")
	suite.Require().NoError(err)
	txBz, err := txEncoder(tx)
	suite.Require().NoError(err)

	msgEthereumTx2.From = from.Bytes()
	msgEthereumTx2.Sign(ethSigner, suite.signer)
	tx2, _ := msgEthereumTx2.BuildTx(suite.backend.clientCtx.TxConfig.NewTxBuilder(), "aphoton")
	txBz2, _ := txEncoder(tx2)

	{
		tx, err := txDecoder(txBz)
		suite.Require().NoError(err)
		msgEthereumTx = tx.GetMsgs()[0].(*evmtypes.MsgEthereumTx)
	}

	{
		tx, err := txDecoder(txBz)
		suite.Require().NoError(err)
		msgEthereumTx2 = tx.GetMsgs()[0].(*evmtypes.MsgEthereumTx)
	}

	testCases := []struct {
		name          string
		registerMock  func()
		block         *types.Block
		responseBlock []*abci.ExecTxResult
		expResult     interface{}
		expPass       bool
	}{
		{
			"fail - tx not found",
			func() {},
			&types.Block{Header: types.Header{Height: 1}, Data: types.Data{Txs: []types.Tx{}}},
			[]*abci.ExecTxResult{
				{
					Code: 0,
					Events: []abci.Event{
						{Type: evmtypes.EventTypeEthereumTx, Attributes: []abci.EventAttribute{
							{Key: "ethereumTxHash", Value: txHash.Hex()},
							{Key: "txIndex", Value: "0"},
							{Key: "amount", Value: "1000"},
							{Key: "txGasUsed", Value: "21000"},
							{Key: "txHash", Value: ""},
							{Key: "recipient", Value: "0x775b87ef5D82ca211811C1a02CE0fE0CA3a455d7"},
						}},
					},
				},
			},
			nil,
			false,
		},
		{
			"fail - block not found",
			func() {
				// var header metadata.MD
				client := suite.backend.clientCtx.Client.(*mocks.Client)
				RegisterBlockError(client, 1)
			},
			&types.Block{Header: types.Header{Height: 1}, Data: types.Data{Txs: []types.Tx{txBz}}},
			[]*abci.ExecTxResult{
				{
					Code: 0,
					Events: []abci.Event{
						{Type: evmtypes.EventTypeEthereumTx, Attributes: []abci.EventAttribute{
							{Key: "ethereumTxHash", Value: txHash.Hex()},
							{Key: "txIndex", Value: "0"},
							{Key: "amount", Value: "1000"},
							{Key: "txGasUsed", Value: "21000"},
							{Key: "txHash", Value: ""},
							{Key: "recipient", Value: "0x775b87ef5D82ca211811C1a02CE0fE0CA3a455d7"},
						}},
					},
				},
			},
			map[string]interface{}{"test": "hello"},
			false,
		},
		{
			"pass - transaction found in a block with multiple transactions",
			func() {
				queryClient := suite.backend.queryClient.QueryClient.(*mocks.EVMQueryClient)
				client := suite.backend.clientCtx.Client.(*mocks.Client)
				RegisterBlockMultipleTxs(client, 1, []types.Tx{txBz, txBz2})
				RegisterTraceTransactionWithPredecessors(queryClient, msgEthereumTx, []*evmtypes.MsgEthereumTx{msgEthereumTx})
			},
			&types.Block{Header: types.Header{Height: 1, ChainID: ChainID}, Data: types.Data{Txs: []types.Tx{txBz, txBz2}}},
			[]*abci.ExecTxResult{
				{
					Code: 0,
					Events: []abci.Event{
						{Type: evmtypes.EventTypeEthereumTx, Attributes: []abci.EventAttribute{
							{Key: "ethereumTxHash", Value: txHash.Hex()},
							{Key: "txIndex", Value: "0"},
							{Key: "amount", Value: "1000"},
							{Key: "txGasUsed", Value: "21000"},
							{Key: "txHash", Value: ""},
							{Key: "recipient", Value: "0x775b87ef5D82ca211811C1a02CE0fE0CA3a455d7"},
						}},
					},
				},
				{
					Code: 0,
					Events: []abci.Event{
						{Type: evmtypes.EventTypeEthereumTx, Attributes: []abci.EventAttribute{
							{Key: "ethereumTxHash", Value: txHash2.Hex()},
							{Key: "txIndex", Value: "1"},
							{Key: "amount", Value: "1000"},
							{Key: "txGasUsed", Value: "21000"},
							{Key: "txHash", Value: ""},
							{Key: "recipient", Value: "0x775b87ef5D82ca211811C1a02CE0fE0CA3a455d7"},
						}},
					},
				},
			},
			map[string]interface{}{"test": "hello"},
			true,
		},
		{
			"pass - transaction found",
			func() {
				queryClient := suite.backend.queryClient.QueryClient.(*mocks.EVMQueryClient)
				client := suite.backend.clientCtx.Client.(*mocks.Client)
				RegisterBlock(client, 1, txBz)
				RegisterTraceTransaction(queryClient, msgEthereumTx)
			},
			&types.Block{Header: types.Header{Height: 1}, Data: types.Data{Txs: []types.Tx{txBz}}},
			[]*abci.ExecTxResult{
				{
					Code: 0,
					Events: []abci.Event{
						{Type: evmtypes.EventTypeEthereumTx, Attributes: []abci.EventAttribute{
							{Key: "ethereumTxHash", Value: txHash.Hex()},
							{Key: "txIndex", Value: "0"},
							{Key: "amount", Value: "1000"},
							{Key: "txGasUsed", Value: "21000"},
							{Key: "txHash", Value: ""},
							{Key: "recipient", Value: "0x775b87ef5D82ca211811C1a02CE0fE0CA3a455d7"},
						}},
					},
				},
			},
			map[string]interface{}{"test": "hello"},
			true,
		},
	}

	for _, tc := range testCases {
		suite.Run(fmt.Sprintf("case %s", tc.name), func() {
			suite.SetupTest() // reset test and queries
			tc.registerMock()

			db := dbm.NewMemDB()
			suite.backend.indexer = indexer.NewKVIndexer(db, tmlog.NewNopLogger(), suite.backend.clientCtx)

			err := suite.backend.indexer.IndexBlock(tc.block, tc.responseBlock)
			suite.Require().NoError(err)
			txResult, err := suite.backend.TraceTransaction(txHash, nil)

			if tc.expPass {
				suite.Require().NoError(err)
				suite.Require().Equal(tc.expResult, txResult)
			} else {
				suite.Require().Error(err)
			}
		})
	}
}

func (suite *BackendTestSuite) TestTraceBlock() {
	msgEthTx, bz := suite.buildEthereumTx()
	emptyBlock := tmtypes.MakeBlock(1, []tmtypes.Tx{}, nil, nil)
	emptyBlock.ChainID = ChainID
	filledBlock := tmtypes.MakeBlock(1, []tmtypes.Tx{bz}, nil, nil)
	filledBlock.ChainID = ChainID
	resBlockEmpty := tmrpctypes.ResultBlock{Block: emptyBlock, BlockID: emptyBlock.LastBlockID}
	resBlockFilled := tmrpctypes.ResultBlock{Block: filledBlock, BlockID: filledBlock.LastBlockID}

	testCases := []struct {
		name            string
		registerMock    func()
		expTraceResults []*evmtypes.TxTraceResult
		resBlock        *tmrpctypes.ResultBlock
		config          *rpctypes.TraceConfig
		expPass         bool
	}{
		{
			"pass - no transaction returning empty array",
			func() {},
			[]*evmtypes.TxTraceResult{},
			&resBlockEmpty,
			&rpctypes.TraceConfig{},
			true,
		},
		{
			"fail - cannot unmarshal data",
			func() {
				queryClient := suite.backend.queryClient.QueryClient.(*mocks.EVMQueryClient)
				RegisterTraceBlock(queryClient, []*evmtypes.MsgEthereumTx{msgEthTx})
				client := suite.backend.clientCtx.Client.(*mocks.Client)
				RegisterBlockResults(client, 1)
			},
			[]*evmtypes.TxTraceResult{},
			&resBlockFilled,
			&rpctypes.TraceConfig{},
			false,
		},
	}

	for _, tc := range testCases {
		suite.Run(fmt.Sprintf("case %s", tc.name), func() {
			suite.SetupTest() // reset test and queries
			tc.registerMock()

			traceResults, err := suite.backend.TraceBlock(1, tc.config, tc.resBlock)

			if tc.expPass {
				suite.Require().NoError(err)
				suite.Require().Equal(tc.expTraceResults, traceResults)
			} else {
				suite.Require().Error(err)
			}
		})
	}
}

func (suite *BackendTestSuite) TestDebugTraceCall() {
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

	blockNum := rpctypes.NewBlockNumber(big.NewInt(1))

	testCases := []struct {
		name         string
		registerMock func()
		blockNum     rpctypes.BlockNumberOrHash
		callArgs     evmtypes.TransactionArgs
		expEthTx     interface{}
		expPass      bool
	}{
		{
			"pass",
			func() {
				client := suite.backend.clientCtx.Client.(*mocks.Client)
				queryClient := suite.backend.queryClient.QueryClient.(*mocks.EVMQueryClient)
				RegisterBlock(client, 1, bz)
				RegisterTraceCall(
					queryClient,
					&evmtypes.QueryTraceCallRequest{Args: argsBz, ChainId: suite.backend.chainID.Int64(), BlockNumber: 1},
					&evmtypes.QueryTraceCallResponse{Data: []byte("{}")},
				)
			},
			rpctypes.BlockNumberOrHash{
				BlockNumber: &blockNum,
			},
			callArgs,
			map[string]interface{}{},
			true,
		},
	}

	for _, tc := range testCases {
		suite.Run(fmt.Sprintf("case %s", tc.name), func() {
			suite.SetupTest()
			tc.registerMock()

			result, err := suite.backend.TraceCall(tc.callArgs, tc.blockNum, nil)

			if tc.expPass {
				suite.Require().Equal(tc.expEthTx, result)
				suite.Require().NoError(err)
			} else {
				suite.Require().Error(err)
			}
		})
	}
}

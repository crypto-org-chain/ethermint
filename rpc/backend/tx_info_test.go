package backend

import (
	"encoding/json"
	"fmt"
	"math/big"

	tmlog "cosmossdk.io/log/v2"
	sdkmath "cosmossdk.io/math"
	abci "github.com/cometbft/cometbft/abci/types"
	tmrpctypes "github.com/cometbft/cometbft/rpc/core/types"
	"github.com/cometbft/cometbft/types"
	dbm "github.com/cosmos/cosmos-db"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	ethtypes "github.com/ethereum/go-ethereum/core/types"
	"github.com/evmos/ethermint/indexer"
	"github.com/evmos/ethermint/rpc/backend/mocks"
	rpctypes "github.com/evmos/ethermint/rpc/types"
	"github.com/evmos/ethermint/tests"
	ethermint "github.com/evmos/ethermint/types"
	evmtypes "github.com/evmos/ethermint/x/evm/types"
	"github.com/holiman/uint256"
	"github.com/stretchr/testify/mock"
	"google.golang.org/grpc/metadata"
)

func (suite *BackendTestSuite) TestGetTransactionByHash() {
	msgEthereumTx, _ := suite.buildEthereumTx()
	txBz := suite.signAndEncodeEthTx(msgEthereumTx)
	txHash := msgEthereumTx.Hash()
	block := &types.Block{Header: types.Header{Height: 1, ChainID: "test"}, Data: types.Data{Txs: []types.Tx{txBz}}}
	responseDeliver := []*abci.ExecTxResult{
		{
			Code: 0,
			Events: []abci.Event{
				{Type: evmtypes.EventTypeEthereumTx, Attributes: []abci.EventAttribute{
					{Key: "ethereumTxHash", Value: txHash.Hex()},
					{Key: "txIndex", Value: "0"},
					{Key: "amount", Value: "1000"},
					{Key: "txGasUsed", Value: "21000"},
					{Key: "txHash", Value: ""},
					{Key: "recipient", Value: ""},
				}},
			},
		},
	}

	rpcTransaction, _ := rpctypes.NewRPCTransaction(msgEthereumTx, common.Hash{}, 0, 0, 0, big.NewInt(1), suite.backend.chainID)

	testCases := []struct {
		name         string
		registerMock func()
		tx           *evmtypes.MsgEthereumTx
		expRPCTx     *rpctypes.RPCTransaction
		expPass      bool
	}{
		{
			"fail - Block error",
			func() {
				client := suite.backend.clientCtx.Client.(*mocks.Client)
				RegisterBlockError(client, 1)
			},
			msgEthereumTx,
			rpcTransaction,
			false,
		},
		{
			"fail - Block Result error",
			func() {
				client := suite.backend.clientCtx.Client.(*mocks.Client)
				RegisterBlock(client, 1, txBz)
				RegisterBlockResultsError(client, 1)
			},
			msgEthereumTx,
			nil,
			true,
		},
		{
			"pass - Base fee error",
			func() {
				client := suite.backend.clientCtx.Client.(*mocks.Client)
				queryClient := suite.backend.queryClient.QueryClient.(*mocks.EVMQueryClient)
				RegisterBlock(client, 1, txBz)
				RegisterBlockResults(client, 1)
				RegisterBaseFeeError(queryClient)
			},
			msgEthereumTx,
			rpcTransaction,
			true,
		},
		{
			"pass - Transaction found and returned",
			func() {
				client := suite.backend.clientCtx.Client.(*mocks.Client)
				queryClient := suite.backend.queryClient.QueryClient.(*mocks.EVMQueryClient)
				RegisterBlock(client, 1, txBz)
				RegisterBlockResults(client, 1)
				RegisterBaseFee(queryClient, sdkmath.NewInt(1))
			},
			msgEthereumTx,
			rpcTransaction,
			true,
		},
	}

	for _, tc := range testCases {
		suite.Run(tc.name, func() {
			suite.SetupTest() // reset
			tc.registerMock()

			db := dbm.NewMemDB()
			suite.backend.indexer = indexer.NewKVIndexer(db, tmlog.NewNopLogger(), suite.backend.clientCtx)
			err := suite.backend.indexer.IndexBlock(block, responseDeliver)
			suite.Require().NoError(err)

			rpcTx, err := suite.backend.GetTransactionByHash(tc.tx.Hash())

			if tc.expPass {
				suite.Require().NoError(err)
				suite.Require().Equal(rpcTx, tc.expRPCTx)
				// mock block has zero time — BlockTimestamp must be nil, not a wrapped uint64.
				if rpcTx != nil {
					suite.Require().Nil(rpcTx.BlockTimestamp)
				}
			} else {
				suite.Require().Error(err)
			}
		})
	}
}

func (suite *BackendTestSuite) TestGetTransactionsByHashPending() {
	msgEthereumTx, bz := suite.buildEthereumTx()
	rpcTransaction, _ := rpctypes.NewRPCTransaction(msgEthereumTx, common.Hash{}, 0, 0, 0, big.NewInt(1), suite.backend.chainID)

	testCases := []struct {
		name         string
		registerMock func()
		tx           *evmtypes.MsgEthereumTx
		expRPCTx     *rpctypes.RPCTransaction
		expPass      bool
	}{
		{
			"fail - Pending transactions returns error",
			func() {
				client := suite.backend.clientCtx.Client.(*mocks.Client)
				RegisterUnconfirmedTxsError(client, nil)
			},
			msgEthereumTx,
			nil,
			true,
		},
		{
			"fail - Tx not found return nil",
			func() {
				client := suite.backend.clientCtx.Client.(*mocks.Client)
				RegisterUnconfirmedTxs(client, nil, nil)
			},
			msgEthereumTx,
			nil,
			true,
		},
		{
			"pass - Tx found and returned",
			func() {
				client := suite.backend.clientCtx.Client.(*mocks.Client)
				RegisterUnconfirmedTxs(client, nil, types.Txs{bz})
			},
			msgEthereumTx,
			rpcTransaction,
			true,
		},
	}

	for _, tc := range testCases {
		suite.Run(tc.name, func() {
			suite.SetupTest() // reset
			tc.registerMock()

			rpcTx, err := suite.backend.getTransactionByHashPending(tc.tx.Hash())

			if tc.expPass {
				suite.Require().NoError(err)
				suite.Require().Equal(rpcTx, tc.expRPCTx)
			} else {
				suite.Require().Error(err)
			}
		})
	}
}

func (suite *BackendTestSuite) TestGetTxByEthHash() {
	msgEthereumTx, bz := suite.buildEthereumTx()
	rpcTransaction, _ := rpctypes.NewRPCTransaction(msgEthereumTx, common.Hash{}, 0, 0, 0, big.NewInt(1), suite.backend.chainID)

	testCases := []struct {
		name         string
		registerMock func()
		tx           *evmtypes.MsgEthereumTx
		expRPCTx     *rpctypes.RPCTransaction
		expPass      bool
	}{
		{
			"fail - Indexer disabled can't find transaction",
			func() {
				suite.backend.indexer = nil
				client := suite.backend.clientCtx.Client.(*mocks.Client)
				query := fmt.Sprintf("%s.%s='%s'", evmtypes.TypeMsgEthereumTx, evmtypes.AttributeKeyEthereumTxHash, msgEthereumTx.Hash().Hex())
				RegisterTxSearch(client, query, bz)
			},
			msgEthereumTx,
			rpcTransaction,
			false,
		},
	}

	for _, tc := range testCases {
		suite.Run(tc.name, func() {
			suite.SetupTest() // reset
			tc.registerMock()

			rpcTx, err := suite.backend.GetTxByEthHash(tc.tx.Hash())

			if tc.expPass {
				suite.Require().NoError(err)
				suite.Require().Equal(rpcTx, tc.expRPCTx)
			} else {
				suite.Require().Error(err)
			}
		})
	}
}

func (suite *BackendTestSuite) TestGetTransactionByBlockHashAndIndex() {
	_, bz := suite.buildEthereumTx()

	testCases := []struct {
		name         string
		registerMock func()
		blockHash    common.Hash
		expRPCTx     *rpctypes.RPCTransaction
		expPass      bool
	}{
		{
			"pass - block not found",
			func() {
				client := suite.backend.clientCtx.Client.(*mocks.Client)
				RegisterBlockByHashError(client, common.Hash{}, bz)
			},
			common.Hash{},
			nil,
			true,
		},
		{
			"pass - Block results error",
			func() {
				client := suite.backend.clientCtx.Client.(*mocks.Client)
				RegisterBlockByHash(client, common.Hash{}, bz)
				RegisterBlockResultsError(client, 1)
			},
			common.Hash{},
			nil,
			true,
		},
	}

	for _, tc := range testCases {
		suite.Run(tc.name, func() {
			suite.SetupTest() // reset
			tc.registerMock()

			rpcTx, err := suite.backend.GetTransactionByBlockHashAndIndex(tc.blockHash, 1)

			if tc.expPass {
				suite.Require().NoError(err)
				suite.Require().Equal(rpcTx, tc.expRPCTx)
			} else {
				suite.Require().Error(err)
			}
		})
	}
}

func (suite *BackendTestSuite) TestGetTransactionByBlockAndIndex() {
	msgEthTx, bz := suite.buildEthereumTx()

	defaultBlock := types.MakeBlock(1, []types.Tx{bz}, nil, nil)
	defaultResponseDeliverTx := []*abci.ExecTxResult{
		{
			Code: 0,
			Events: []abci.Event{
				{Type: evmtypes.EventTypeEthereumTx, Attributes: []abci.EventAttribute{
					{Key: "ethereumTxHash", Value: msgEthTx.Hash().Hex()},
					{Key: "txIndex", Value: "0"},
					{Key: "amount", Value: "1000"},
					{Key: "txGasUsed", Value: "21000"},
					{Key: "txHash", Value: ""},
					{Key: "recipient", Value: ""},
				}},
			},
		},
	}

	txFromMsg, _ := rpctypes.NewTransactionFromMsg(
		msgEthTx,
		common.BytesToHash(defaultBlock.Hash().Bytes()),
		1,
		0,
		0,
		big.NewInt(1),
		suite.backend.chainID,
	)
	testCases := []struct {
		name         string
		registerMock func()
		block        *tmrpctypes.ResultBlock
		idx          hexutil.Uint
		expRPCTx     *rpctypes.RPCTransaction
		expPass      bool
	}{
		{
			"pass - block txs index out of bound ",
			func() {
				client := suite.backend.clientCtx.Client.(*mocks.Client)
				RegisterBlockResults(client, 1)
			},
			&tmrpctypes.ResultBlock{Block: types.MakeBlock(1, []types.Tx{bz}, nil, nil)},
			1,
			nil,
			true,
		},
		{
			"pass - Can't fetch base fee",
			func() {
				queryClient := suite.backend.queryClient.QueryClient.(*mocks.EVMQueryClient)
				client := suite.backend.clientCtx.Client.(*mocks.Client)
				RegisterBlockResults(client, 1)
				RegisterBaseFeeError(queryClient)
			},
			&tmrpctypes.ResultBlock{Block: defaultBlock},
			0,
			txFromMsg,
			true,
		},
		{
			"pass - Gets Tx by transaction index",
			func() {
				queryClient := suite.backend.queryClient.QueryClient.(*mocks.EVMQueryClient)
				client := suite.backend.clientCtx.Client.(*mocks.Client)
				db := dbm.NewMemDB()
				suite.backend.indexer = indexer.NewKVIndexer(db, tmlog.NewNopLogger(), suite.backend.clientCtx)
				txBz := suite.signAndEncodeEthTx(msgEthTx)
				block := &types.Block{Header: types.Header{Height: 1, ChainID: "test"}, Data: types.Data{Txs: []types.Tx{txBz}}}
				err := suite.backend.indexer.IndexBlock(block, defaultResponseDeliverTx)
				suite.Require().NoError(err)
				RegisterBlockResults(client, 1)
				RegisterBaseFee(queryClient, sdkmath.NewInt(1))
			},
			&tmrpctypes.ResultBlock{Block: defaultBlock},
			0,
			txFromMsg,
			true,
		},
		{
			"pass - returns the Ethereum format transaction by the Ethereum hash",
			func() {
				queryClient := suite.backend.queryClient.QueryClient.(*mocks.EVMQueryClient)
				client := suite.backend.clientCtx.Client.(*mocks.Client)
				RegisterBlockResults(client, 1)
				RegisterBaseFee(queryClient, sdkmath.NewInt(1))
			},
			&tmrpctypes.ResultBlock{Block: defaultBlock},
			0,
			txFromMsg,
			true,
		},
	}

	for _, tc := range testCases {
		suite.Run(tc.name, func() {
			suite.SetupTest() // reset
			tc.registerMock()

			rpcTx, err := suite.backend.GetTransactionByBlockAndIndex(tc.block, tc.idx)

			if tc.expPass {
				suite.Require().NoError(err)
				suite.Require().Equal(rpcTx, tc.expRPCTx)
				// mock block has zero time — BlockTimestamp must be nil, not a wrapped uint64.
				if rpcTx != nil {
					suite.Require().Nil(rpcTx.BlockTimestamp)
				}
			} else {
				suite.Require().Error(err)
			}
		})
	}
}

func (suite *BackendTestSuite) TestGetTransactionByBlockNumberAndIndex() {
	msgEthTx, bz := suite.buildEthereumTx()
	defaultBlock := types.MakeBlock(1, []types.Tx{bz}, nil, nil)
	txFromMsg, _ := rpctypes.NewTransactionFromMsg(
		msgEthTx,
		common.BytesToHash(defaultBlock.Hash().Bytes()),
		1,
		0,
		0,
		big.NewInt(1),
		suite.backend.chainID,
	)
	testCases := []struct {
		name         string
		registerMock func()
		blockNum     rpctypes.BlockNumber
		idx          hexutil.Uint
		expRPCTx     *rpctypes.RPCTransaction
		expPass      bool
	}{
		{
			"fail -  block not found return nil",
			func() {
				client := suite.backend.clientCtx.Client.(*mocks.Client)
				RegisterBlockError(client, 1)
			},
			0,
			0,
			nil,
			true,
		},
		{
			"pass - returns the transaction identified by block number and index",
			func() {
				client := suite.backend.clientCtx.Client.(*mocks.Client)
				queryClient := suite.backend.queryClient.QueryClient.(*mocks.EVMQueryClient)
				RegisterBlock(client, 1, bz)
				RegisterBlockResults(client, 1)
				RegisterBaseFee(queryClient, sdkmath.NewInt(1))
			},
			0,
			0,
			txFromMsg,
			true,
		},
	}

	for _, tc := range testCases {
		suite.Run(tc.name, func() {
			suite.SetupTest() // reset
			tc.registerMock()

			rpcTx, err := suite.backend.GetTransactionByBlockNumberAndIndex(tc.blockNum, tc.idx)
			if tc.expPass {
				suite.Require().NoError(err)
				suite.Require().Equal(rpcTx, tc.expRPCTx)
			} else {
				suite.Require().Error(err)
			}
		})
	}
}

func (suite *BackendTestSuite) TestGetTransactionByTxIndex() {
	_, bz := suite.buildEthereumTx()

	testCases := []struct {
		name         string
		registerMock func()
		height       int64
		index        uint
		expTxResult  *ethermint.TxResult
		expPass      bool
	}{
		{
			"fail - Ethereum tx with query not found",
			func() {
				client := suite.backend.clientCtx.Client.(*mocks.Client)
				suite.backend.indexer = nil
				RegisterTxSearch(client, "tx.height=0 AND ethereum_tx.txIndex=0", bz)
			},
			0,
			0,
			&ethermint.TxResult{},
			false,
		},
	}

	for _, tc := range testCases {
		suite.Run(tc.name, func() {
			suite.SetupTest() // reset
			tc.registerMock()

			txResults, err := suite.backend.GetTxByTxIndex(tc.height, tc.index)

			if tc.expPass {
				suite.Require().NoError(err)
				suite.Require().Equal(txResults, tc.expTxResult)
			} else {
				suite.Require().Error(err)
			}
		})
	}
}

func (suite *BackendTestSuite) TestQueryTendermintTxIndexer() {
	testCases := []struct {
		name         string
		registerMock func()
		txGetter     func(*rpctypes.ParsedTxs) *rpctypes.ParsedTx
		query        string
		expTxResult  *ethermint.TxResult
		expPass      bool
	}{
		{
			"fail - Ethereum tx with query not found",
			func() {
				client := suite.backend.clientCtx.Client.(*mocks.Client)
				RegisterTxSearchEmpty(client, "")
			},
			func(txs *rpctypes.ParsedTxs) *rpctypes.ParsedTx {
				return &rpctypes.ParsedTx{}
			},
			"",
			&ethermint.TxResult{},
			false,
		},
	}

	for _, tc := range testCases {
		suite.Run(tc.name, func() {
			suite.SetupTest() // reset
			tc.registerMock()

			txResults, err := suite.backend.queryTendermintTxIndexer(tc.query, tc.txGetter)

			if tc.expPass {
				suite.Require().NoError(err)
				suite.Require().Equal(txResults, tc.expTxResult)
			} else {
				suite.Require().Error(err)
			}
		})
	}
}

func (suite *BackendTestSuite) TestGetTransactionReceipt() {
	msgEthereumTx, _ := suite.buildEthereumTx()
	txBz := suite.signAndEncodeEthTx(msgEthereumTx)
	txHash := msgEthereumTx.Hash()

	testCases := []struct {
		name         string
		registerMock func()
		tx           *evmtypes.MsgEthereumTx
		block        *types.Block
		blockResult  []*abci.ExecTxResult
		expTxReceipt map[string]interface{}
		expPass      bool
	}{
		{
			"fail - Receipts do not match ",
			func() {
				var header metadata.MD
				queryClient := suite.backend.queryClient.QueryClient.(*mocks.EVMQueryClient)
				client := suite.backend.clientCtx.Client.(*mocks.Client)
				RegisterParams(queryClient, &header, 1)
				RegisterParamsWithoutHeader(queryClient, 1)
				RegisterBlock(client, 1, txBz)
				// Block results must include ethereum_tx events so buildReceiptEntriesFromBlock
				// can locate the tx; RegisterBlockResults (no events) is insufficient.
				client.On("BlockResults", rpctypes.ContextWithHeight(1), mock.AnythingOfType("*int64")).
					Return(&tmrpctypes.ResultBlockResults{
						Height: 1,
						TxsResults: []*abci.ExecTxResult{{
							Code:    0,
							GasUsed: 21000,
							Events: []abci.Event{
								{Type: evmtypes.EventTypeEthereumTx, Attributes: []abci.EventAttribute{
									{Key: evmtypes.AttributeKeyEthereumTxHash, Value: txHash.Hex()},
									{Key: evmtypes.AttributeKeyTxIndex, Value: "0"},
								}},
							},
						}},
					}, nil)
			},
			msgEthereumTx,
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
			map[string]interface{}(nil),
			false,
		},
	}

	for _, tc := range testCases {
		suite.Run(tc.name, func() {
			suite.SetupTest() // reset
			tc.registerMock()

			db := dbm.NewMemDB()
			suite.backend.indexer = indexer.NewKVIndexer(db, tmlog.NewNopLogger(), suite.backend.clientCtx)
			err := suite.backend.indexer.IndexBlock(tc.block, tc.blockResult)
			suite.Require().NoError(err)

			txReceipt, err := suite.backend.GetTransactionReceipt(tc.tx.Hash(), nil)
			if tc.expPass {
				suite.Require().NoError(err)
				suite.Require().Equal(txReceipt, tc.expTxReceipt)
			} else {
				suite.Require().NotEqual(txReceipt, tc.expTxReceipt)
			}
		})
	}
}

// TestGetTransactionReceipt_BlockScopedWhenIndexerOverwritten verifies that when
// the KV indexer has been overwritten to point a tx hash at a later block,
// block-scoped receipt queries still rebuild the receipt from the requested
// block data.
func (suite *BackendTestSuite) TestGetTransactionReceipt_BlockScopedWhenIndexerOverwritten() {
	txBz, txHash, block1, txResult := suite.indexSameTxInTwoBlocks()

	indexed, err := suite.backend.indexer.GetByTxHash(txHash)
	suite.Require().NoError(err)
	suite.Require().Equal(int64(2), indexed.Height)

	client := suite.backend.clientCtx.Client.(*mocks.Client)
	queryClient := suite.backend.queryClient.QueryClient.(*mocks.EVMQueryClient)
	var header metadata.MD
	RegisterParams(queryClient, &header, 1)
	RegisterParamsWithoutHeader(queryClient, 1)
	resBlock1, err := RegisterBlock(client, 1, txBz)
	suite.Require().NoError(err)
	blockRes1 := &tmrpctypes.ResultBlockResults{
		Height:     block1.Height,
		TxsResults: txResult,
	}
	client.On("BlockResults", rpctypes.ContextWithHeight(1), mock.AnythingOfType("*int64")).
		Return(blockRes1, nil)

	receipt, err := suite.backend.GetTransactionReceipt(txHash, resBlock1)
	suite.Require().NoError(err)
	suite.Require().NotNil(receipt)
	suite.Require().Equal(hexutil.Uint64(1), receipt["blockNumber"])

	blockNum := rpctypes.BlockNumber(1)
	receipts, err := suite.backend.GetBlockReceipts(rpctypes.BlockNumberOrHash{BlockNumber: &blockNum})
	suite.Require().NoError(err)
	suite.Require().Len(receipts, 1)
	suite.Require().Equal(hexutil.Uint64(1), receipts[0]["blockNumber"])
}

// TestGetTransactionReceipt_BlockScopedWhenBlockResultsFetchFails verifies that
// when rebuilding receipt data from a block-scoped query and fetching block
// results fails, the call returns nil without error.
func (suite *BackendTestSuite) TestGetTransactionReceipt_BlockScopedWhenBlockResultsFetchFails() {
	_, txHash, block1, _ := suite.indexSameTxInTwoBlocks()

	client := suite.backend.clientCtx.Client.(*mocks.Client)
	resBlock1 := &tmrpctypes.ResultBlock{Block: block1}
	RegisterBlockResultsError(client, 1)

	receipt, err := suite.backend.GetTransactionReceipt(txHash, resBlock1)
	suite.Require().NoError(err)
	suite.Require().Nil(receipt)
}

// indexSameTxInTwoBlocks builds one eth tx, indexes it in block 1 then block 2
// so that the KV indexer's hash→height mapping is overwritten to point at
// block 2. Returns the encoded tx bytes, its hash, block 1, and the shared
// tx-result slice used for both blocks.
func (suite *BackendTestSuite) indexSameTxInTwoBlocks() (types.Tx, common.Hash, *types.Block, []*abci.ExecTxResult) {
	msgEthereumTx, _ := suite.buildEthereumTx()
	txBz := suite.signAndEncodeEthTx(msgEthereumTx)
	txHash := msgEthereumTx.Hash()

	txResult := []*abci.ExecTxResult{
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
	}

	db := dbm.NewMemDB()
	suite.backend.indexer = indexer.NewKVIndexer(db, tmlog.NewNopLogger(), suite.backend.clientCtx)

	block1 := &types.Block{Header: types.Header{Height: 1, ChainID: ChainID}, Data: types.Data{Txs: []types.Tx{txBz}}}
	block2 := &types.Block{Header: types.Header{Height: 2, ChainID: ChainID}, Data: types.Data{Txs: []types.Tx{txBz}}}
	suite.Require().NoError(suite.backend.indexer.IndexBlock(block1, txResult))
	suite.Require().NoError(suite.backend.indexer.IndexBlock(block2, txResult))

	return txBz, txHash, block1, txResult
}

func (suite *BackendTestSuite) TestBuildReceiptFromBlock_BlockGasExceeded() {
	msgEthereumTx, txBz := suite.buildEthereumTxWithNonceAndGas(0, 45000)
	resBlock := &tmrpctypes.ResultBlock{
		Block: types.MakeBlock(1, []types.Tx{txBz}, nil, nil),
	}
	blockRes := &tmrpctypes.ResultBlockResults{
		Height: 1,
		TxsResults: []*abci.ExecTxResult{
			{
				Code: 11,
				Log:  rpctypes.ExceedBlockGasLimitError,
			},
		},
	}

	res, ethMsg, err := suite.findReceiptEntry(resBlock, blockRes, msgEthereumTx.Hash())
	suite.Require().NoError(err)
	suite.Require().NotNil(res)
	suite.Require().NotNil(ethMsg)
	suite.Require().Equal(msgEthereumTx.Hash(), ethMsg.Hash())
	suite.Require().Equal(int64(1), res.Height)
	suite.Require().Equal(uint32(0), res.TxIndex)
	suite.Require().Equal(uint32(0), res.MsgIndex)
	suite.Require().Equal(int32(0), res.EthTxIndex)
	suite.Require().True(res.Failed)
	suite.Require().Equal(msgEthereumTx.GetGas(), res.GasUsed)
	suite.Require().Equal(msgEthereumTx.GetGas(), res.CumulativeGasUsed)
}

func (suite *BackendTestSuite) TestBuildReceiptFromBlock_SuccessfulTx() {
	msgEthereumTx, txBz := suite.buildEthereumTxWithNonceAndGas(0, 100000)
	resBlock := &tmrpctypes.ResultBlock{
		Block: types.MakeBlock(1, []types.Tx{txBz}, nil, nil),
	}
	blockRes := &tmrpctypes.ResultBlockResults{
		Height: 1,
		TxsResults: []*abci.ExecTxResult{
			{
				Code:    0,
				GasUsed: 21000,
				Events: []abci.Event{
					{
						Type: evmtypes.EventTypeEthereumTx,
						Attributes: []abci.EventAttribute{
							{Key: evmtypes.AttributeKeyEthereumTxHash, Value: msgEthereumTx.Hash().Hex()},
							{Key: evmtypes.AttributeKeyTxIndex, Value: "0"},
						},
					},
				},
			},
		},
	}

	res, ethMsg, err := suite.findReceiptEntry(resBlock, blockRes, msgEthereumTx.Hash())
	suite.Require().NoError(err)
	suite.Require().NotNil(res)
	suite.Require().NotNil(ethMsg)
	suite.Require().Equal(msgEthereumTx.Hash(), ethMsg.Hash())
	suite.Require().Equal(uint64(21000), res.GasUsed)
	suite.Require().Equal(uint64(21000), res.CumulativeGasUsed)
	suite.Require().False(res.Failed)
}

func (suite *BackendTestSuite) TestBuildReceiptFromBlock_HashMiss() {
	// Build a real tx with valid events so an entry IS created, then query a different hash.
	// Without this, ParseTxResult would return empty results (no events) and the loop
	// would continue before hash-matching — testing missing events, not an actual hash miss.
	msgEthereumTx, txBz := suite.buildEthereumTxWithNonceAndGas(0, 100000)
	resBlock := &tmrpctypes.ResultBlock{
		Block: types.MakeBlock(1, []types.Tx{txBz}, nil, nil),
	}
	blockRes := &tmrpctypes.ResultBlockResults{
		Height: 1,
		TxsResults: []*abci.ExecTxResult{
			{
				Code:    0,
				GasUsed: 21000,
				Events: []abci.Event{
					{
						Type: evmtypes.EventTypeEthereumTx,
						Attributes: []abci.EventAttribute{
							{Key: evmtypes.AttributeKeyEthereumTxHash, Value: msgEthereumTx.Hash().Hex()},
							{Key: evmtypes.AttributeKeyTxIndex, Value: "0"},
						},
					},
				},
			},
		},
	}

	res, ethMsg, err := suite.findReceiptEntry(resBlock, blockRes, common.HexToHash("0x1234"))
	suite.Require().NoError(err)
	suite.Require().Nil(res)
	suite.Require().Nil(ethMsg)
}

func (suite *BackendTestSuite) TestBuildReceiptFromBlock_MixedBlock() {
	msg1, txBz1 := suite.buildEthereumTxWithNonceAndGas(0, 100000)
	msg2, txBz2 := suite.buildEthereumTxWithNonceAndGas(1, 35000)

	resBlock := &tmrpctypes.ResultBlock{
		Block: types.MakeBlock(1, []types.Tx{txBz1, {0x01}, txBz2}, nil, nil),
	}
	blockRes := &tmrpctypes.ResultBlockResults{
		Height: 1,
		TxsResults: []*abci.ExecTxResult{
			{
				Code:    0,
				GasUsed: 21000,
				Events: []abci.Event{
					{
						Type: evmtypes.EventTypeEthereumTx,
						Attributes: []abci.EventAttribute{
							{Key: evmtypes.AttributeKeyEthereumTxHash, Value: msg1.Hash().Hex()},
							{Key: evmtypes.AttributeKeyTxIndex, Value: "0"},
						},
					},
				},
			},
			{
				Code: 0,
			},
			{
				Code: 11,
				Log:  rpctypes.ExceedBlockGasLimitError,
			},
		},
	}

	res, ethMsg, err := suite.findReceiptEntry(resBlock, blockRes, msg2.Hash())
	suite.Require().NoError(err)
	suite.Require().NotNil(res)
	suite.Require().Equal(msg2.Hash(), ethMsg.Hash())
	suite.Require().Equal(uint32(2), res.TxIndex)
	suite.Require().Equal(int32(1), res.EthTxIndex)
	suite.Require().True(res.Failed)
	suite.Require().Equal(msg2.GetGas(), res.GasUsed)
}

func (suite *BackendTestSuite) TestGetBlockReceipts_BlockGasExceededWithoutIndexer() {
	msg1, txBz1 := suite.buildEthereumTxWithNonceAndGas(0, 100000)
	msg2, txBz2 := suite.buildEthereumTxWithNonceAndGas(1, 35000)

	client := suite.backend.clientCtx.Client.(*mocks.Client)
	queryClient := suite.backend.queryClient.QueryClient.(*mocks.EVMQueryClient)
	var header metadata.MD
	RegisterParams(queryClient, &header, 1)
	RegisterParamsWithoutHeader(queryClient, 1)
	_, err := RegisterBlockMultipleTxs(client, 1, []types.Tx{txBz1, txBz2})
	suite.Require().NoError(err)

	blockRes := &tmrpctypes.ResultBlockResults{
		Height: 1,
		TxsResults: []*abci.ExecTxResult{
			{
				Code:    0,
				GasUsed: 21000,
				Events: []abci.Event{
					{
						Type: evmtypes.EventTypeEthereumTx,
						Attributes: []abci.EventAttribute{
							{Key: evmtypes.AttributeKeyEthereumTxHash, Value: msg1.Hash().Hex()},
							{Key: evmtypes.AttributeKeyTxIndex, Value: "0"},
						},
					},
				},
			},
			{
				Code: 11,
				Log:  rpctypes.ExceedBlockGasLimitError,
			},
		},
	}
	client.On("BlockResults", rpctypes.ContextWithHeight(1), mock.AnythingOfType("*int64")).
		Return(blockRes, nil)
	suite.backend.indexer = nil

	blockNum := rpctypes.BlockNumber(1)
	receipts, err := suite.backend.GetBlockReceipts(rpctypes.BlockNumberOrHash{BlockNumber: &blockNum})
	suite.Require().NoError(err)
	suite.Require().Len(receipts, 2)

	var exceededReceipt map[string]interface{}
	for _, receipt := range receipts {
		if receipt["transactionHash"] == msg2.Hash() {
			exceededReceipt = receipt
			break
		}
	}

	suite.Require().NotNil(exceededReceipt)
	suite.Require().Equal(hexutil.Uint(ethtypes.ReceiptStatusFailed), exceededReceipt["status"])
	suite.Require().Equal(hexutil.Uint64(msg2.GetGas()), exceededReceipt["gasUsed"])
	suite.Require().Equal(hexutil.Uint64(21000+msg2.GetGas()), exceededReceipt["cumulativeGasUsed"])
}

func (suite *BackendTestSuite) TestGetBlockReceipts_IgnoresIndexerHashMismatch() {
	msgEthereumTx, txBz := suite.buildEthereumTxWithNonceAndGas(0, 45000)

	client := suite.backend.clientCtx.Client.(*mocks.Client)
	queryClient := suite.backend.queryClient.QueryClient.(*mocks.EVMQueryClient)
	var header metadata.MD
	RegisterParams(queryClient, &header, 1)
	RegisterParamsWithoutHeader(queryClient, 1)
	_, err := RegisterBlockMultipleTxs(client, 1, []types.Tx{txBz})
	suite.Require().NoError(err)

	blockRes := &tmrpctypes.ResultBlockResults{
		Height: 1,
		TxsResults: []*abci.ExecTxResult{
			{
				Code: 11,
				Log:  rpctypes.ExceedBlockGasLimitError,
			},
		},
	}
	client.On("BlockResults", rpctypes.ContextWithHeight(1), mock.AnythingOfType("*int64")).
		Return(blockRes, nil)
	suite.backend.indexer = failingLookupIndexer{}

	blockNum := rpctypes.BlockNumber(1)
	receipts, err := suite.backend.GetBlockReceipts(rpctypes.BlockNumberOrHash{BlockNumber: &blockNum})
	suite.Require().NoError(err)
	suite.Require().Len(receipts, 1)
	suite.Require().Equal(msgEthereumTx.Hash(), receipts[0]["transactionHash"])
}

func (suite *BackendTestSuite) TestGetBlockReceipts_ByHash() {
	msgEthereumTx, txBz := suite.buildEthereumTxWithNonceAndGas(0, 45000)
	hash := common.HexToHash("0x1234567890abcdef1234567890abcdef1234567890abcdef1234567890abcdef")

	client := suite.backend.clientCtx.Client.(*mocks.Client)
	queryClient := suite.backend.queryClient.QueryClient.(*mocks.EVMQueryClient)
	var header metadata.MD
	RegisterParams(queryClient, &header, 1)
	RegisterParamsWithoutHeader(queryClient, 1)
	RegisterBlockByHash(client, hash, txBz)

	blockRes := &tmrpctypes.ResultBlockResults{
		Height: 1,
		TxsResults: []*abci.ExecTxResult{
			{
				Code: 11,
				Log:  rpctypes.ExceedBlockGasLimitError,
			},
		},
	}
	client.On("BlockResults", rpctypes.ContextWithHeight(1), mock.AnythingOfType("*int64")).
		Return(blockRes, nil)

	receipts, err := suite.backend.GetBlockReceipts(rpctypes.BlockNumberOrHash{BlockHash: &hash})
	suite.Require().NoError(err)
	suite.Require().Len(receipts, 1)
	suite.Require().Equal(msgEthereumTx.Hash(), receipts[0]["transactionHash"])
}

func (suite *BackendTestSuite) TestGetBlockReceipts_EmptyBlockNumberOrHashDefaultsToLatest() {
	msgEthereumTx, txBz := suite.buildEthereumTxWithNonceAndGas(0, 45000)

	client := suite.backend.clientCtx.Client.(*mocks.Client)
	queryClient := suite.backend.queryClient.QueryClient.(*mocks.EVMQueryClient)
	var header metadata.MD
	RegisterParams(queryClient, &header, 1)
	RegisterParamsWithoutHeader(queryClient, 1)
	_, err := RegisterBlockMultipleTxs(client, 1, []types.Tx{txBz})
	suite.Require().NoError(err)

	blockRes := &tmrpctypes.ResultBlockResults{
		Height: 1,
		TxsResults: []*abci.ExecTxResult{
			{
				Code: 11,
				Log:  rpctypes.ExceedBlockGasLimitError,
			},
		},
	}
	client.On("BlockResults", rpctypes.ContextWithHeight(1), mock.AnythingOfType("*int64")).
		Return(blockRes, nil)

	receipts, err := suite.backend.GetBlockReceipts(rpctypes.BlockNumberOrHash{})
	suite.Require().NoError(err)
	suite.Require().Len(receipts, 1)
	suite.Require().Equal(msgEthereumTx.Hash(), receipts[0]["transactionHash"])
}

func (suite *BackendTestSuite) TestBuildReceiptDirect_SetCodeTxEffectiveGasPrice() {
	msgSetCodeTx := suite.buildSetCodeTx()

	queryClient := suite.backend.queryClient.QueryClient.(*mocks.EVMQueryClient)
	var header metadata.MD
	RegisterParams(queryClient, &header, 1)
	RegisterParamsWithoutHeader(queryClient, 1)
	RegisterBaseFee(queryClient, sdkmath.NewInt(1))

	block := &tmrpctypes.ResultBlock{
		Block: types.MakeBlock(1, nil, nil, nil),
	}
	blockResults := &tmrpctypes.ResultBlockResults{
		Height: 1,
		TxsResults: []*abci.ExecTxResult{
			{
				Code:    0,
				GasUsed: 21000,
			},
		},
	}
	txResult := &ethermint.TxResult{
		Height:            1,
		TxIndex:           0,
		MsgIndex:          0,
		EthTxIndex:        0,
		GasUsed:           21000,
		CumulativeGasUsed: 21000,
	}

	receipt, err := suite.backend.buildReceiptDirect(block, blockResults, txResult, msgSetCodeTx)
	suite.Require().NoError(err)
	suite.Require().Equal((*hexutil.Big)(big.NewInt(10001)), receipt["effectiveGasPrice"])
}

// TestBuildReceiptDirect_EIP1559_NilBaseFee verifies that effectiveGasPrice is
// omitted when BaseFee is unavailable, not returned as GasFeeCap.
func (suite *BackendTestSuite) TestBuildReceiptDirect_EIP1559_NilBaseFee() {
	msgSetCodeTx := suite.buildSetCodeTx()

	queryClient := suite.backend.queryClient.QueryClient.(*mocks.EVMQueryClient)
	var header metadata.MD
	RegisterParams(queryClient, &header, 1)
	RegisterParamsWithoutHeader(queryClient, 1)
	RegisterBaseFeeError(queryClient) // pruned node / disabled fee market

	block := &tmrpctypes.ResultBlock{
		Block: types.MakeBlock(1, nil, nil, nil),
	}
	blockResults := &tmrpctypes.ResultBlockResults{
		Height: 1,
		TxsResults: []*abci.ExecTxResult{
			{Code: 0, GasUsed: 21000},
		},
	}
	txResult := &ethermint.TxResult{
		Height: 1, TxIndex: 0, MsgIndex: 0, EthTxIndex: 0,
		GasUsed: 21000, CumulativeGasUsed: 21000,
	}

	receipt, err := suite.backend.buildReceiptDirect(block, blockResults, txResult, msgSetCodeTx)
	suite.Require().NoError(err)
	_, present := receipt["effectiveGasPrice"]
	suite.Require().False(present, "effectiveGasPrice must be omitted when baseFee is unavailable for EIP-1559 tx")
}

func (suite *BackendTestSuite) TestGetGasUsed() {
	origin := suite.backend.cfg.JSONRPC.FixRevertGasRefundHeight
	testCases := []struct {
		name                     string
		fixRevertGasRefundHeight int64
		txResult                 *ethermint.TxResult
		gas                      uint64
		exp                      uint64
	}{
		{
			"success txResult",
			1,
			&ethermint.TxResult{
				Height:  1,
				Failed:  false,
				GasUsed: 53026,
			},
			0,
			53026,
		},
		{
			"fail txResult before cap",
			2,
			&ethermint.TxResult{
				Height:  1,
				Failed:  true,
				GasUsed: 53026,
			},
			5000000000000,
			5000000000000,
		},
		{
			"fail txResult after cap",
			2,
			&ethermint.TxResult{
				Height:  3,
				Failed:  true,
				GasUsed: 53026,
			},
			5000000000000,
			53026,
		},
	}
	for _, tc := range testCases {
		suite.Run(fmt.Sprintf("Case %s", tc.name), func() {
			suite.backend.cfg.JSONRPC.FixRevertGasRefundHeight = tc.fixRevertGasRefundHeight
			suite.Require().Equal(tc.exp, suite.backend.GetGasUsed(tc.txResult, tc.gas))
			suite.backend.cfg.JSONRPC.FixRevertGasRefundHeight = origin
		})
	}
}

type failingLookupIndexer struct{}

func (failingLookupIndexer) LastIndexedBlock() (int64, error) {
	return -1, nil
}

func (failingLookupIndexer) IndexBlock(*types.Block, []*abci.ExecTxResult) error {
	return nil
}

func (failingLookupIndexer) GetByTxHash(common.Hash) (*ethermint.TxResult, error) {
	panic("unexpected indexer GetByTxHash call")
}

func (failingLookupIndexer) GetByBlockAndIndex(int64, int32) (*ethermint.TxResult, error) {
	panic("unexpected indexer GetByBlockAndIndex call")
}

func (suite *BackendTestSuite) TestGetTransactionByHash_SetCodeTxType() {
	msgSetCodeTx := suite.buildSetCodeTx()
	txBz := suite.signAndEncodeEthTx(msgSetCodeTx)
	txHash := msgSetCodeTx.Hash()
	block := &types.Block{Header: types.Header{Height: 1, ChainID: "test"}, Data: types.Data{Txs: []types.Tx{txBz}}}
	responseDeliver := []*abci.ExecTxResult{
		{
			Code: 0,
			Events: []abci.Event{
				{Type: evmtypes.EventTypeEthereumTx, Attributes: []abci.EventAttribute{
					{Key: "ethereumTxHash", Value: txHash.Hex()},
					{Key: "txIndex", Value: "0"},
					{Key: "amount", Value: "0"},
					{Key: "txGasUsed", Value: "100000"},
					{Key: "txHash", Value: ""},
					{Key: "recipient", Value: "0x742d35cc6561c9d8f6b1b8e6e2c8b9f8f4a1e2d3"},
				}},
			},
		},
	}

	expectedRPCTx, _ := rpctypes.NewRPCTransaction(msgSetCodeTx, common.Hash{}, 0, 0, 0, big.NewInt(1), suite.backend.chainID)

	testCases := []struct {
		name         string
		registerMock func()
		expPass      bool
	}{
		{
			"pass - SetCodeTx transaction found and returned",
			func() {
				client := suite.backend.clientCtx.Client.(*mocks.Client)
				queryClient := suite.backend.queryClient.QueryClient.(*mocks.EVMQueryClient)
				RegisterBlock(client, 1, txBz)
				RegisterBlockResults(client, 1)
				RegisterBaseFee(queryClient, sdkmath.NewInt(1))
			},
			true,
		},
	}

	for _, tc := range testCases {
		suite.Run(tc.name, func() {
			suite.SetupTest()
			tc.registerMock()

			db := dbm.NewMemDB()
			suite.backend.indexer = indexer.NewKVIndexer(db, tmlog.NewNopLogger(), suite.backend.clientCtx)
			err := suite.backend.indexer.IndexBlock(block, responseDeliver)
			suite.Require().NoError(err)

			rpcTx, err := suite.backend.GetTransactionByHash(txHash)

			if tc.expPass {
				suite.Require().NoError(err)
				suite.Require().NotNil(rpcTx)

				suite.Require().Equal(hexutil.Uint64(ethtypes.SetCodeTxType), rpcTx.Type)

				suite.Require().NotNil(rpcTx.GasFeeCap, "GasFeeCap should be set")
				suite.Require().NotNil(rpcTx.GasTipCap, "GasTipCap should be set")
				suite.Require().NotNil(rpcTx.ChainID, "ChainID should be set")
				suite.Require().NotNil(rpcTx.Accesses, "AccessList should be set")
				suite.Require().NotNil(rpcTx.AuthorizationList, "AuthorizationList should be set")

				suite.Require().Equal(expectedRPCTx.Type, rpcTx.Type)
				suite.Require().Equal(expectedRPCTx.From, rpcTx.From)
				suite.Require().Equal(expectedRPCTx.Hash, rpcTx.Hash)
			} else {
				suite.Require().Error(err)
			}
		})
	}
}

func (suite *BackendTestSuite) buildSetCodeTx() *evmtypes.MsgEthereumTx {
	auth := ethtypes.SetCodeAuthorization{
		ChainID: *uint256.MustFromBig(suite.backend.chainID),
		Address: common.HexToAddress("0x4Cd241E8d1510e30b2076397afc7508Ae59C66c9"),
		Nonce:   1,
		V:       uint8(27),
		R:       *uint256.NewInt(1),
		S:       *uint256.NewInt(1),
	}

	setCodeTx := &ethtypes.SetCodeTx{
		ChainID:    uint256.MustFromBig(suite.backend.chainID),
		Nonce:      0,
		GasTipCap:  uint256.NewInt(10000),
		GasFeeCap:  uint256.NewInt(1000000000000),
		Gas:        100000,
		To:         common.HexToAddress("0x742d35cc6561c9d8f6b1b8e6e2c8b9f8f4a1e2d3"),
		Value:      uint256.NewInt(0),
		Data:       []byte{},
		AccessList: ethtypes.AccessList{},
		AuthList:   []ethtypes.SetCodeAuthorization{auth},
		V:          uint256.NewInt(1),
		R:          uint256.NewInt(1),
		S:          uint256.NewInt(1),
	}

	ethTx := ethtypes.NewTx(setCodeTx)
	msgEthereumTx := &evmtypes.MsgEthereumTx{}
	err := msgEthereumTx.FromSignedEthereumTx(ethTx, ethtypes.LatestSignerForChainID(suite.backend.chainID))
	suite.Require().NoError(err)

	msgEthereumTx.From = suite.signerAddress

	return msgEthereumTx
}

func (suite *BackendTestSuite) buildEthereumTxWithNonceAndGas(nonce uint64, gasLimit uint64) (*evmtypes.MsgEthereumTx, []byte) {
	msgEthereumTx := evmtypes.NewTx(
		suite.backend.chainID,
		nonce,
		&common.Address{},
		big.NewInt(0),
		gasLimit,
		big.NewInt(1),
		nil,
		nil,
		nil,
		nil,
	)
	msgEthereumTx.From = suite.signerAddress
	err := msgEthereumTx.Sign(ethtypes.LatestSignerForChainID(suite.backend.chainID), suite.signer)
	suite.Require().NoError(err)

	tx, err := msgEthereumTx.BuildTx(suite.backend.clientCtx.TxConfig.NewTxBuilder(), "aphoton")
	suite.Require().NoError(err)

	bz, err := suite.backend.clientCtx.TxConfig.TxEncoder()(tx)
	suite.Require().NoError(err)

	sdkTx, err := suite.backend.clientCtx.TxConfig.TxDecoder()(bz)
	suite.Require().NoError(err)

	return sdkTx.GetMsgs()[0].(*evmtypes.MsgEthereumTx), bz
}

// findReceiptEntry is a test helper that builds all receipt entries from block
// data and returns the one matching the given hash, or nil if not found.
func (suite *BackendTestSuite) findReceiptEntry(
	resBlock *tmrpctypes.ResultBlock,
	blockRes *tmrpctypes.ResultBlockResults,
	hash common.Hash,
) (*ethermint.TxResult, *evmtypes.MsgEthereumTx, error) {
	entries, err := suite.backend.collectReceiptEntriesFromBlock(resBlock, blockRes, nil)
	if err != nil {
		return nil, nil, err
	}
	for _, entry := range entries {
		if entry.hash == hash {
			return entry.txResult, entry.ethMsg, nil
		}
	}
	return nil, nil, nil
}

func (suite *BackendTestSuite) TestCreateAccessList() {
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

	makeData := func(gasUsed uint64, vmErr string) []byte {
		al := evmtypes.AccessListResult{AccessList: ethtypes.AccessList{}, GasUsed: hexutil.Uint64(gasUsed), Error: vmErr}
		bz, _ := json.Marshal(al)
		return bz
	}

	testCases := []struct {
		name         string
		registerMock func()
		expResult    *rpctypes.AccessListResult
		expPass      bool
	}{
		{
			"fail - block number resolution fails",
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
			"pass - result fields correctly mapped",
			func() {
				client := suite.backend.clientCtx.Client.(*mocks.Client)
				queryClient := suite.backend.queryClient.QueryClient.(*mocks.EVMQueryClient)
				height := int64(1)
				RegisterHeader(client, &height, bz)
				RegisterCreateAccessList(queryClient, baseReq, makeData(21000, ""))
			},
			&rpctypes.AccessListResult{
				AccessList: ethtypes.AccessList{},
				GasUsed:    hexutil.Uint64(21000),
			},
			true,
		},
		{
			"pass - vm error propagated to Error field",
			func() {
				client := suite.backend.clientCtx.Client.(*mocks.Client)
				queryClient := suite.backend.queryClient.QueryClient.(*mocks.EVMQueryClient)
				height := int64(1)
				RegisterHeader(client, &height, bz)
				RegisterCreateAccessList(queryClient, baseReq, makeData(5000, "execution reverted"))
			},
			&rpctypes.AccessListResult{
				AccessList: ethtypes.AccessList{},
				GasUsed:    hexutil.Uint64(5000),
				Error:      "execution reverted",
			},
			true,
		},
	}

	for _, tc := range testCases {
		suite.Run(fmt.Sprintf("case %s", tc.name), func() {
			suite.SetupTest()
			tc.registerMock()

			blockNrOrHash := rpctypes.BlockNumberOrHash{BlockNumber: func() *rpctypes.BlockNumber {
				n := rpctypes.BlockNumber(1)
				return &n
			}()}
			result, err := suite.backend.CreateAccessList(callArgs, blockNrOrHash, nil)
			if tc.expPass {
				suite.Require().NoError(err)
				suite.Require().Equal(tc.expResult, result)
			} else {
				suite.Require().Error(err)
			}
		})
	}
}

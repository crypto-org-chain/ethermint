package backend

import (
	"fmt"
	"math/big"

	tmlog "cosmossdk.io/log"
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
	ethermint "github.com/evmos/ethermint/types"
	evmtypes "github.com/evmos/ethermint/x/evm/types"
	"github.com/holiman/uint256"
	mock "github.com/stretchr/testify/mock"
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

	rpcTransaction, _ := rpctypes.NewRPCTransaction(msgEthereumTx, common.Hash{}, 0, 0, big.NewInt(1), suite.backend.chainID)

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
			} else {
				suite.Require().Error(err)
			}
		})
	}
}

func (suite *BackendTestSuite) TestGetTransactionsByHashPending() {
	msgEthereumTx, bz := suite.buildEthereumTx()
	rpcTransaction, _ := rpctypes.NewRPCTransaction(msgEthereumTx, common.Hash{}, 0, 0, big.NewInt(1), suite.backend.chainID)

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
	rpcTransaction, _ := rpctypes.NewRPCTransaction(msgEthereumTx, common.Hash{}, 0, 0, big.NewInt(1), suite.backend.chainID)

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
				RegisterBlockResults(client, 1)
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

			txReceipt, err := suite.backend.GetTransactionReceipt(tc.tx.Hash(), nil, nil)
			if tc.expPass {
				suite.Require().NoError(err)
				suite.Require().Equal(txReceipt, tc.expTxReceipt)
			} else {
				suite.Require().NotEqual(txReceipt, tc.expTxReceipt)
			}
		})
	}
}

// TestGetTransactionReceipt_BlockScopedWhenIndexerOverwritten documents the fix for duplicate eth tx hashes:
// the KV indexer only stores the latest inclusion per hash. GetTransactionReceipt with an explicit block
// still uses the indexer-height guard (returns nil if overwritten), while GetBlockReceipts uses a block-walk
// and returns the receipt physically present in the block regardless of indexer state.
func (suite *BackendTestSuite) TestGetTransactionReceipt_BlockScopedWhenIndexerOverwritten() {
	suite.SetupTest()

	msgEthereumTx, txBz := suite.buildEthereumTx()
	txHash := msgEthereumTx.Hash()

	execTx := &abci.ExecTxResult{
		Code:    0,
		GasUsed: 21000,
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
	}

	block1 := &types.Block{Header: types.Header{Height: 1}, Data: types.Data{Txs: []types.Tx{txBz}}}
	block2 := &types.Block{Header: types.Header{Height: 2}, Data: types.Data{Txs: []types.Tx{txBz}}}

	db := dbm.NewMemDB()
	suite.backend.indexer = indexer.NewKVIndexer(db, tmlog.NewNopLogger(), suite.backend.clientCtx)
	suite.Require().NoError(suite.backend.indexer.IndexBlock(block1, []*abci.ExecTxResult{execTx}))
	r1, err := suite.backend.indexer.GetByTxHash(txHash)
	suite.Require().NoError(err)
	suite.Require().Equal(int64(1), r1.Height)

	suite.Require().NoError(suite.backend.indexer.IndexBlock(block2, []*abci.ExecTxResult{execTx}))
	r2, err := suite.backend.indexer.GetByTxHash(txHash)
	suite.Require().NoError(err)
	suite.Require().Equal(int64(2), r2.Height)

	client := suite.backend.clientCtx.Client.(*mocks.Client)
	queryClient := suite.backend.queryClient.QueryClient.(*mocks.EVMQueryClient)

	resBlock1, err := RegisterBlock(client, 1, txBz)
	suite.Require().NoError(err)

	// TendermintBlockByNumber / BlockResults use b.ctx (ContextWithHeight(1) from SetupTest), not ContextWithHeight(requestedHeight).
	blockRes1 := &tmrpctypes.ResultBlockResults{Height: 1, TxsResults: []*abci.ExecTxResult{execTx}}
	client.On("BlockResults", rpctypes.ContextWithHeight(1), mock.AnythingOfType("*int64")).Return(blockRes1, nil)

	var header metadata.MD
	RegisterParams(queryClient, &header, 1)
	RegisterParamsWithoutHeader(queryClient, 1)

	receipt, err := suite.backend.GetTransactionReceipt(txHash, resBlock1, blockRes1)
	suite.Require().NoError(err)
	suite.Require().Nil(receipt) // overwritten index → skip, receipt belongs to block 2

	// GetBlockReceipts uses block-walk and returns the receipt physically in the block,
	// regardless of what the KV indexer says.
	receipts, err := suite.backend.GetBlockReceipts(rpctypes.BlockNumber(1))
	suite.Require().NoError(err)
	suite.Require().Len(receipts, 1)
}

// TestGetBlockReceipts_BlockGasExceeded verifies that eth_getBlockReceipts includes
// a tx that failed with "exceeds block gas limit" (KV indexer may not have it).
func (suite *BackendTestSuite) TestGetBlockReceipts_BlockGasExceeded() {
	suite.SetupTest()

	msgEthereumTx, txBz := suite.buildEthereumTx()
	txHash := msgEthereumTx.Hash()

	// ExecTxResult with Code=11 (out of block gas): no ethereum_tx events.
	execTx := &abci.ExecTxResult{
		Code:    11,
		Log:     "out of gas in location: block gas meter; gasWanted: 21000",
		GasUsed: 21000,
		Events:  []abci.Event{},
	}

	block := &types.Block{Header: types.Header{Height: 1}, Data: types.Data{Txs: []types.Tx{txBz}}}

	db := dbm.NewMemDB()
	suite.backend.indexer = indexer.NewKVIndexer(db, tmlog.NewNopLogger(), suite.backend.clientCtx)
	suite.Require().NoError(suite.backend.indexer.IndexBlock(block, []*abci.ExecTxResult{execTx}))
	// Verify the tx hash is indexed (kv_indexer indexes block-gas-exceeded txs directly).
	res, err := suite.backend.indexer.GetByTxHash(txHash)
	suite.Require().NoError(err)
	suite.Require().NotNil(res)

	client := suite.backend.clientCtx.Client.(*mocks.Client)
	queryClient := suite.backend.queryClient.QueryClient.(*mocks.EVMQueryClient)
	_, err = RegisterBlock(client, 1, txBz)
	suite.Require().NoError(err)

	blockRes := &tmrpctypes.ResultBlockResults{Height: 1, TxsResults: []*abci.ExecTxResult{execTx}}
	client.On("BlockResults", rpctypes.ContextWithHeight(1), mock.AnythingOfType("*int64")).Return(blockRes, nil)

	var header metadata.MD
	RegisterParams(queryClient, &header, 1)
	RegisterParamsWithoutHeader(queryClient, 1)

	receipts, err := suite.backend.GetBlockReceipts(rpctypes.BlockNumber(1))
	suite.Require().NoError(err)
	suite.Require().Len(receipts, 1)
	suite.Require().Equal(hexutil.Uint(0), receipts[0]["status"]) // ReceiptStatusFailed
	suite.Require().Equal(txHash, receipts[0]["transactionHash"])
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

	expectedRPCTx, _ := rpctypes.NewRPCTransaction(msgSetCodeTx, common.Hash{}, 0, 0, big.NewInt(1), suite.backend.chainID)

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

package backend

import (
	"context"
	"errors"
	"math"
	"net/http"
	"time"

	tmlog "cosmossdk.io/log/v2"
	abci "github.com/cometbft/cometbft/abci/types"
	cmtmempool "github.com/cometbft/cometbft/mempool"
	tmrpctypes "github.com/cometbft/cometbft/rpc/core/types"
	"github.com/cometbft/cometbft/types"
	dbm "github.com/cosmos/cosmos-db"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/rlp"
	"github.com/evmos/ethermint/indexer"
	"github.com/evmos/ethermint/rpc/backend/mocks"
	rpctypes "github.com/evmos/ethermint/rpc/types"
	evmtypes "github.com/evmos/ethermint/x/evm/types"
	"github.com/stretchr/testify/mock"
	"google.golang.org/grpc/metadata"
)

type sendRawTxSyncFixture struct {
	rawTx        []byte
	hash         common.Hash
	block        *types.Block
	blockResults *tmrpctypes.ResultBlockResults
}

func (suite *BackendTestSuite) setupSendRawTxSync(withReceipt bool, broadcastErr error) sendRawTxSyncFixture {
	ethTx, _ := suite.buildEthereumTx()
	rawTx, err := rlp.EncodeToBytes(ethTx.AsTransaction())
	suite.Require().NoError(err)
	cosmosTx, err := ethTx.BuildTx(suite.backend.clientCtx.TxConfig.NewTxBuilder(), "aphoton")
	suite.Require().NoError(err)
	txBytes, err := suite.backend.clientCtx.TxConfig.TxEncoder()(cosmosTx)
	suite.Require().NoError(err)

	client := suite.backend.clientCtx.Client.(*mocks.Client)
	queryClient := suite.backend.queryClient.QueryClient.(*mocks.EVMQueryClient)
	suite.backend.allowUnprotectedTxs = true
	RegisterParamsWithoutHeader(queryClient, 1)
	if broadcastErr == nil {
		RegisterBroadcastTx(client, txBytes)
	} else {
		client.On("BroadcastTxSync", context.Background(), types.Tx(txBytes)).Return(nil, broadcastErr)
	}

	blockResults := &tmrpctypes.ResultBlockResults{
		Height: 1,
		TxsResults: []*abci.ExecTxResult{{
			Code:    0,
			GasUsed: 21000,
			Events: []abci.Event{{
				Type: evmtypes.EventTypeEthereumTx,
				Attributes: []abci.EventAttribute{
					{Key: evmtypes.AttributeKeyEthereumTxHash, Value: ethTx.Hash().Hex()},
					{Key: evmtypes.AttributeKeyTxIndex, Value: "0"},
				},
			}},
		}},
	}
	block := types.MakeBlock(1, []types.Tx{txBytes}, nil, nil)
	block.ChainID = ChainID
	if withReceipt {
		var header metadata.MD
		RegisterParams(queryClient, &header, 1)
		client.On("Block", rpctypes.ContextWithHeight(1), mock.AnythingOfType("*int64")).
			Return(&tmrpctypes.ResultBlock{Block: block}, nil)
		client.On("BlockResults", rpctypes.ContextWithHeight(1), mock.AnythingOfType("*int64")).Return(blockResults, nil)
	}

	suite.backend.indexer = indexer.NewKVIndexer(dbm.NewMemDB(), tmlog.NewNopLogger(), suite.backend.clientCtx)

	return sendRawTxSyncFixture{
		rawTx:        rawTx,
		hash:         ethTx.Hash(),
		block:        block,
		blockResults: blockResults,
	}
}

func (suite *BackendTestSuite) TestSendRawTransactionSync_ReceiptAlreadyAvailable() {
	f := suite.setupSendRawTxSync(true, nil)
	suite.Require().NoError(suite.backend.indexer.IndexBlock(f.block, f.blockResults.TxsResults))

	receipt, err := suite.backend.SendRawTransactionSync(context.Background(), f.rawTx, nil)
	suite.Require().NoError(err)
	suite.Require().Equal(f.hash, receipt["transactionHash"])
}

func (suite *BackendTestSuite) TestSendRawTransactionSync_WaitsForInclusion() {
	f := suite.setupSendRawTxSync(true, nil)

	done := make(chan error, 1)
	go func() {
		time.Sleep(2 * _receiptPollInterval)
		done <- suite.backend.indexer.IndexBlock(f.block, f.blockResults.TxsResults)
	}()

	timeout := uint64(10_000)
	receipt, err := suite.backend.SendRawTransactionSync(context.Background(), f.rawTx, &timeout)
	suite.Require().NoError(<-done)
	suite.Require().NoError(err)
	suite.Require().Equal(f.hash, receipt["transactionHash"])
}

func (suite *BackendTestSuite) TestSendRawTransactionSync_ResendOfIncludedTx() {
	f := suite.setupSendRawTxSync(true, cmtmempool.ErrTxInCache)
	suite.Require().NoError(suite.backend.indexer.IndexBlock(f.block, f.blockResults.TxsResults))

	receipt, err := suite.backend.SendRawTransactionSync(context.Background(), f.rawTx, nil)
	suite.Require().NoError(err)
	suite.Require().Equal(f.hash, receipt["transactionHash"])
}

func (suite *BackendTestSuite) TestSendRawTransactionSync_ResendAfterCacheEviction() {
	f := suite.setupSendRawTxSync(true, errors.New("invalid nonce"))
	suite.Require().NoError(suite.backend.indexer.IndexBlock(f.block, f.blockResults.TxsResults))

	receipt, err := suite.backend.SendRawTransactionSync(context.Background(), f.rawTx, nil)
	suite.Require().NoError(err, "an included tx rejected on its nonce must still return its receipt")
	suite.Require().Equal(f.hash, receipt["transactionHash"])
}

func (suite *BackendTestSuite) TestSendRawTransactionSync_BroadcastErrorWithoutReceipt() {
	f := suite.setupSendRawTxSync(false, errors.New("invalid nonce"))

	receipt, err := suite.backend.SendRawTransactionSync(context.Background(), f.rawTx, nil)
	suite.Require().ErrorContains(err, "invalid nonce")
	suite.Require().Nil(receipt)
}

func (suite *BackendTestSuite) TestSendRawTransactionSync_ResendOfPendingTxWaits() {
	f := suite.setupSendRawTxSync(false, cmtmempool.ErrTxInCache)

	timeout := uint64(1)
	_, err := suite.backend.SendRawTransactionSync(context.Background(), f.rawTx, &timeout)

	var syncErr *rpctypes.ReceiptWaitTimeoutError
	suite.Require().True(errors.As(err, &syncErr), "a resent pending tx must be waited on, not rejected")
	suite.Require().Equal(f.hash, syncErr.Hash)
}

func (suite *BackendTestSuite) TestSendRawTransactionSync_Timeout() {
	f := suite.setupSendRawTxSync(false, nil)

	timeout := uint64(1)
	receipt, err := suite.backend.SendRawTransactionSync(context.Background(), f.rawTx, &timeout)
	suite.Require().Nil(receipt)

	var syncErr *rpctypes.ReceiptWaitTimeoutError
	suite.Require().True(errors.As(err, &syncErr))
	suite.Require().Equal(rpctypes.ErrCodeReceiptWaitTimeout, syncErr.ErrorCode())
	suite.Require().Equal(f.hash.Hex(), syncErr.ErrorData())
	suite.Require().Equal(time.Millisecond, syncErr.Timeout)
}

func (suite *BackendTestSuite) TestSendRawTransactionSync_MaxReceiptWaiters() {
	ethTx, _ := suite.buildEthereumTx()
	rawTx, err := rlp.EncodeToBytes(ethTx.AsTransaction())
	suite.Require().NoError(err)
	suite.backend.receiptWaiters = make(chan struct{}, 1)
	suite.backend.receiptWaiters <- struct{}{}

	// No broadcast mock is registered, so submitting the tx would fail the test.
	_, err = suite.backend.SendRawTransactionSync(context.Background(), rawTx, nil)
	var limitErr *rpctypes.ReceiptWaitersLimitError
	suite.Require().True(errors.As(err, &limitErr))
	suite.Require().Equal(rpctypes.ErrCodeLimitExceeded, limitErr.ErrorCode())
	suite.Require().Equal(1, limitErr.Limit)
}

func (suite *BackendTestSuite) TestSendRawTransactionSync_NoTimeBudget() {
	ethTx, _ := suite.buildEthereumTx()
	rawTx, err := rlp.EncodeToBytes(ethTx.AsTransaction())
	suite.Require().NoError(err)
	ctx := context.WithValue(context.Background(), http.ServerContextKey, &http.Server{WriteTimeout: 200 * time.Millisecond})

	// No broadcast mock is registered, so submitting the tx would fail the test.
	_, err = suite.backend.SendRawTransactionSync(ctx, rawTx, nil)
	suite.Require().ErrorContains(err, "the transaction was not submitted")
}

func (suite *BackendTestSuite) TestSendRawTransactionSync_ContextCanceled() {
	ethTx, _ := suite.buildEthereumTx()
	rawTx, err := rlp.EncodeToBytes(ethTx.AsTransaction())
	suite.Require().NoError(err)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	// No broadcast mock is registered, so submitting the tx would fail the test.
	_, err = suite.backend.SendRawTransactionSync(ctx, rawTx, nil)
	suite.Require().ErrorIs(err, context.Canceled)
}

func (suite *BackendTestSuite) TestReceiptWaitTimeout() {
	ms := func(v uint64) *uint64 { return &v }
	httpCtx := func(writeTimeout time.Duration) context.Context {
		return context.WithValue(context.Background(), http.ServerContextKey, &http.Server{WriteTimeout: writeTimeout})
	}

	testCases := []struct {
		name       string
		ctx        context.Context
		defaultCfg time.Duration
		maxCfg     time.Duration
		timeoutMs  *uint64
		exp        time.Duration
	}{
		{name: "nil uses default", defaultCfg: 3 * time.Second, maxCfg: time.Minute, exp: 3 * time.Second},
		{name: "zero uses default", defaultCfg: 3 * time.Second, maxCfg: time.Minute, timeoutMs: ms(0), exp: 3 * time.Second},
		{name: "client value below max", defaultCfg: time.Second, maxCfg: time.Minute, timeoutMs: ms(1500), exp: 1500 * time.Millisecond},
		{name: "client value capped by max", defaultCfg: time.Second, maxCfg: 5 * time.Second, timeoutMs: ms(60_000), exp: 5 * time.Second},
		{name: "overflowing client value capped by max", defaultCfg: time.Second, maxCfg: 5 * time.Second, timeoutMs: ms(math.MaxUint64), exp: 5 * time.Second},
		{name: "default capped by max", defaultCfg: time.Minute, maxCfg: 5 * time.Second, exp: 5 * time.Second},
		{
			name:      "capped below http write timeout",
			ctx:       httpCtx(2 * time.Second),
			maxCfg:    time.Minute,
			timeoutMs: ms(60_000),
			exp:       2*time.Second - 100*time.Millisecond - _receiptWaitResponseMargin,
		},
		{name: "never negative", ctx: httpCtx(200 * time.Millisecond), defaultCfg: time.Second, maxCfg: time.Minute, exp: 0},
	}

	for _, tc := range testCases {
		suite.Run(tc.name, func() {
			suite.backend.cfg.JSONRPC.ReceiptWaitTimeout = tc.defaultCfg
			suite.backend.cfg.JSONRPC.ReceiptWaitMaxTimeout = tc.maxCfg
			ctx := tc.ctx
			if ctx == nil {
				ctx = context.Background()
			}
			suite.Require().Equal(tc.exp, suite.backend.receiptWaitTimeout(ctx, tc.timeoutMs))
		})
	}
}

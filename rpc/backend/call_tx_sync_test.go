package backend

import (
	"context"
	"errors"
	"math"
	"math/big"
	"net/http"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	tmlog "cosmossdk.io/log/v2"
	abci "github.com/cometbft/cometbft/abci/types"
	cmtmempool "github.com/cometbft/cometbft/mempool"
	tmrpctypes "github.com/cometbft/cometbft/rpc/core/types"
	"github.com/cometbft/cometbft/types"
	dbm "github.com/cosmos/cosmos-db"
	"github.com/ethereum/go-ethereum/common"
	ethtypes "github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/rlp"
	"github.com/evmos/ethermint/indexer"
	"github.com/evmos/ethermint/rpc/backend/mocks"
	"github.com/evmos/ethermint/rpc/stream"
	rpctypes "github.com/evmos/ethermint/rpc/types"
	evmtypes "github.com/evmos/ethermint/x/evm/types"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
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

func (suite *BackendTestSuite) TestSendRawTransactionSync_ValidationErrorSkipsReceiptLookup() {
	f := suite.setupSendRawTxSync(true, nil)
	suite.Require().NoError(suite.backend.indexer.IndexBlock(f.block, f.blockResults.TxsResults))
	suite.backend.chainID = big.NewInt(9999)
	// Any broadcast or receipt lookup would now panic on the mocks.
	suite.backend.clientCtx.Client.(*mocks.Client).ExpectedCalls = nil
	suite.backend.queryClient.QueryClient.(*mocks.EVMQueryClient).ExpectedCalls = nil

	receipt, err := suite.backend.SendRawTransactionSync(context.Background(), f.rawTx, nil)
	suite.Require().ErrorContains(err, "incorrect chain-id")
	suite.Require().Nil(receipt)
}

func (suite *BackendTestSuite) TestSendRawTransactionSync_ResendOfPendingTxWaits() {
	f := suite.setupSendRawTxSync(false, cmtmempool.ErrTxInCache)

	timeout := uint64(1)
	_, err := suite.backend.SendRawTransactionSync(context.Background(), f.rawTx, &timeout)

	var syncErr *rpctypes.ReceiptWaitTimeoutError
	suite.Require().True(errors.As(err, &syncErr), "a resent pending tx must be waited on, not rejected")
	suite.Require().Equal(f.hash, syncErr.Hash)
	suite.Require().True(syncErr.AlreadyKnown)
	suite.Require().Contains(syncErr.Error(), "may not be in the transaction pool")
}

func (suite *BackendTestSuite) TestSendRawTransactionSync_ResendsOfOneTxAreCapped() {
	f := suite.setupSendRawTxSync(true, cmtmempool.ErrTxInCache)
	suite.backend.receiptWaiters = make(chan struct{}, 4*_maxWaitsPerTx)

	timeout := uint64(10_000)
	errs := make(chan error, _maxWaitsPerTx)
	for range _maxWaitsPerTx {
		go func() {
			_, err := suite.backend.SendRawTransactionSync(context.Background(), f.rawTx, &timeout)
			errs <- err
		}()
	}
	suite.Require().Eventually(func() bool {
		ws := suite.backend.receiptWatches
		ws.mu.Lock()
		defer ws.mu.Unlock()
		w := ws.byHash[f.hash]
		return w != nil && w.refs == _maxWaitsPerTx
	}, 5*time.Second, 10*time.Millisecond)
	suite.Require().Len(suite.backend.receiptWaiters, _maxWaitsPerTx, "each waiting request holds a slot")

	_, err := suite.backend.SendRawTransactionSync(context.Background(), f.rawTx, &timeout)
	var limitErr *rpctypes.ReceiptWaitersLimitError
	suite.Require().True(errors.As(err, &limitErr))
	suite.Require().True(limitErr.PerTx)
	suite.Require().Equal(_maxWaitsPerTx, limitErr.Limit)

	suite.Require().NoError(suite.backend.indexer.IndexBlock(f.block, f.blockResults.TxsResults))
	for range _maxWaitsPerTx {
		suite.Require().NoError(<-errs)
	}
	suite.Require().Eventually(func() bool { return len(suite.backend.receiptWaiters) == 0 }, time.Second, 10*time.Millisecond)
}

func (suite *BackendTestSuite) TestSendRawTransactionSync_TimeoutStartsAfterBroadcast() {
	f := suite.setupSendRawTxSync(false, nil)
	const broadcastDelay = 300 * time.Millisecond
	client := suite.backend.clientCtx.Client.(*mocks.Client)
	for _, c := range client.ExpectedCalls {
		if c.Method == "BroadcastTxSync" {
			c.After(broadcastDelay)
		}
	}

	timeout := uint64(200)
	start := time.Now()
	_, err := suite.backend.SendRawTransactionSync(context.Background(), f.rawTx, &timeout)
	var syncErr *rpctypes.ReceiptWaitTimeoutError
	suite.Require().True(errors.As(err, &syncErr))
	suite.Require().Equal(200*time.Millisecond, syncErr.Timeout)
	suite.Require().GreaterOrEqual(time.Since(start), broadcastDelay+200*time.Millisecond)
}

func (suite *BackendTestSuite) TestSendRawTransactionSync_TimeoutCappedByRequestBudget() {
	f := suite.setupSendRawTxSync(false, nil)
	writeTimeout := MinSyncHTTPTimeout + 200*time.Millisecond
	ctx := context.WithValue(context.Background(), http.ServerContextKey, &http.Server{WriteTimeout: writeTimeout})
	budget, bounded := receiptRequestBudget(ctx)
	suite.Require().True(bounded)

	timeout := uint64(10_000)
	_, err := suite.backend.SendRawTransactionSync(ctx, f.rawTx, &timeout)
	var syncErr *rpctypes.ReceiptWaitTimeoutError
	suite.Require().True(errors.As(err, &syncErr))
	suite.Require().Positive(syncErr.Timeout)
	suite.Require().Less(syncErr.Timeout, budget, "the wait must end before the request timeout")
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
	suite.Require().False(syncErr.AlreadyKnown)
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

	testCases := []struct {
		name       string
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
	}

	for _, tc := range testCases {
		suite.Run(tc.name, func() {
			suite.backend.cfg.JSONRPC.ReceiptWaitTimeout = tc.defaultCfg
			suite.backend.cfg.JSONRPC.ReceiptWaitMaxTimeout = tc.maxCfg
			suite.Require().Equal(tc.exp, suite.backend.receiptWaitTimeout(tc.timeoutMs))
		})
	}
}

func TestReceiptRequestBudget(t *testing.T) {
	httpCtx := func(writeTimeout time.Duration) context.Context {
		return context.WithValue(context.Background(), http.ServerContextKey, &http.Server{WriteTimeout: writeTimeout})
	}

	_, bounded := receiptRequestBudget(context.Background())
	require.False(t, bounded)

	budget, bounded := receiptRequestBudget(httpCtx(2 * time.Second))
	require.True(t, bounded)
	require.Equal(t, 2*time.Second-100*time.Millisecond-_receiptWaitResponseMargin, budget)

	budget, _ = receiptRequestBudget(httpCtx(MinSyncHTTPTimeout))
	require.Equal(t, _minRequestBudget, budget, "MinSyncHTTPTimeout is the exact floor")

	budget, _ = receiptRequestBudget(httpCtx(MinSyncHTTPTimeout - time.Millisecond))
	require.Less(t, budget, _minRequestBudget)
}

func (suite *BackendTestSuite) withHeaderStream() *stream.Stream[stream.RPCHeader] {
	headers := stream.NewStream[stream.RPCHeader](16, 64)
	suite.backend.headerStream = func() (*stream.Stream[stream.RPCHeader], error) { return headers, nil }
	return headers
}

func rpcHeaderAt(height int64) stream.RPCHeader {
	return stream.RPCHeader{EthHeader: &ethtypes.Header{Number: big.NewInt(height)}}
}

func (suite *BackendTestSuite) TestSendRawTransactionSync_WakesOnNewBlockDespiteIndexLag() {
	f := suite.setupSendRawTxSync(true, nil)
	headers := suite.withHeaderStream()

	done := make(chan error, 1)
	go func() {
		time.Sleep(50 * time.Millisecond)
		headers.Add(rpcHeaderAt(1))
		// The indexer finishes after the header event; a lag retry must pick it up.
		time.Sleep(_indexLagRetryDelay + 50*time.Millisecond)
		done <- suite.backend.indexer.IndexBlock(f.block, f.blockResults.TxsResults)
	}()

	timeout := uint64(10_000)
	start := time.Now()
	receipt, err := suite.backend.SendRawTransactionSync(context.Background(), f.rawTx, &timeout)
	suite.Require().NoError(<-done)
	suite.Require().NoError(err)
	suite.Require().Equal(f.hash, receipt["transactionHash"])
	suite.Require().Less(time.Since(start), time.Second)
}

type progressIndexer struct {
	failingLookupIndexer
	last atomic.Int64
}

func (p *progressIndexer) LastIndexedBlock() (int64, error) { return p.last.Load(), nil }

func (suite *BackendTestSuite) TestReceiptWaiter_BlocksUntilNewBlock() {
	idx := &progressIndexer{}
	idx.last.Store(-1)
	suite.backend.indexer = idx
	headers := suite.withHeaderStream()
	w := suite.backend.newReceiptWaiter()

	ctx, cancel := context.WithTimeout(context.Background(), 3*_receiptPollInterval)
	defer cancel()
	start := time.Now()
	suite.Require().False(w.wait(ctx), "must not wake without a new block")
	suite.Require().GreaterOrEqual(time.Since(start), 3*_receiptPollInterval)

	headers.Add(rpcHeaderAt(1))
	suite.Require().True(w.wait(context.Background()))
	suite.Require().Equal(int64(1), w.height)

	// The indexer lags behind height 1, so it retries past the CometBFT cap, up to its own.
	for range _evmIndexLagRetries {
		retryCtx, retryCancel := context.WithTimeout(context.Background(), time.Second)
		start := time.Now()
		suite.Require().True(w.wait(retryCtx))
		suite.Require().Less(time.Since(start), 2*_indexLagRetryDelay)
		retryCancel()
	}
	stuckCtx, stuckCancel := context.WithTimeout(context.Background(), 3*_indexLagRetryDelay)
	defer stuckCancel()
	suite.Require().False(w.wait(stuckCtx), "stuck indexer: retries capped until the next block")

	headers.Add(rpcHeaderAt(2))
	suite.Require().True(w.wait(context.Background()))
	// The lookup after header 2 missed; the indexer catches up only afterwards, so it must retry once.
	idx.last.Store(2)
	retryCtx, retryCancel := context.WithTimeout(context.Background(), time.Second)
	defer retryCancel()
	start = time.Now()
	suite.Require().True(w.wait(retryCtx))
	suite.Require().Less(time.Since(start), 2*_indexLagRetryDelay)

	idleCtx, idleCancel := context.WithTimeout(context.Background(), 3*_indexLagRetryDelay)
	defer idleCancel()
	suite.Require().False(w.wait(idleCtx), "indexer caught up: waits for the next block")
}

func (suite *BackendTestSuite) TestReceiptWaiter_NoIndexLagRetryForBlockWithoutEVMTxs() {
	idx := &progressIndexer{}
	idx.last.Store(-1)
	suite.backend.indexer = idx
	headers := suite.withHeaderStream()
	w := suite.backend.newReceiptWaiter()

	empty := rpcHeaderAt(1)
	empty.EthHeader.TxHash = ethtypes.EmptyRootHash
	headers.Add(empty)
	idleCtx, idleCancel := context.WithTimeout(context.Background(), 3*_indexLagRetryDelay)
	defer idleCancel()
	suite.Require().False(w.wait(idleCtx), "a block without EVM txs cannot hold the tx, so it does not wake")

	headers.Add(rpcHeaderAt(2))
	suite.Require().True(w.wait(context.Background()))
	suite.Require().Equal(int64(2), w.height)
}

func TestReceiptPoller_ConcurrentPollsShareOneLookup(t *testing.T) {
	var calls atomic.Int32
	unblock := make(chan struct{})
	p := &receiptPoller{
		hash: common.Hash{1},
		lookup: func(common.Hash) (map[string]interface{}, error) {
			calls.Add(1)
			<-unblock
			return map[string]interface{}{"status": "0x1"}, nil
		},
	}

	const pollers = 3
	results := make(chan map[string]interface{}, pollers)
	var wg sync.WaitGroup
	for range pollers {
		wg.Go(func() {
			ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
			defer cancel()
			results <- p.poll(ctx, time.Time{})
		})
	}
	wg.Wait()
	close(results)
	for receipt := range results {
		require.Nil(t, receipt)
	}
	require.EqualValues(t, 1, calls.Load(), "concurrent polls join the running lookup")

	close(unblock)
	require.NotNil(t, p.poll(context.Background(), time.Time{}))
	require.EqualValues(t, 1, calls.Load())
}

func TestReceiptPoller_MissFromLookupStartedBeforeNotBeforeIsRetried(t *testing.T) {
	var calls atomic.Int32
	unblock := make(chan struct{})
	p := &receiptPoller{
		hash: common.Hash{1},
		lookup: func(common.Hash) (map[string]interface{}, error) {
			if calls.Add(1) == 1 {
				<-unblock
				return nil, nil
			}
			return map[string]interface{}{"status": "0x1"}, nil
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	require.Nil(t, p.poll(ctx, time.Time{}))
	cancel()

	got := make(chan map[string]interface{}, 1)
	go func() { got <- p.poll(context.Background(), time.Now()) }()
	close(unblock)
	require.NotNil(t, <-got, "a miss from a lookup older than the wake-up is looked up again")
	require.EqualValues(t, 2, calls.Load())
}

func TestReceiptPoller_LookupPanicIsAMissNotInclusion(t *testing.T) {
	p := &receiptPoller{
		logger: tmlog.NewNopLogger(),
		lookup: func(common.Hash) (map[string]interface{}, error) {
			panic("nil block result")
		},
	}
	require.Nil(t, p.poll(context.Background(), time.Time{}))
	require.NoError(t, p.includedErr(), "a panic does not show the tx was included")
}

func TestReceiptPoller_TransientBuildErrorIsRetried(t *testing.T) {
	var calls atomic.Int32
	p := &receiptPoller{
		hash: common.Hash{1},
		lookup: func(common.Hash) (map[string]interface{}, error) {
			if calls.Add(1) == 1 {
				return nil, errors.New("grpc unavailable")
			}
			return map[string]interface{}{"status": "0x1"}, nil
		},
	}
	require.Nil(t, p.poll(context.Background(), time.Time{}))
	require.ErrorContains(t, p.includedErr(), "was included but its receipt could not be built")
	require.ErrorContains(t, p.includedErr(), common.Hash{1}.Hex())

	require.NotNil(t, p.poll(context.Background(), time.Time{}))
	require.NoError(t, p.includedErr(), "only the latest lookup's error is kept")
}

func newReceiptWatchBackend(slots int, maxWait time.Duration) *Backend {
	b := &Backend{
		logger:         tmlog.NewNopLogger(),
		receiptWatches: &receiptWatches{byHash: make(map[common.Hash]*receiptWatch)},
	}
	if slots > 0 {
		b.receiptWaiters = make(chan struct{}, slots)
	}
	b.cfg.JSONRPC.ReceiptWaitMaxTimeout = maxWait
	return b
}

func watchCount(b *Backend) int {
	b.receiptWatches.mu.Lock()
	defer b.receiptWatches.mu.Unlock()
	return len(b.receiptWatches.byHash)
}

func TestReceiptWatch_SlowLookupHoldsTheSlotAndResendTakesItOver(t *testing.T) {
	b := newReceiptWatchBackend(4, time.Minute)
	hash := common.Hash{1}
	var calls atomic.Int32
	unblock := make(chan struct{})
	lookup := func(common.Hash) (map[string]interface{}, error) {
		calls.Add(1)
		<-unblock
		return map[string]interface{}{"status": "0x1"}, nil
	}
	pollBriefly := func(p *receiptPoller) map[string]interface{} {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
		defer cancel()
		return p.poll(ctx, time.Time{})
	}

	p, release, err := b.acquireReceiptWatch(hash)
	require.NoError(t, err)
	p.lookup = lookup
	require.Nil(t, pollBriefly(p), "returns at the deadline while the lookup runs")
	require.Nil(t, pollBriefly(p))
	require.EqualValues(t, 1, calls.Load(), "the running lookup is reused, not restarted")
	release()
	time.Sleep(20 * time.Millisecond)
	require.Len(t, b.receiptWaiters, 1, "slot held while the lookup still runs")

	p2, release2, err := b.acquireReceiptWatch(hash)
	require.NoError(t, err)
	require.Same(t, p, p2, "a resend joins the watch whose lookup still runs")
	require.Len(t, b.receiptWaiters, 1, "the resend takes over the held slot")
	require.Nil(t, pollBriefly(p2))
	require.EqualValues(t, 1, calls.Load())

	release2()
	close(unblock)
	require.Eventually(t, func() bool { return len(b.receiptWaiters) == 0 && watchCount(b) == 0 },
		time.Second, time.Millisecond, "slot and watch freed once the lookup ends")
}

func TestReceiptWatch_NoLookupAfterDeadlineFreesTheSlotAtOnce(t *testing.T) {
	b := newReceiptWatchBackend(4, time.Minute)
	p, release, err := b.acquireReceiptWatch(common.Hash{1})
	require.NoError(t, err)
	p.lookup = func(common.Hash) (map[string]interface{}, error) {
		t.Error("lookup started after the deadline")
		return nil, nil
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	require.Nil(t, p.poll(ctx, time.Time{}))

	release()
	require.Empty(t, b.receiptWaiters)
	require.Zero(t, watchCount(b))
}

func TestReceiptWatch_HungLookupFreesTheSlotAfterMaxHold(t *testing.T) {
	hang := make(chan struct{})
	defer close(hang)
	b := newReceiptWatchBackend(4, 25*time.Millisecond)
	p, release, err := b.acquireReceiptWatch(common.Hash{1})
	require.NoError(t, err)
	p.lookup = func(common.Hash) (map[string]interface{}, error) {
		<-hang
		return nil, nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	require.Nil(t, p.poll(ctx, time.Time{}))

	release()
	require.Eventually(t, func() bool { return len(b.receiptWaiters) == 0 && watchCount(b) == 0 },
		time.Second, time.Millisecond, "slot still held by a hung lookup after maxHold")
}

func TestReceiptWatch_ResendTakeoverDoesNotExtendHold(t *testing.T) {
	hang := make(chan struct{})
	defer close(hang)
	b := newReceiptWatchBackend(4, 25*time.Millisecond)
	hash := common.Hash{1}
	p, release, err := b.acquireReceiptWatch(hash)
	require.NoError(t, err)
	p.lookup = func(common.Hash) (map[string]interface{}, error) {
		<-hang
		return nil, nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	require.Nil(t, p.poll(ctx, time.Time{}))
	release()

	p2, release2, err := b.acquireReceiptWatch(hash)
	require.NoError(t, err)
	require.Same(t, p, p2)
	time.Sleep(60 * time.Millisecond)
	release2()
	require.Empty(t, b.receiptWaiters, "the hold counts from the lookup's start")
	require.Zero(t, watchCount(b))
}

func TestReceiptPoller_FinishedLookupWinsOverExpiredContext(t *testing.T) {
	p := &receiptPoller{
		hash:   common.Hash{1},
		logger: tmlog.NewNopLogger(),
		lookup: func(common.Hash) (map[string]interface{}, error) {
			return map[string]interface{}{"status": "0x1"}, nil
		},
	}
	call := &receiptLookup{started: time.Now(), done: make(chan struct{})}
	p.pending = call
	p.run(call)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	for range 50 {
		p.pending = call
		require.NotNil(t, p.poll(ctx, time.Time{}))
	}
}

func TestReceiptWatch_PerTxCapOffWithoutWaiterCap(t *testing.T) {
	b := newReceiptWatchBackend(0, time.Minute)
	for range _maxWaitsPerTx + 1 {
		_, release, err := b.acquireReceiptWatch(common.Hash{1})
		require.NoError(t, err)
		defer release()
	}
}

func (suite *BackendTestSuite) TestReceiptWaiter_HeaderStreamErrorFallsBackToPolling() {
	suite.backend.headerStream = func() (*stream.Stream[stream.RPCHeader], error) {
		return nil, errors.New("subscribe failed")
	}
	w := suite.backend.newReceiptWaiter()
	suite.Require().Nil(w.headers)

	start := time.Now()
	suite.Require().True(w.wait(context.Background()))
	suite.Require().Less(time.Since(start), 2*_receiptPollInterval)
}

func (suite *BackendTestSuite) TestReceiptWaiter_CometBFTIndexerRetriesBounded() {
	suite.backend.indexer = nil
	headers := suite.withHeaderStream()
	w := suite.backend.newReceiptWaiter()

	headers.Add(rpcHeaderAt(1))
	suite.Require().True(w.wait(context.Background()))

	for range _cometIndexLagRetries {
		retryCtx, retryCancel := context.WithTimeout(context.Background(), time.Second)
		start := time.Now()
		suite.Require().True(w.wait(retryCtx))
		suite.Require().Less(time.Since(start), 2*_indexLagRetryDelay)
		retryCancel()
	}
	idleCtx, idleCancel := context.WithTimeout(context.Background(), 3*_indexLagRetryDelay)
	defer idleCancel()
	suite.Require().False(w.wait(idleCtx), "retries exhausted: waits for the next block")
}

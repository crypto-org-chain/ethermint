package stream

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"cosmossdk.io/log/v2"
	abci "github.com/cometbft/cometbft/abci/types"
	rpcclient "github.com/cometbft/cometbft/rpc/client"
	coretypes "github.com/cometbft/cometbft/rpc/core/types"
	tmtypes "github.com/cometbft/cometbft/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	ethtypes "github.com/ethereum/go-ethereum/core/types"
	"github.com/evmos/ethermint/rpc/types"
	ethermint "github.com/evmos/ethermint/types"
	evmtypes "github.com/evmos/ethermint/x/evm/types"
	"google.golang.org/grpc"
)

const (
	streamSubscriberName = "ethermint-json-rpc"
	subscribBufferSize   = 1024

	headerStreamSegmentSize = 128
	headerStreamCapacity    = 128 * 32
	txStreamSegmentSize     = 1024
	txStreamCapacity        = 1024 * 32
	logStreamSegmentSize    = 2048
	logStreamCapacity       = 2048 * 32

	_subscribeRetryBackoff = time.Second
)

var blockEvents = tmtypes.QueryForEvent(tmtypes.EventNewBlock).String()

type RPCHeader struct {
	EthHeader   *ethtypes.Header
	Hash        common.Hash
	TimestampMs *hexutil.Uint64
}

type validatorAccountFunc func(
	ctx context.Context, in *evmtypes.QueryValidatorAccountRequest, opts ...grpc.CallOption,
) (*evmtypes.QueryValidatorAccountResponse, error)

// RPCStream provides data streams for newHeads, logs, and pendingTransactions.
// it's only started on demand, so there's no overhead if the filter apis are not called at all.
type RPCStream struct {
	evtClient rpcclient.EventsClient
	logger    log.Logger
	txDecoder sdk.TxDecoder

	// initMu guards lazy init: HeaderStream is called concurrently by filters and receipt waiters.
	initMu         sync.Mutex
	subscribeErr   error
	subscribeErrAt time.Time
	// ready is set once the streams exist, so callers skip initMu after setup.
	ready atomic.Bool
	// headerStream/logStream are backed by cometbft event subscription
	headerStream *Stream[RPCHeader]
	logStream    *Stream[*types.RPCLog]

	// pendingTxStream is backed by check-tx ante handler
	pendingTxStream *Stream[common.Hash]

	wg               sync.WaitGroup
	validatorAccount validatorAccountFunc
}

func NewRPCStreams(
	evtClient rpcclient.EventsClient,
	logger log.Logger,
	txDecoder sdk.TxDecoder,
	validatorAccount validatorAccountFunc,
) *RPCStream {
	return &RPCStream{
		evtClient:        evtClient,
		logger:           logger,
		txDecoder:        txDecoder,
		validatorAccount: validatorAccount,
		pendingTxStream:  NewStream[common.Hash](txStreamSegmentSize, txStreamCapacity),
	}
}

func (s *RPCStream) initSubscriptions() error {
	if s.ready.Load() {
		return nil
	}
	s.initMu.Lock()
	defer s.initMu.Unlock()
	if s.ready.Load() {
		return nil
	}
	// Callers on every request would otherwise retry a failing Subscribe back to back under initMu.
	if s.subscribeErr != nil && time.Since(s.subscribeErrAt) < _subscribeRetryBackoff {
		return s.subscribeErr
	}

	chBlocks, err := s.evtClient.Subscribe(context.Background(), streamSubscriberName, blockEvents, subscribBufferSize)
	if err != nil {
		// Streams stay unset so a later call retries instead of returning a stream nothing feeds.
		s.subscribeErr = fmt.Errorf("subscribe to new blocks: %w", err)
		s.subscribeErrAt = time.Now()
		s.logger.Error("failed to subscribe to new blocks", "err", err)
		return s.subscribeErr
	}
	s.subscribeErr = nil

	s.headerStream = NewStream[RPCHeader](headerStreamSegmentSize, headerStreamCapacity)
	s.logStream = NewStream[*types.RPCLog](logStreamSegmentSize, logStreamCapacity)
	s.ready.Store(true)

	// Added before the goroutine starts so Close cannot miss it.
	s.wg.Add(1)
	go s.start(&s.wg, chBlocks)
	return nil
}

func (s *RPCStream) Close() error {
	// Taken so an init past Subscribe finishes first; released before Wait, which can be long.
	s.initMu.Lock()
	ready := s.ready.Load()
	s.initMu.Unlock()
	if !ready {
		return nil
	}

	if err := s.evtClient.UnsubscribeAll(context.Background(), streamSubscriberName); err != nil {
		return err
	}
	s.wg.Wait()
	return nil
}

func (s *RPCStream) HeaderStream() *Stream[RPCHeader] {
	return mustStream(s.TryHeaderStream())
}

// TryHeaderStream is HeaderStream, but returns the subscription error instead of panicking.
func (s *RPCStream) TryHeaderStream() (*Stream[RPCHeader], error) {
	if err := s.initSubscriptions(); err != nil {
		return nil, err
	}
	return s.headerStream, nil
}

func (s *RPCStream) PendingTxStream() *Stream[common.Hash] {
	return s.pendingTxStream
}

func (s *RPCStream) LogStream() *Stream[*types.RPCLog] {
	return mustStream(s.TryLogStream())
}

// TryLogStream is LogStream, but returns the subscription error instead of panicking.
func (s *RPCStream) TryLogStream() (*Stream[*types.RPCLog], error) {
	if err := s.initSubscriptions(); err != nil {
		return nil, err
	}
	return s.logStream, nil
}

func mustStream[T any](st *Stream[T], err error) *Stream[T] {
	if err != nil {
		panic(err)
	}
	return st
}

// ListenPendingTx is a callback passed to application to listen for pending transactions in CheckTx.
func (s *RPCStream) ListenPendingTx(hash common.Hash) {
	s.PendingTxStream().Add(hash)
}

func (s *RPCStream) start(
	wg *sync.WaitGroup,
	chBlocks <-chan coretypes.ResultEvent,
) {
	defer func() {
		wg.Done()
		if err := s.evtClient.UnsubscribeAll(context.Background(), streamSubscriberName); err != nil {
			s.logger.Error("failed to unsubscribe", "err", err)
		}
	}()

	for ev := range chBlocks {
		data, ok := ev.Data.(tmtypes.EventDataNewBlock)
		if !ok {
			s.logger.Error("event data type mismatch", "type", fmt.Sprintf("%T", ev.Data))
			continue
		}

		// publish the header before the logs, the logs used to come from the tx events, published after it.
		if header, err := s.blockHeader(data); err != nil {
			s.logger.Error("failed to build newHeads header, dropping header", "height", data.Block.Height, "err", err)
		} else {
			s.headerStream.Add(header)
		}
		s.logStream.Add(s.blockLogs(data)...)
	}
}

func (s *RPCStream) blockHeader(data tmtypes.EventDataNewBlock) (RPCHeader, error) {
	baseFee := types.BaseFeeFromEvents(data.ResultFinalizeBlock.Events)
	res, err := s.validatorAccount(
		types.ContextWithHeight(data.Block.Height),
		&evmtypes.QueryValidatorAccountRequest{
			ConsAddress: sdk.ConsAddress(data.Block.Header.ProposerAddress).String(),
		},
	)
	if err != nil {
		return RPCHeader{}, fmt.Errorf("failed to get validator account: %w", err)
	}
	validator, err := sdk.AccAddressFromBech32(res.AccountAddress)
	if err != nil {
		return RPCHeader{}, fmt.Errorf("failed to convert validator account: %w", err)
	}
	// TODO: fetch bloom from events
	header := types.EthHeaderFromTendermint(data.Block.Header, ethtypes.Bloom{}, baseFee, validator)
	txHash, err := evmTxHashFromEventData(data, s.txDecoder)
	if err != nil {
		// Drop rather than publish a wrong transactionsRoot; a gap in
		// the stream is better than incorrect data cached by clients.
		return RPCHeader{}, fmt.Errorf("failed to compute transactionsRoot: %w", err)
	}
	header.TxHash = txHash
	return RPCHeader{
		EthHeader:   header,
		Hash:        common.BytesToHash(data.Block.Header.Hash()),
		TimestampMs: types.BlockTimestampMs(data.Block.Time),
	}, nil
}

func (s *RPCStream) blockLogs(data tmtypes.EventDataNewBlock) []*types.RPCLog {
	height, err := ethermint.SafeUint64(data.Block.Height)
	if err != nil {
		return nil
	}
	var logs []*ethtypes.Log
	for _, txResult := range data.ResultFinalizeBlock.TxResults {
		if txResult == nil || !isEvmTx(txResult.Events) {
			// ignore transaction as it's not from the evm module
			continue
		}
		txLogs, err := evmtypes.DecodeTxLogsFromEvents(txResult.Data, txResult.Events, height)
		if err != nil {
			s.logger.Error("fail to decode evm tx response", "error", err.Error())
			continue
		}
		logs = append(logs, txLogs...)
	}
	return types.NewRPCLogs(logs, data.Block.Time)
}

func isEvmTx(events []abci.Event) bool {
	for _, event := range events {
		if event.Type != evmtypes.TypeMsgEthereumTx {
			continue
		}
		for _, attr := range event.Attributes {
			if attr.Key == evmtypes.AttributeKeyEthereumTxHash {
				return true
			}
		}
	}
	return false
}

func evmTxHashFromEventData(data tmtypes.EventDataNewBlock, txDecoder sdk.TxDecoder) (common.Hash, error) {
	msgs, err := types.EvmMsgsFromTxs(txDecoder, data.Block.Txs, data.ResultFinalizeBlock.TxResults)
	if err != nil {
		return common.Hash{}, err
	}
	return types.EvmTxHashFromMsgs(msgs), nil
}

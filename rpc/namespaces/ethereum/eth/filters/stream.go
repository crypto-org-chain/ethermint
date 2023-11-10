package filters

import (
	"context"
	"fmt"
	"sync"

	"github.com/cometbft/cometbft/libs/log"
	rpcclient "github.com/cometbft/cometbft/rpc/client"
	coretypes "github.com/cometbft/cometbft/rpc/core/types"
	tmtypes "github.com/cometbft/cometbft/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/ethereum/go-ethereum/common"
	ethtypes "github.com/ethereum/go-ethereum/core/types"
	"github.com/evmos/ethermint/rpc/ethereum/pubsub"
	"github.com/evmos/ethermint/rpc/types"
	evmtypes "github.com/evmos/ethermint/x/evm/types"
)

const (
	streamSubscriberName = "ethermint-json-rpc"
	subscriberBufferSize = 1024

	headerStreamSegmentSize = 128
	headerStreamCapacity    = 128 * 32
	txStreamSegmentSize     = 1024
	txStreamCapacity        = 1024 * 32
	logStreamSegmentSize    = 2048
	logStreamCapacity       = 2048 * 32
)

type RPCHeader struct {
	EthHeader *ethtypes.Header
	Hash      common.Hash
}

// RPCStream provides data streams for newHeads, logs, and pendingTransactions.
type RPCStream struct {
	evtClient rpcclient.EventsClient
	logger    log.Logger
	txDecoder sdk.TxDecoder

	HeaderStream *pubsub.Stream[RPCHeader]
	TxStream     *pubsub.Stream[common.Hash]
	LogStream    *pubsub.Stream[*ethtypes.Log]

	wg sync.WaitGroup
}

func NewRPCStreams(evtClient rpcclient.EventsClient, logger log.Logger, txDecoder sdk.TxDecoder) (*RPCStream, error) {
	s := &RPCStream{
		evtClient: evtClient,
		logger:    logger,
		txDecoder: txDecoder,

		HeaderStream: pubsub.NewStream[RPCHeader](headerStreamSegmentSize, headerStreamCapacity),
		TxStream:     pubsub.NewStream[common.Hash](txStreamSegmentSize, txStreamCapacity),
		LogStream:    pubsub.NewStream[*ethtypes.Log](logStreamSegmentSize, logStreamCapacity),
	}

	ctx := context.Background()

	chHeaders, err := s.evtClient.Subscribe(ctx, subscriberName, headerEvents, subscriberBufferSize)
	if err != nil {
		return nil, err
	}

	chTx, err := s.evtClient.Subscribe(ctx, subscriberName, txEvents, subscriberBufferSize)
	if err != nil {
		if err := s.evtClient.UnsubscribeAll(ctx, subscriberName); err != nil {
			s.logger.Error("failed to unsubscribe", "err", err)
		}
		return nil, err
	}

	chLogs, err := s.evtClient.Subscribe(ctx, subscriberName, evmEvents, subscriberBufferSize)
	if err != nil {
		if err := s.evtClient.UnsubscribeAll(context.Background(), subscriberName); err != nil {
			s.logger.Error("failed to unsubscribe", "err", err)
		}
		return nil, err
	}

	go s.start(&s.wg, chHeaders, chTx, chLogs)

	return s, nil
}

func (s *RPCStream) Close() error {
	if err := s.evtClient.UnsubscribeAll(context.Background(), subscriberName); err != nil {
		return err
	}
	s.wg.Wait()
	return nil
}

func (s *RPCStream) start(
	wg *sync.WaitGroup,
	chHeaders <-chan coretypes.ResultEvent,
	chTx <-chan coretypes.ResultEvent,
	chLogs <-chan coretypes.ResultEvent,
) {
	wg.Add(1)
	defer func() {
		wg.Done()
		if err := s.evtClient.UnsubscribeAll(context.Background(), subscriberName); err != nil {
			s.logger.Error("failed to unsubscribe", "err", err)
		}
	}()

	for {
		select {
		case ev, ok := <-chHeaders:
			if !ok {
				chHeaders = nil
				break
			}

			data, ok := ev.Data.(tmtypes.EventDataNewBlockHeader)
			if !ok {
				s.logger.Error("event data type mismatch", "type", fmt.Sprintf("%T", ev.Data))
				continue
			}

			baseFee := types.BaseFeeFromEvents(data.ResultBeginBlock.Events)

			// TODO: fetch bloom from events
			header := types.EthHeaderFromTendermint(data.Header, ethtypes.Bloom{}, baseFee)
			s.HeaderStream.Add(RPCHeader{EthHeader: header, Hash: common.BytesToHash(data.Header.Hash())})
		case ev, ok := <-chTx:
			if !ok {
				chTx = nil
				break
			}

			data, ok := ev.Data.(tmtypes.EventDataTx)
			if !ok {
				s.logger.Error("event data type mismatch", "type", fmt.Sprintf("%T", ev.Data))
				continue
			}

			tx, err := s.txDecoder(data.Tx)
			if err != nil {
				s.logger.Error("fail to decode tx", "error", err.Error())
				continue
			}

			for _, msg := range tx.GetMsgs() {
				if ethTx, ok := msg.(*evmtypes.MsgEthereumTx); ok {
					s.TxStream.Add(ethTx.AsTransaction().Hash())
				}
			}
		case ev, ok := <-chLogs:
			if !ok {
				chLogs = nil
				break
			}

			if _, ok := ev.Events[evmtypes.TypeMsgEthereumTx]; !ok {
				// ignore transaction as it's not from the evm module
				continue
			}

			// get transaction result data
			dataTx, ok := ev.Data.(tmtypes.EventDataTx)
			if !ok {
				s.logger.Error("event data type mismatch", "type", fmt.Sprintf("%T", ev.Data))
				continue
			}
			txLogs, err := evmtypes.DecodeTxLogsFromEvents(dataTx.TxResult.Result.Data, uint64(dataTx.TxResult.Height))
			if err != nil {
				s.logger.Error("fail to decode tx response", "error", err.Error())
				continue
			}

			s.LogStream.Add(txLogs...)
		}

		if chHeaders == nil && chTx == nil && chLogs == nil {
			break
		}
	}
}

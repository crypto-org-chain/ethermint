package stream

import (
	"context"
	"errors"
	"fmt"
	"math/big"
	"testing"
	"time"

	"cosmossdk.io/log/v2"
	abci "github.com/cometbft/cometbft/abci/types"
	coretypes "github.com/cometbft/cometbft/rpc/core/types"
	tmtypes "github.com/cometbft/cometbft/types"
	codectypes "github.com/cosmos/cosmos-sdk/codec/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	gogoproto "github.com/cosmos/gogoproto/proto"
	"github.com/ethereum/go-ethereum/common"
	ethtypes "github.com/ethereum/go-ethereum/core/types"
	rpctypes "github.com/evmos/ethermint/rpc/types"
	evmtypes "github.com/evmos/ethermint/x/evm/types"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	proto "google.golang.org/protobuf/proto"
)

// evmTxResult builds a tx result with a log of addr, evm sets the ethereum tx hash event.
func evmTxResult(t *testing.T, addr common.Address, evm bool) *abci.ExecTxResult {
	t.Helper()
	anyVal, err := codectypes.NewAnyWithValue(&evmtypes.MsgEthereumTxResponse{
		Logs: []*evmtypes.Log{{Address: addr.Hex()}},
	})
	require.NoError(t, err)
	data, err := gogoproto.Marshal(&sdk.TxMsgData{MsgResponses: []*codectypes.Any{anyVal}})
	require.NoError(t, err)
	res := &abci.ExecTxResult{Data: data}
	if evm {
		res.Events = []abci.Event{{
			Type:       evmtypes.TypeMsgEthereumTx,
			Attributes: []abci.EventAttribute{{Key: evmtypes.AttributeKeyEthereumTxHash, Value: "0x01"}},
		}}
	}
	return res
}

func TestStartPublishesBlockLogs(t *testing.T) {
	const height = int64(10)
	blockTime := time.Unix(1700000000, 0)
	evmAddr := common.HexToAddress("0x01")
	data := tmtypes.EventDataNewBlock{
		Block: &tmtypes.Block{Header: tmtypes.Header{Height: height, Time: blockTime}},
		ResultFinalizeBlock: abci.ResponseFinalizeBlock{
			TxResults: []*abci.ExecTxResult{
				evmTxResult(t, evmAddr, true),
				evmTxResult(t, common.HexToAddress("0x02"), false),
			},
		},
	}

	s := &RPCStream{
		evtClient:    stubEventsClient{},
		logger:       log.NewNopLogger(),
		headerStream: NewStream[RPCHeader](headerStreamSegmentSize, headerStreamCapacity),
		logStream:    NewStream[*ethtypes.Log](logStreamSegmentSize, logStreamCapacity),
		// fail the header, the logs must still be published
		validatorAccount: func(
			context.Context, *evmtypes.QueryValidatorAccountRequest, ...grpc.CallOption,
		) (*evmtypes.QueryValidatorAccountResponse, error) {
			return nil, errors.New("validator not found")
		},
	}
	ch := make(chan coretypes.ResultEvent, 1)
	ch <- coretypes.ResultEvent{Data: data}
	close(ch)
	s.start(&s.wg, ch)

	logs, _ := s.logStream.ReadAllNonBlocking(0)
	require.Len(t, logs, 1, "only the logs of evm txs are published")
	require.Equal(t, evmAddr, logs[0].Address)
	require.Equal(t, uint64(height), logs[0].BlockNumber)
	require.Equal(t, uint64(blockTime.Unix()), logs[0].BlockTimestamp)
	headers, _ := s.headerStream.ReadAllNonBlocking(0)
	require.Empty(t, headers)
}

func TestStartPublishesHeaderBeforeLogs(t *testing.T) {
	const height = int64(10)
	data := tmtypes.EventDataNewBlock{
		Block: &tmtypes.Block{
			Header: tmtypes.Header{Height: height},
			Data:   tmtypes.Data{Txs: tmtypes.Txs{[]byte("evm-tx")}},
		},
		ResultFinalizeBlock: abci.ResponseFinalizeBlock{
			TxResults: []*abci.ExecTxResult{evmTxResult(t, common.HexToAddress("0x01"), true)},
		},
	}

	s := &RPCStream{
		evtClient:    stubEventsClient{},
		logger:       log.NewNopLogger(),
		txDecoder:    func([]byte) (sdk.Tx, error) { return &mockCosmosOnlyTx{}, nil },
		headerStream: NewStream[RPCHeader](headerStreamSegmentSize, headerStreamCapacity),
		logStream:    NewStream[*ethtypes.Log](logStreamSegmentSize, logStreamCapacity),
	}
	s.validatorAccount = func(
		context.Context, *evmtypes.QueryValidatorAccountRequest, ...grpc.CallOption,
	) (*evmtypes.QueryValidatorAccountResponse, error) {
		logs, _ := s.logStream.ReadAllNonBlocking(0)
		require.Empty(t, logs, "the logs must not be published before the header is built")
		return &evmtypes.QueryValidatorAccountResponse{AccountAddress: sdk.AccAddress(make([]byte, 20)).String()}, nil
	}
	ch := make(chan coretypes.ResultEvent, 1)
	ch <- coretypes.ResultEvent{Data: data}
	close(ch)
	s.start(&s.wg, ch)

	headers, _ := s.headerStream.ReadAllNonBlocking(0)
	require.Len(t, headers, 1)
	logs, _ := s.logStream.ReadAllNonBlocking(0)
	require.Len(t, logs, 1)
}

// stubEventsClient is a no-op rpcclient.EventsClient.
type stubEventsClient struct{}

func (stubEventsClient) Subscribe(context.Context, string, string, ...int) (<-chan coretypes.ResultEvent, error) {
	return nil, nil
}
func (stubEventsClient) Unsubscribe(context.Context, string, string) error { return nil }
func (stubEventsClient) UnsubscribeAll(context.Context, string) error      { return nil }

func TestEvmTxHashFromEventData(t *testing.T) {
	t.Run("empty block returns EmptyRootHash", func(t *testing.T) {
		data := tmtypes.EventDataNewBlock{
			Block:               &tmtypes.Block{},
			ResultFinalizeBlock: abci.ResponseFinalizeBlock{},
		}
		txDecoder := func([]byte) (sdk.Tx, error) { return nil, nil }
		got, err := evmTxHashFromEventData(data, txDecoder)
		require.NoError(t, err)
		require.Equal(t, ethtypes.EmptyRootHash, got)
	})

	t.Run("all txs failed returns EmptyRootHash", func(t *testing.T) {
		data := tmtypes.EventDataNewBlock{
			Block: &tmtypes.Block{
				Data: tmtypes.Data{Txs: tmtypes.Txs{[]byte("tx")}},
			},
			ResultFinalizeBlock: abci.ResponseFinalizeBlock{
				TxResults: []*abci.ExecTxResult{{Code: 1}},
			},
		}
		txDecoder := func([]byte) (sdk.Tx, error) { return nil, nil }
		got, err := evmTxHashFromEventData(data, txDecoder)
		require.NoError(t, err)
		require.Equal(t, ethtypes.EmptyRootHash, got)
	})

	t.Run("txDecoder error returns error", func(t *testing.T) {
		data := tmtypes.EventDataNewBlock{
			Block: &tmtypes.Block{
				Data: tmtypes.Data{Txs: tmtypes.Txs{[]byte("invalid")}},
			},
			ResultFinalizeBlock: abci.ResponseFinalizeBlock{
				TxResults: []*abci.ExecTxResult{{Code: 0}},
			},
		}
		txDecoder := func([]byte) (sdk.Tx, error) {
			return nil, fmt.Errorf("cannot decode tx")
		}
		_, err := evmTxHashFromEventData(data, txDecoder)
		require.Error(t, err)
	})

	t.Run("tx/result count mismatch returns error", func(t *testing.T) {
		data := tmtypes.EventDataNewBlock{
			Block: &tmtypes.Block{
				Data: tmtypes.Data{Txs: tmtypes.Txs{[]byte("tx1"), []byte("tx2")}},
			},
			ResultFinalizeBlock: abci.ResponseFinalizeBlock{
				TxResults: []*abci.ExecTxResult{{Code: 0}}, // only 1 result for 2 txs
			},
		}
		txDecoder := func([]byte) (sdk.Tx, error) { return nil, nil }
		_, err := evmTxHashFromEventData(data, txDecoder)
		require.Error(t, err)
	})

	t.Run("no EVM messages returns EmptyRootHash", func(t *testing.T) {
		data := tmtypes.EventDataNewBlock{
			Block: &tmtypes.Block{
				Data: tmtypes.Data{Txs: tmtypes.Txs{[]byte("cosmos-tx")}},
			},
			ResultFinalizeBlock: abci.ResponseFinalizeBlock{
				TxResults: []*abci.ExecTxResult{{Code: 0}},
			},
		}
		txDecoder := func([]byte) (sdk.Tx, error) {
			return &mockCosmosOnlyTx{}, nil
		}
		got, err := evmTxHashFromEventData(data, txDecoder)
		require.NoError(t, err)
		require.Equal(t, ethtypes.EmptyRootHash, got)
	})

	t.Run("EVM tx produces non-empty trie root matching EvmTxHashFromMsgs", func(t *testing.T) {
		to := common.HexToAddress("0x1234567890123456789012345678901234567890")
		msg := evmtypes.NewTx(big.NewInt(9000), 0, &to, big.NewInt(0), 21000, big.NewInt(1), nil, nil, nil, nil)
		data := tmtypes.EventDataNewBlock{
			Block: &tmtypes.Block{
				Data: tmtypes.Data{Txs: tmtypes.Txs{[]byte("evm-tx")}},
			},
			ResultFinalizeBlock: abci.ResponseFinalizeBlock{
				TxResults: []*abci.ExecTxResult{{Code: 0}},
			},
		}
		txDecoder := func([]byte) (sdk.Tx, error) {
			return &mockEvmTx{msgs: []*evmtypes.MsgEthereumTx{msg}}, nil
		}
		got, err := evmTxHashFromEventData(data, txDecoder)
		require.NoError(t, err)
		require.NotEqual(t, ethtypes.EmptyRootHash, got)
		require.Equal(t, rpctypes.EvmTxHashFromMsgs([]*evmtypes.MsgEthereumTx{msg}), got)
	})
}

// mockCosmosOnlyTx is an sdk.Tx with no EVM messages.
type mockCosmosOnlyTx struct{}

func (m *mockCosmosOnlyTx) GetMsgs() []sdk.Msg                  { return []sdk.Msg{} }
func (m *mockCosmosOnlyTx) ValidateBasic() error                { return nil }
func (m *mockCosmosOnlyTx) GetMsgsV2() ([]proto.Message, error) { return nil, nil }

// mockEvmTx is an sdk.Tx wrapping one or more EVM messages.
type mockEvmTx struct{ msgs []*evmtypes.MsgEthereumTx }

func (m *mockEvmTx) GetMsgs() []sdk.Msg {
	out := make([]sdk.Msg, len(m.msgs))
	for i, msg := range m.msgs {
		out[i] = msg
	}
	return out
}
func (m *mockEvmTx) ValidateBasic() error                { return nil }
func (m *mockEvmTx) GetMsgsV2() ([]proto.Message, error) { return nil, nil }

package stream

import (
	"context"
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
	proto "google.golang.org/protobuf/proto"
)

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

func TestLogBlockTime(t *testing.T) {
	const blockTime = int64(1_700_000_000)

	testCases := []struct {
		name      string
		heights   []int64
		failAt    int64
		expTimes  []uint64
		expLookup int
	}{
		{"same height is fetched once", []int64{5, 5, 5}, 0, []uint64{1_700_000_005, 1_700_000_005, 1_700_000_005}, 1},
		{"new height is fetched again", []int64{5, 6}, 0, []uint64{1_700_000_005, 1_700_000_006}, 2},
		// A failed lookup must not be cached, so the next tx of the block retries.
		{"failed lookup returns zero and retries", []int64{5, 5}, 5, []uint64{0, 0}, 2},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			headers := &stubHeaderClient{baseTime: blockTime, failAt: tc.failAt}
			s := &RPCStream{logger: log.NewNopLogger(), headerClient: headers}

			for i, height := range tc.heights {
				require.Equal(t, tc.expTimes[i], s.logBlockTime(height))
			}
			require.Equal(t, tc.expLookup, headers.lookups)
		})
	}
}

// stubHeaderClient returns a header whose time is baseTime+height.
type stubHeaderClient struct {
	baseTime int64
	failAt   int64
	lookups  int
}

func (c *stubHeaderClient) Header(_ context.Context, height *int64) (*coretypes.ResultHeader, error) {
	c.lookups++
	if *height == c.failAt {
		return nil, fmt.Errorf("header not found")
	}
	return &coretypes.ResultHeader{Header: &tmtypes.Header{Time: time.Unix(c.baseTime+*height, 0)}}, nil
}

type stubEventsClient struct{}

func (stubEventsClient) Subscribe(context.Context, string, string, ...int) (<-chan coretypes.ResultEvent, error) {
	return nil, nil
}
func (stubEventsClient) Unsubscribe(context.Context, string, string) error { return nil }
func (stubEventsClient) UnsubscribeAll(context.Context, string) error      { return nil }

func TestStartPublishesLogsWithBlockTimestamp(t *testing.T) {
	const height = int64(5)
	rsp, err := codectypes.NewAnyWithValue(&evmtypes.MsgEthereumTxResponse{
		Logs: []*evmtypes.Log{{Address: common.HexToAddress("0x1").Hex()}},
	})
	require.NoError(t, err)
	data, err := gogoproto.Marshal(&sdk.TxMsgData{MsgResponses: []*codectypes.Any{rsp}})
	require.NoError(t, err)

	s := &RPCStream{
		evtClient:    stubEventsClient{},
		logger:       log.NewNopLogger(),
		headerClient: &stubHeaderClient{baseTime: 1_700_000_000},
		logStream:    NewStream[*ethtypes.Log](logStreamSegmentSize, logStreamCapacity),
	}
	chBlocks := make(chan coretypes.ResultEvent)
	close(chBlocks)
	chLogs := make(chan coretypes.ResultEvent, 1)
	chLogs <- coretypes.ResultEvent{
		Data: tmtypes.EventDataTx{TxResult: abci.TxResult{
			Height: height,
			Result: abci.ExecTxResult{Data: data},
		}},
		Events: map[string][]string{evmTxHashKey: {"0x1"}},
	}
	close(chLogs)

	// start returns once both channels are closed and drained.
	s.start(&s.wg, chBlocks, chLogs)

	logs, _ := s.logStream.ReadNonBlocking(0)
	require.Len(t, logs, 1)
	require.Equal(t, uint64(height), logs[0].BlockNumber)
	require.Equal(t, uint64(1_700_000_000+height), logs[0].BlockTimestamp)
}

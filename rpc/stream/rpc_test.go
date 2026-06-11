package stream

import (
	"fmt"
	"math/big"
	"testing"

	abci "github.com/cometbft/cometbft/abci/types"
	tmtypes "github.com/cometbft/cometbft/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
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

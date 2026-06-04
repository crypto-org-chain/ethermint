package stream

import (
	"fmt"
	"testing"

	abci "github.com/cometbft/cometbft/abci/types"
	tmtypes "github.com/cometbft/cometbft/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	ethtypes "github.com/ethereum/go-ethereum/core/types"
	proto "google.golang.org/protobuf/proto"
	"github.com/stretchr/testify/require"
)

func TestEvmTxHashFromEventData(t *testing.T) {
	t.Run("empty block returns EmptyRootHash", func(t *testing.T) {
		data := tmtypes.EventDataNewBlock{
			Block:               &tmtypes.Block{},
			ResultFinalizeBlock: abci.ResponseFinalizeBlock{},
		}
		txDecoder := func([]byte) (sdk.Tx, error) { return nil, nil }
		require.Equal(t, ethtypes.EmptyRootHash, evmTxHashFromEventData(data, txDecoder))
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
		require.Equal(t, ethtypes.EmptyRootHash, evmTxHashFromEventData(data, txDecoder))
	})

	t.Run("txDecoder error returns EmptyRootHash", func(t *testing.T) {
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
		require.Equal(t, ethtypes.EmptyRootHash, evmTxHashFromEventData(data, txDecoder))
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
		require.Equal(t, ethtypes.EmptyRootHash, evmTxHashFromEventData(data, txDecoder))
	})
}

// mockCosmosOnlyTx is an sdk.Tx with no EVM messages.
type mockCosmosOnlyTx struct{}

func (m *mockCosmosOnlyTx) GetMsgs() []sdk.Msg                      { return []sdk.Msg{} }
func (m *mockCosmosOnlyTx) ValidateBasic() error                     { return nil }
func (m *mockCosmosOnlyTx) GetMsgsV2() ([]proto.Message, error)      { return nil, nil }

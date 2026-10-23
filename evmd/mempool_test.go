package evmd

import (
	"testing"

	dbm "github.com/cosmos/cosmos-db"
	"github.com/stretchr/testify/require"

	"cosmossdk.io/log/v2"

	"github.com/cosmos/cosmos-sdk/baseapp"
	sdk "github.com/cosmos/cosmos-sdk/types"
	mempool "github.com/cosmos/cosmos-sdk/types/mempool"
	banktypes "github.com/cosmos/cosmos-sdk/x/bank/types"

	evmante "github.com/evmos/ethermint/ante"
	evmtypes "github.com/evmos/ethermint/x/evm/types"
)

func TestTxReplacement(t *testing.T) {
	eth := emptyEthExtTx{msgs: []sdk.Msg{&evmtypes.MsgEthereumTx{}}}
	batch := emptyEthExtTx{msgs: []sdk.Msg{&evmtypes.MsgEthereumTx{}, &evmtypes.MsgEthereumTx{}}}
	cosmos := emptyEthExtTx{msgs: []sdk.Msg{&banktypes.MsgSend{}}}
	for _, tc := range []struct {
		name     string
		op, np   int64
		oTx, nTx sdk.Tx
		exp      bool
	}{
		{"fee bump met", 100, 110, eth, eth, true},
		{"fee bump not met", 100, 109, eth, eth, false},
		{"equal priority after rounding", 5, 5, eth, eth, false},
		{"zero priority needs an increase", 0, 0, eth, eth, false},
		{"any increase over zero", 0, 1, eth, eth, true},
		{"cosmos tx is never replaced", 100, 1000, cosmos, cosmos, false},
		{"eth tx never replaces a cosmos tx", 100, 1000, cosmos, eth, false},
		{"batch never replaces", 100, 1000, eth, batch, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.exp, txReplacement(tc.op, tc.np, tc.oTx, tc.nTx))
		})
	}
}

func TestTxPool(t *testing.T) {
	priorityMempool := mempool.NewPriorityMempool(mempool.DefaultPriorityNonceMempoolConfig())
	for _, tc := range []struct {
		name    string
		mempool mempool.Mempool
		expPool evmante.TxPool
	}{
		{"priority nonce mempool", priorityMempool, priorityMempool},
		{"no-op mempool", mempool.NoOpMempool{}, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			bApp := baseapp.NewBaseApp("txpool-test", log.NewNopLogger(), dbm.NewMemDB(), nil)
			bApp.SetMempool(tc.mempool)
			// Compare the interfaces directly: a typed nil would not equal nil.
			require.True(t, (&EthermintApp{BaseApp: bApp}).txPool() == tc.expPool)
		})
	}
}

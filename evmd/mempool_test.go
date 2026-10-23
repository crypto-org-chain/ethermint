package evmd

import (
	"testing"

	dbm "github.com/cosmos/cosmos-db"
	"github.com/stretchr/testify/require"

	"cosmossdk.io/log/v2"

	"github.com/cosmos/cosmos-sdk/baseapp"
	mempool "github.com/cosmos/cosmos-sdk/types/mempool"

	evmante "github.com/evmos/ethermint/ante"
)

func TestTxReplacement(t *testing.T) {
	for _, tc := range []struct {
		name   string
		op, np int64
		exp    bool
	}{
		{"fee bump met", 100, 110, true},
		{"fee bump not met", 100, 109, false},
		{"equal priority after rounding", 5, 5, false},
		{"zero priority needs an increase", 0, 0, false},
		{"any increase over zero", 0, 1, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.exp, txReplacement(tc.op, tc.np, nil, nil))
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

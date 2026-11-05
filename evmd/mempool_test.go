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

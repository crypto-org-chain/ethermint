package server

import (
	"testing"

	"cosmossdk.io/log/v2"
	dbm "github.com/cosmos/cosmos-db"
	"github.com/cosmos/cosmos-sdk/server/types"
	"github.com/evmos/ethermint/evmd/ante"
	"github.com/stretchr/testify/require"
)

type mockApplication struct {
	types.Application
	pendingTxListeners []ante.PendingTxListener
}

func (m *mockApplication) RegisterPendingTxListener(listener ante.PendingTxListener) {
	m.pendingTxListeners = append(m.pendingTxListeners, listener)
}

func mockAppCreator(logger log.Logger, db dbm.DB, opts types.AppOptions) AppWithPendingTxListener {
	return &mockApplication{}
}

func TestNewDefaultStartOptions(t *testing.T) {
	defaultHome := "/tmp/test"

	opts := NewDefaultStartOptions(mockAppCreator, defaultHome)

	require.NotNil(t, opts.AppCreator)
	require.Equal(t, defaultHome, opts.DefaultNodeHome)
	require.NotNil(t, opts.DBOpener)

	logger := log.NewNopLogger()
	db := dbm.NewMemDB()
	var appOpts types.AppOptions

	app := opts.AppCreator(logger, db, appOpts)
	require.NotNil(t, app)

}

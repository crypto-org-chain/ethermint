package v8_test

import (
	"testing"

	storetypes "cosmossdk.io/store/types"
	"github.com/cosmos/cosmos-sdk/testutil"
	v8 "github.com/evmos/ethermint/x/evm/migrations/v8"
	"github.com/stretchr/testify/require"
)

func TestMigrateStore(t *testing.T) {
	storeKey := storetypes.NewKVStoreKey("evm")
	tKey := storetypes.NewTransientStoreKey("transient_test")
	ctx := testutil.DefaultContext(storeKey, tKey)

	// v8 migration is a no-op store change; it must always succeed.
	require.NoError(t, v8.MigrateStore(ctx))
}

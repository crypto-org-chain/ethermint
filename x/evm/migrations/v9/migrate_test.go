package v9_test

import (
	"math/big"
	"testing"

	storetypes "github.com/cosmos/cosmos-sdk/store/v2/types"
	"github.com/cosmos/cosmos-sdk/testutil"
	"github.com/ethereum/go-ethereum/common"
	v9 "github.com/evmos/ethermint/x/evm/migrations/v9"
	"github.com/evmos/ethermint/x/evm/types"
	"github.com/stretchr/testify/require"
)

func TestMigrateStore(t *testing.T) {
	storeKey := storetypes.NewKVStoreKey(types.ModuleName)
	tKey := storetypes.NewTransientStoreKey("transient_test")
	ctx := testutil.DefaultContext(storeKey, tKey)
	kvStore := ctx.KVStore(storeKey)

	slotKey := types.StateKey(common.BigToAddress(big.NewInt(1)), common.BigToHash(big.NewInt(1)))
	legacyValue := common.BigToHash(big.NewInt(7)).Bytes()
	kvStore.Set(slotKey, legacyValue)
	require.False(t, kvStore.Has(types.KeyPrefixCompactStorage))

	require.NoError(t, v9.MigrateStore(ctx, storeKey))

	require.True(t, kvStore.Has(types.KeyPrefixCompactStorage))
	// existing slots are not rewritten
	require.Equal(t, legacyValue, kvStore.Get(slotKey))
}

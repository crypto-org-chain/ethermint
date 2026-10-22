package v9_test

import (
	"math/big"
	"testing"

	storetypes "github.com/cosmos/cosmos-sdk/store/v2/types"
	"github.com/cosmos/cosmos-sdk/testutil"
	"github.com/ethereum/go-ethereum/common"
	"github.com/evmos/ethermint/encoding"
	v9 "github.com/evmos/ethermint/x/evm/migrations/v9"
	"github.com/evmos/ethermint/x/evm/types"
	"github.com/stretchr/testify/require"
)

func TestMigrateStore(t *testing.T) {
	cdc := encoding.MakeConfig().Codec
	storeKey := storetypes.NewKVStoreKey(types.ModuleName)
	tKey := storetypes.NewTransientStoreKey("transient_test")
	ctx := testutil.DefaultContext(storeKey, tKey)
	kvStore := ctx.KVStore(storeKey)

	v8Params := types.DefaultParams()
	v8Params.CompactStorage = false
	v8Params.StorageSweepBatch = 0
	kvStore.Set(types.KeyPrefixParams, cdc.MustMarshal(&v8Params))
	slotKey := types.StateKey(common.BigToAddress(big.NewInt(1)), common.BigToHash(big.NewInt(1)))
	legacyValue := common.BigToHash(big.NewInt(7)).Bytes()
	kvStore.Set(slotKey, legacyValue)

	require.NoError(t, v9.MigrateStore(ctx, storeKey, cdc))

	var params types.Params
	cdc.MustUnmarshal(kvStore.Get(types.KeyPrefixParams), &params)
	require.True(t, params.CompactStorage)
	require.Equal(t, uint64(10_000), params.StorageSweepBatch)
	params.CompactStorage = false
	params.StorageSweepBatch = 0
	require.Equal(t, v8Params, params)
	require.Equal(t, types.KeyPrefixStorage, kvStore.Get(types.KeyPrefixStorageSweep))
	// left for the sweep
	require.Equal(t, legacyValue, kvStore.Get(slotKey))
}

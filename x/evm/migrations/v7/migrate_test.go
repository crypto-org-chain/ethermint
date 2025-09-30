package v7_test

import (
	"testing"

	sdkmath "cosmossdk.io/math"
	storetypes "cosmossdk.io/store/types"
	"github.com/cosmos/cosmos-sdk/testutil"
	"github.com/evmos/ethermint/encoding"
	v7 "github.com/evmos/ethermint/x/evm/migrations/v7"
	"github.com/evmos/ethermint/x/evm/types"
	"github.com/stretchr/testify/require"
)

func TestMigrateStore(t *testing.T) {
	encCfg := encoding.MakeConfig()
	cdc := encCfg.Codec

	storeKey := storetypes.NewKVStoreKey(types.ModuleName)
	tKey := storetypes.NewTransientStoreKey("transient_test")
	ctx := testutil.DefaultContext(storeKey, tKey)
	kvStore := ctx.KVStore(storeKey)

	shanghaiTime := sdkmath.ZeroInt()
	v6Params := types.DefaultParams()
	v6Params.ChainConfig.ShanghaiTime = &shanghaiTime
	v6Params.ChainConfig.CancunTime = nil
	v6Params.ChainConfig.PragueTime = nil

	v6ParamsBz := cdc.MustMarshal(&v6Params)
	kvStore.Set(types.KeyPrefixParams, v6ParamsBz)

	require.NoError(t, v7.MigrateStore(ctx, storeKey, cdc))

	migratedParamsBz := kvStore.Get(types.KeyPrefixParams)
	require.NotNil(t, migratedParamsBz)

	var migratedParams types.Params
	cdc.MustUnmarshal(migratedParamsBz, &migratedParams)

	require.NotNil(t, migratedParams.ChainConfig.ShanghaiTime)
	require.Equal(t, shanghaiTime, *migratedParams.ChainConfig.ShanghaiTime)

	require.NotNil(t, migratedParams.ChainConfig.CancunTime)
	require.Equal(t, sdkmath.ZeroInt(), *migratedParams.ChainConfig.CancunTime)

	require.NotNil(t, migratedParams.ChainConfig.PragueTime)
	require.Equal(t, sdkmath.ZeroInt(), *migratedParams.ChainConfig.PragueTime)

	require.Equal(t, v6Params.EvmDenom, migratedParams.EvmDenom)
	require.Equal(t, v6Params.EnableCreate, migratedParams.EnableCreate)
	require.Equal(t, v6Params.EnableCall, migratedParams.EnableCall)
	require.Equal(t, v6Params.AllowUnprotectedTxs, migratedParams.AllowUnprotectedTxs)
	require.Equal(t, v6Params.ExtraEIPs, migratedParams.ExtraEIPs)

	require.Equal(t, v6Params.ChainConfig.HomesteadBlock, migratedParams.ChainConfig.HomesteadBlock)
	require.Equal(t, v6Params.ChainConfig.BerlinBlock, migratedParams.ChainConfig.BerlinBlock)
	require.Equal(t, v6Params.ChainConfig.LondonBlock, migratedParams.ChainConfig.LondonBlock)

	require.NoError(t, migratedParams.Validate())
}

func TestMigrateStoreEmptyStore(t *testing.T) {
	encCfg := encoding.MakeConfig()
	cdc := encCfg.Codec

	storeKey := storetypes.NewKVStoreKey(types.ModuleName)
	tKey := storetypes.NewTransientStoreKey("transient_test")
	ctx := testutil.DefaultContext(storeKey, tKey)

	err := v7.MigrateStore(ctx, storeKey, cdc)
	require.Error(t, err)
}

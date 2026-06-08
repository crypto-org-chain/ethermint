package v8_test

import (
	"math/big"
	"testing"

	sdkmath "cosmossdk.io/math"
	storetypes "github.com/cosmos/cosmos-sdk/store/v2/types"
	"github.com/cosmos/cosmos-sdk/testutil"
	"github.com/evmos/ethermint/encoding"
	v8 "github.com/evmos/ethermint/x/evm/migrations/v8"
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
	cancunTime := sdkmath.ZeroInt()
	pragueTime := sdkmath.ZeroInt()
	v7Params := types.DefaultParams()
	v7Params.ChainConfig.ShanghaiTime = &shanghaiTime
	v7Params.ChainConfig.CancunTime = &cancunTime
	v7Params.ChainConfig.PragueTime = &pragueTime
	v7Params.ChainConfig.OsakaTime = nil

	v7ParamsBz := cdc.MustMarshal(&v7Params)
	kvStore.Set(types.KeyPrefixParams, v7ParamsBz)

	require.NoError(t, v8.MigrateStore(ctx, storeKey, cdc))

	migratedParamsBz := kvStore.Get(types.KeyPrefixParams)
	require.NotNil(t, migratedParamsBz)

	var migratedParams types.Params
	cdc.MustUnmarshal(migratedParamsBz, &migratedParams)

	require.NotNil(t, migratedParams.ChainConfig.ShanghaiTime)
	require.Equal(t, shanghaiTime, *migratedParams.ChainConfig.ShanghaiTime)

	require.NotNil(t, migratedParams.ChainConfig.CancunTime)
	require.Equal(t, cancunTime, *migratedParams.ChainConfig.CancunTime)

	require.NotNil(t, migratedParams.ChainConfig.PragueTime)
	require.Equal(t, pragueTime, *migratedParams.ChainConfig.PragueTime)

	require.NotNil(t, migratedParams.ChainConfig.OsakaTime)
	require.Equal(t, sdkmath.ZeroInt(), *migratedParams.ChainConfig.OsakaTime)
	require.True(t, migratedParams.ChainConfig.EthereumConfig(big.NewInt(1)).IsOsaka(big.NewInt(0), 0))

	require.Equal(t, v7Params.EvmDenom, migratedParams.EvmDenom)
	require.Equal(t, v7Params.EnableCreate, migratedParams.EnableCreate)
	require.Equal(t, v7Params.EnableCall, migratedParams.EnableCall)
	require.Equal(t, v7Params.AllowUnprotectedTxs, migratedParams.AllowUnprotectedTxs)
	require.Equal(t, v7Params.ExtraEIPs, migratedParams.ExtraEIPs)

	require.NoError(t, migratedParams.Validate())
}

func TestMigrateStoreEmptyStore(t *testing.T) {
	encCfg := encoding.MakeConfig()
	cdc := encCfg.Codec

	storeKey := storetypes.NewKVStoreKey(types.ModuleName)
	tKey := storetypes.NewTransientStoreKey("transient_test")
	ctx := testutil.DefaultContext(storeKey, tKey)

	err := v8.MigrateStore(ctx, storeKey, cdc)
	require.Error(t, err)
}

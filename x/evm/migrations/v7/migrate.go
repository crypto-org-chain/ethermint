package v7

import (
	sdkmath "cosmossdk.io/math"
	"github.com/cosmos/cosmos-sdk/codec"
	storetypes "github.com/cosmos/cosmos-sdk/store/v2/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/evmos/ethermint/x/evm/types"
)

// MigrateStore migrates the x/evm module state from the consensus version 6 to
// version 7. Specifically, it adds CancunTime and PragueTime to the existing
// ChainConfig while preserving the ShanghaiTime from v6 migration.
func MigrateStore(
	ctx sdk.Context,
	storeKey storetypes.StoreKey,
	cdc codec.BinaryCodec,
) error {
	var params types.Params
	store := ctx.KVStore(storeKey)
	bz := store.Get(types.KeyPrefixParams)
	cdc.MustUnmarshal(bz, &params)
	zeroInt := sdkmath.ZeroInt()
	params.ChainConfig.CancunTime = &zeroInt
	params.ChainConfig.PragueTime = &zeroInt
	if err := params.Validate(); err != nil {
		return err
	}
	bz = cdc.MustMarshal(&params)
	store.Set(types.KeyPrefixParams, bz)
	return nil
}

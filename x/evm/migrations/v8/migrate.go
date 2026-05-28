package v8

import (
	sdkmath "cosmossdk.io/math"
	storetypes "cosmossdk.io/store/types"
	"github.com/cosmos/cosmos-sdk/codec"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/evmos/ethermint/x/evm/types"
)

// MigrateStore migrates the x/evm module state from consensus version 7 to
// version 8. Specifically, it adds OsakaTime to the existing ChainConfig.
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
	params.ChainConfig.OsakaTime = &zeroInt
	if err := params.Validate(); err != nil {
		return err
	}
	bz = cdc.MustMarshal(&params)
	store.Set(types.KeyPrefixParams, bz)
	return nil
}

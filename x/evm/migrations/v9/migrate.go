package v9

import (
	"bytes"

	"github.com/cosmos/cosmos-sdk/codec"
	storetypes "github.com/cosmos/cosmos-sdk/store/v2/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/evmos/ethermint/x/evm/types"
)

// MigrateStore migrates the x/evm module state from consensus version 8 to
// version 9: it turns on compact storage and starts the zero-slot sweep.
func MigrateStore(
	ctx sdk.Context,
	storeKey storetypes.StoreKey,
	cdc codec.BinaryCodec,
) error {
	var params types.Params
	store := ctx.KVStore(storeKey)
	bz := store.Get(types.KeyPrefixParams)
	cdc.MustUnmarshal(bz, &params)

	params.CompactStorage = true
	params.StorageSweepBatch = types.DefaultStorageSweepBatch
	if err := params.Validate(); err != nil {
		return err
	}
	store.Set(types.KeyPrefixParams, cdc.MustMarshal(&params))
	store.Set(types.KeyPrefixStorageSweep, bytes.Clone(types.KeyPrefixStorage))
	return nil
}

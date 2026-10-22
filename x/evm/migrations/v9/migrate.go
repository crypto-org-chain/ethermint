package v9

import (
	"github.com/cosmos/cosmos-sdk/codec"
	storetypes "github.com/cosmos/cosmos-sdk/store/v2/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/evmos/ethermint/x/evm/types"
)

// MigrateStore migrates the x/evm module state from consensus version 8 to
// version 9. It turns on the compact_storage param and starts a sweep that
// deletes slots stored as zero bytes, beginning in the upgrade block's
// BeginBlock; other existing slots keep their 32-byte encoding until they are
// next written.
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
	if err := params.Validate(); err != nil {
		return err
	}
	store.Set(types.KeyPrefixParams, cdc.MustMarshal(&params))
	store.Set(types.KeyPrefixStorageSweep, types.KeyPrefixStorage)
	return nil
}

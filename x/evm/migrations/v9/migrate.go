package v9

import (
	storetypes "github.com/cosmos/cosmos-sdk/store/v2/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/evmos/ethermint/x/evm/types"
)

// MigrateStore migrates the x/evm module state from consensus version 8 to
// version 9. It enables compact storage and starts a sweep that deletes slots
// stored as zero bytes over the following blocks; other existing slots keep
// their 32-byte encoding until they are next written.
func MigrateStore(ctx sdk.Context, storeKey storetypes.StoreKey) error {
	store := ctx.KVStore(storeKey)
	store.Set(types.KeyPrefixCompactStorage, []byte{1})
	store.Set(types.KeyPrefixStorageSweep, types.KeyPrefixStorage)
	return nil
}

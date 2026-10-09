package v9

import (
	storetypes "github.com/cosmos/cosmos-sdk/store/v2/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/evmos/ethermint/x/evm/types"
)

// MigrateStore migrates the x/evm module state from consensus version 8 to
// version 9. It enables compact storage for future writes only; existing slots
// keep their 32-byte encoding until they are next written.
func MigrateStore(ctx sdk.Context, storeKey storetypes.StoreKey) error {
	ctx.KVStore(storeKey).Set(types.KeyPrefixCompactStorage, []byte{1})
	return nil
}

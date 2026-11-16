package v5

import (
	"github.com/cosmos/cosmos-sdk/codec"
	storetypes "github.com/cosmos/cosmos-sdk/store/v2/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	v0types "github.com/evmos/ethermint/x/evm/migrations/v0/types"
	v4types "github.com/evmos/ethermint/x/evm/migrations/v4/types"
	"github.com/evmos/ethermint/x/evm/types"
)

// MigrateStore migrates the x/evm module state from the consensus version 4 to
// version 5. Specifically, it takes the parameters that are currently stored
// in separate keys and stores them directly into the x/evm module state using
// a single params key.
func MigrateStore(
	ctx sdk.Context,
	storeKey storetypes.StoreKey,
	cdc codec.BinaryCodec,
) error {
	store := ctx.KVStore(storeKey)
	params, _ := v4types.LoadParams(store, cdc)
	if err := params.Validate(); err != nil {
		return err
	}
	bz := cdc.MustMarshal(&params)
	store.Set(types.KeyPrefixParams, bz)
	store.Delete(v0types.ParamStoreKeyChainConfig)
	store.Delete(v0types.ParamStoreKeyExtraEIPs)
	store.Delete(v0types.ParamStoreKeyEVMDenom)
	store.Delete(v0types.ParamStoreKeyEnableCreate)
	store.Delete(v0types.ParamStoreKeyEnableCall)
	store.Delete(v0types.ParamStoreKeyAllowUnprotectedTxs)
	return nil
}

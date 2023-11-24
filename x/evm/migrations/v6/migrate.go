package v6

import (
	"github.com/cosmos/cosmos-sdk/codec"
	storetypes "github.com/cosmos/cosmos-sdk/store/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	v4types "github.com/evmos/ethermint/x/evm/migrations/v4/types"
	"github.com/evmos/ethermint/x/evm/types"
)

// MigrateStore migrates the x/evm module state from the consensus version 5 to
// version 6. Specifically, it replaces V4ChainConfig with ChainConfig which contains
// ShanghaiTime, CancunTime and PragueTime.
func MigrateStore(
	ctx sdk.Context,
	storeKey storetypes.StoreKey,
	cdc codec.BinaryCodec,
) error {
	var (
		params    v4types.V4Params
		newParams types.Params
	)
	store := ctx.KVStore(storeKey)
	bz := store.Get(types.KeyPrefixParams)
	cdc.MustUnmarshal(bz, &params)
	newParams = params.ToParams()
	shanghaiTime := sdk.ZeroInt()
	newParams.ChainConfig.ShanghaiTime = &shanghaiTime
	if err := newParams.Validate(); err != nil {
		return err
	}
	bz = cdc.MustMarshal(&newParams)
	store.Set(types.KeyPrefixParams, bz)
	return nil
}

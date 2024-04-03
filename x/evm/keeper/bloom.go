package keeper

import (
	"math/big"

	"cosmossdk.io/store/prefix"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/evmos/ethermint/x/evm/types"
)

func (k Keeper) SetTxBloom(ctx sdk.Context, bloom []byte) {
	store := ctx.KVStore(k.transientKey)
	store.Set(types.TransientBloomKey(ctx.TxIndex(), ctx.MsgIndex()), bloom)
}

func (k Keeper) CollectTxBloom(ctx sdk.Context) {
	store := prefix.NewStore(ctx.KVStore(k.transientKey), types.KeyPrefixTransientBloom)
	it := store.Iterator(nil, nil)
	defer it.Close()

	bloom := new(big.Int)
	for ; it.Valid(); it.Next() {
		bloom.Or(bloom, big.NewInt(0).SetBytes(it.Value()))
	}

	k.EmitBlockBloomEvent(ctx, bloom.Bytes())
}

// Copyright 2021 Evmos Foundation
// This file is part of Evmos' Ethermint library.
//
// The Ethermint library is free software: you can redistribute it and/or modify
// it under the terms of the GNU Lesser General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// The Ethermint library is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU Lesser General Public License for more details.
//
// You should have received a copy of the GNU Lesser General Public License
// along with the Ethermint library. If not, see https://github.com/evmos/ethermint/blob/main/LICENSE
package keeper

import (
	"bytes"

	storetypes "github.com/cosmos/cosmos-sdk/store/v2/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	ethermint "github.com/evmos/ethermint/types"
	"github.com/evmos/ethermint/x/evm/types"
)

// BeginBlock sets the EIP155 chain id, records block hashes, and runs a batch of the zero-slot sweep.
func (k *Keeper) BeginBlock(ctx sdk.Context) error {
	k.WithChainID(ctx)

	// cache parameters that's common for the whole block.
	cfg, err := k.EVMBlockConfig(ctx, k.ChainID())
	if err != nil {
		return err
	}
	k.SetHeaderHash(ctx)
	headerHashNum, err := ethermint.SafeInt64(cfg.Params.GetHeaderHashNum())
	if err != nil {
		panic(err)
	}
	if i := ctx.BlockHeight() - headerHashNum; i > 0 {
		h, err := ethermint.SafeUint64(i)
		if err != nil {
			panic(err)
		}
		k.DeleteHeaderHash(ctx, h)
	}
	k.sweepZeroStorage(ctx, cfg.Params.StorageSweepBatch)
	return nil
}

// sweepZeroStorage deletes legacy all-zero slots among the next limit storage entries from the sweep cursor,
// and sets storage_sweep_batch to 0 when the sweep finishes. It charges gas to ctx, so it belongs in BeginBlock.
func (k *Keeper) sweepZeroStorage(ctx sdk.Context, limit uint64) {
	if limit == 0 {
		return
	}
	store := ctx.KVStore(k.storeKey)
	cursor := store.Get(types.KeyPrefixStorageSweep)
	if cursor == nil {
		return
	}

	// delete after the iterator closes; writing while iterating is unsafe
	zeroSlots, next := collectZeroSlots(store, cursor, limit)
	for _, key := range zeroSlots {
		store.Delete(key)
	}
	if next == nil {
		store.Delete(types.KeyPrefixStorageSweep)
		// a zero batch lets later blocks skip reading the absent cursor
		params := k.GetParams(ctx)
		params.StorageSweepBatch = 0
		if err := k.SetParams(ctx, params); err != nil {
			k.Logger(ctx).Error("failed to reset storage sweep batch", "error", err)
		}
		k.Logger(ctx).Info("zero storage sweep finished")
		return
	}
	store.Set(types.KeyPrefixStorageSweep, next)
	k.Logger(ctx).Debug("zero storage sweep", "deleted", len(zeroSlots))
}

// collectZeroSlots returns next as nil at the end of storage.
func collectZeroSlots(store storetypes.KVStore, cursor []byte, limit uint64) (zeroSlots [][]byte, next []byte) {
	it := store.Iterator(cursor, storetypes.PrefixEndBytes(types.KeyPrefixStorage))
	defer it.Close()

	for i := uint64(0); i < limit && it.Valid(); i++ {
		if len(types.CompactStorageValue(it.Value())) == 0 {
			zeroSlots = append(zeroSlots, bytes.Clone(it.Key()))
		}
		it.Next()
	}
	if it.Valid() {
		next = bytes.Clone(it.Key())
	}
	return zeroSlots, next
}

// EndBlock also retrieves the bloom filter value from the transient store and commits it to the
// KVStore. The EVM end block logic doesn't update the validator set, thus it returns
// an empty slice.
func (k *Keeper) EndBlock(ctx sdk.Context) error {
	k.CollectTxBloom(ctx)
	k.RemoveParamsCache(ctx)
	return nil
}

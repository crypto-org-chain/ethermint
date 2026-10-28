package keeper

import sdk "github.com/cosmos/cosmos-sdk/types"

// SetQueryMaxGasLimitForTest sets the queryMaxGasLimit field for use in tests.
func (k *Keeper) SetQueryMaxGasLimitForTest(limit uint64) {
	k.queryMaxGasLimit = limit
}

// SweepZeroStorageForTest runs one zero-slot sweep batch outside BeginBlock.
func (k *Keeper) SweepZeroStorageForTest(ctx sdk.Context, limit uint64) {
	k.sweepZeroStorage(ctx, limit)
}

// IsStorageCompactForTest reports the compact_storage value SetState would use.
func (k *Keeper) IsStorageCompactForTest(ctx sdk.Context) bool {
	return k.isStorageCompact(ctx)
}

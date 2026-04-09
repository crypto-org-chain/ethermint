package keeper

// SetQueryMaxGasLimitForTest sets the queryMaxGasLimit field for use in tests.
func (k *Keeper) SetQueryMaxGasLimitForTest(limit uint64) {
	k.queryMaxGasLimit = limit
}

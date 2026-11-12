package keeper

import (
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/ethereum/go-ethereum/common"

	"github.com/evmos/ethermint/x/evm/types"
)

// SetQueryMaxGasLimitForTest sets the queryMaxGasLimit field for use in tests.
func (k *Keeper) SetQueryMaxGasLimitForTest(limit uint64) {
	k.queryMaxGasLimit = limit
}

// GetAccessListExcludesForTest exposes getAccessListExcludes to tests.
func (k Keeper) GetAccessListExcludesForTest(ctx sdk.Context, args types.TransactionArgs, cfg *EVMConfig) (map[common.Address]struct{}, error) {
	return k.getAccessListExcludes(ctx, args, cfg)
}

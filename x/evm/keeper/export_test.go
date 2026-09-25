package keeper

import (
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/ethereum/go-ethereum/common"
	ethtypes "github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/core/vm"
)

// SetQueryMaxGasLimitForTest sets the queryMaxGasLimit field for use in tests.
func (k *Keeper) SetQueryMaxGasLimitForTest(limit uint64) {
	k.queryMaxGasLimit = limit
}

func (k *Keeper) ApplyAuthorizationForTest(
	ctx sdk.Context, auth *ethtypes.SetCodeAuthorization, stateDB vm.StateDB,
) (common.Address, error) {
	return k.applyAuthorization(ctx, auth, stateDB)
}

func AccountCanStoreCodeHashForTest(acct sdk.AccountI) bool {
	return accountCanStoreCodeHash(acct)
}

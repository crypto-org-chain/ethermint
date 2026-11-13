// Package sdk55 holds the upgrade-handler steps an app needs when moving to cosmos-sdk v0.55;
// see docs/upgrade/cosmos-sdk-v0.55.md.
package sdk55

import (
	sdk "github.com/cosmos/cosmos-sdk/types"
	authkeeper "github.com/cosmos/cosmos-sdk/x/auth/keeper"
	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"
)

// EnsureModuleAccount converts a plain account anyone can create at the module address by funding
// it, keeping its number and balance; left plain, every module transfer to it panics.
func EnsureModuleAccount(ctx sdk.Context, ak authkeeper.AccountKeeper, name string) {
	addr, perms := ak.GetModuleAddressAndPermissions(name)
	acc := ak.GetAccount(ctx, addr)
	if acc == nil {
		ak.GetModuleAccount(ctx, name)
		return
	}
	if _, ok := acc.(sdk.ModuleAccountI); ok {
		return
	}
	base := authtypes.NewBaseAccount(addr, nil, acc.GetAccountNumber(), 0)
	ak.SetAccount(ctx, authtypes.NewModuleAccount(base, name, perms...))
}

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
package evmd

import (
	"context"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/types/module"
	authkeeper "github.com/cosmos/cosmos-sdk/x/auth/keeper"
	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"
	stakingtypes "github.com/cosmos/cosmos-sdk/x/staking/types"
	upgradetypes "github.com/cosmos/cosmos-sdk/x/upgrade/types"
)

func (app *EthermintApp) RegisterUpgradeHandlers() {
	app.UpgradeKeeper.SetUpgradeHandler("sdk53",
		func(ctx context.Context, _ upgradetypes.Plan, fromVM module.VersionMap) (module.VersionMap, error) {
			return app.ModuleManager.RunMigrations(ctx, app.configurator, fromVM)
		},
	)

	app.UpgradeKeeper.SetUpgradeHandler("sdk54",
		func(ctx context.Context, _ upgradetypes.Plan, fromVM module.VersionMap) (module.VersionMap, error) {
			return app.ModuleManager.RunMigrations(ctx, app.configurator, fromVM)
		},
	)

	// No store upgrade for the removed x/params: rootmulti drops an unmounted store from
	// the commit info by itself, and StoreUpgrades.Deleted only applies to mounted stores.
	app.UpgradeKeeper.SetUpgradeHandler("sdk55",
		func(ctx context.Context, _ upgradetypes.Plan, fromVM module.VersionMap) (module.VersionMap, error) {
			ensureModuleAccount(sdk.UnwrapSDKContext(ctx), app.AccountKeeper, stakingtypes.KeyRotationFeePoolName)
			return app.ModuleManager.RunMigrations(ctx, app.configurator, fromVM)
		},
	)
}

// ensureModuleAccount creates the module account for name, converting a plain account
// that anyone could have created at its address by funding it (keeping its number and
// balance); a plain account there makes every module transfer to it panic.
func ensureModuleAccount(ctx sdk.Context, ak authkeeper.AccountKeeper, name string) {
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

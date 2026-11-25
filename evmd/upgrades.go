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

	"github.com/cosmos/cosmos-sdk/runtime"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/types/module"
	stakingtypes "github.com/cosmos/cosmos-sdk/x/staking/types"
	upgradetypes "github.com/cosmos/cosmos-sdk/x/upgrade/types"
	channelv11 "github.com/cosmos/ibc-go/v11/modules/core/04-channel/migrations/v11"
	ibcexported "github.com/cosmos/ibc-go/v11/modules/core/exported"

	"github.com/evmos/ethermint/upgrades/sdk55"
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

	// No store upgrade for x/params: the SDK store drops an unmounted store from the commit info
	// by itself. memiavl does not, so a memiavl app must list it in StoreUpgrades.Deleted.
	app.UpgradeKeeper.SetUpgradeHandler("sdk55",
		func(ctx context.Context, _ upgradetypes.Plan, fromVM module.VersionMap) (module.VersionMap, error) {
			sdkCtx := sdk.UnwrapSDKContext(ctx)
			sdk55.EnsureModuleAccount(sdkCtx, app.AccountKeeper, stakingtypes.KeyRotationFeePoolName)
			// ibc-go v11.2.1 reads send sequences from a new key and registers no migration for it;
			// without this, sends on existing channels fail. A second run would reset them to 0.
			ibcStore := runtime.NewKVStoreService(app.keys[ibcexported.StoreKey])
			if err := channelv11.MigrateStore(sdkCtx, ibcStore, app.appCodec, app.IBCKeeper); err != nil {
				return nil, err
			}
			// Queue entries written before the pending-slot index existed never mature without this.
			if err := app.StakingKeeper.PopulateQueuePendingSlots(ctx); err != nil {
				return nil, err
			}
			return app.ModuleManager.RunMigrations(ctx, app.configurator, fromVM)
		},
	)
}

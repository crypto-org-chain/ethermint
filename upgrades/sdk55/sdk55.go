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

// Package sdk55 holds the upgrade-handler steps an app needs when moving to cosmos-sdk v0.55
// and ibc-go v11.2.1; see docs/upgrade/cosmos-sdk-v0.55.md.
package sdk55

import (
	corestore "cosmossdk.io/core/store"

	"github.com/cosmos/cosmos-sdk/codec"
	sdk "github.com/cosmos/cosmos-sdk/types"
	authkeeper "github.com/cosmos/cosmos-sdk/x/auth/keeper"
	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"
	channelv11 "github.com/cosmos/ibc-go/v11/modules/core/04-channel/migrations/v11"
	channeltypes "github.com/cosmos/ibc-go/v11/modules/core/04-channel/types"
	ibckeeper "github.com/cosmos/ibc-go/v11/modules/core/keeper"
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

// MigrateChannelSendSequences moves IBC v1 channel send sequences to the key ibc-go v11.2.1 reads,
// which ibc-go registers no migration for. Safe to run more than once, e.g. after a later ibc-go
// release registers the same migration.
func MigrateChannelSendSequences(
	ctx sdk.Context, ibcStoreService corestore.KVStoreService, cdc codec.BinaryCodec, ibcKeeper *ibckeeper.Keeper,
) error {
	// ibc-go's MigrateStore sets every channel without a v1 key to sequence 0, so first copy an
	// already-migrated sequence back to the v1 key for it to move again.
	store := ibcStoreService.OpenKVStore(ctx)
	var err error
	ibcKeeper.ChannelKeeper.IterateChannels(ctx, func(ic channeltypes.IdentifiedChannel) bool {
		v1Key := channelv11.NextSequenceSendV1Key(ic.PortId, ic.ChannelId)
		var has bool
		if has, err = store.Has(v1Key); err != nil || has {
			return err != nil
		}
		if seq, found := ibcKeeper.ChannelKeeperV2.GetNextSequenceSend(ctx, ic.ChannelId); found {
			err = store.Set(v1Key, sdk.Uint64ToBigEndian(seq))
		}
		return err != nil
	})
	if err != nil {
		return err
	}
	return channelv11.MigrateStore(ctx, ibcStoreService, cdc, ibcKeeper)
}

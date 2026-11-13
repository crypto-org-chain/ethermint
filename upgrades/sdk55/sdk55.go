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
// which ibc-go registers no migration for. Safe to repeat and to run after an upstream copy of the
// migration, but it cannot restore a sequence that a repeated upstream run already set to 0.
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
		if has, err = store.Has(v1Key); err != nil {
			return true
		}
		if has {
			return false
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

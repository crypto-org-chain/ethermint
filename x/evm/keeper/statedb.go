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
	"math/big"

	errorsmod "cosmossdk.io/errors"
	sdkmath "cosmossdk.io/math"
	"github.com/cosmos/cosmos-sdk/store/v2/prefix"
	storetypes "github.com/cosmos/cosmos-sdk/store/v2/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	ethermint "github.com/evmos/ethermint/types"
	"github.com/evmos/ethermint/x/evm/statedb"
	"github.com/evmos/ethermint/x/evm/types"
	"github.com/holiman/uint256"
)

var _ statedb.Keeper = &Keeper{}

// ----------------------------------------------------------------------------
// StateDB Keeper implementation
// ----------------------------------------------------------------------------

// GetState loads contract state from database, implements `statedb.Keeper` interface.
func (k *Keeper) GetState(ctx sdk.Context, addr common.Address, key common.Hash) common.Hash {
	store := prefix.NewStore(ctx.KVStore(k.storeKey), types.AddressStoragePrefix(addr))

	value := store.Get(key.Bytes())
	if len(value) == 0 {
		return common.Hash{}
	}

	return common.BytesToHash(value)
}

// GetCode loads contract code from database, implements `statedb.Keeper` interface.
func (k *Keeper) GetCode(ctx sdk.Context, codeHash common.Hash) []byte {
	store := prefix.NewStore(ctx.KVStore(k.storeKey), types.KeyPrefixCode)
	return store.Get(codeHash.Bytes())
}

// ForEachStorage iterate contract storage, callback return false to break early
func (k *Keeper) ForEachStorage(ctx sdk.Context, addr common.Address, cb func(key, value common.Hash) bool) {
	store := ctx.KVStore(k.storeKey)
	prefix := types.AddressStoragePrefix(addr)

	iterator := storetypes.KVStorePrefixIterator(store, prefix)
	defer iterator.Close()

	for ; iterator.Valid(); iterator.Next() {
		key := common.BytesToHash(iterator.Key())
		value := common.BytesToHash(iterator.Value())

		// check if iteration stops
		if !cb(key, value) {
			return
		}
	}
}

func (k *Keeper) Transfer(ctx sdk.Context, sender, recipient sdk.AccAddress, coins sdk.Coins) error {
	return k.bankKeeper.SendCoins(ctx, sender, recipient, coins)
}

func (k *Keeper) AddBalance(ctx sdk.Context, addr sdk.AccAddress, coin sdk.Coin) (uint256.Int, error) {
	coins := sdk.NewCoins(coin)
	prevBalance := k.GetBalance(ctx, addr, coin.Denom)
	if err := k.bankKeeper.MintCoins(ctx, types.ModuleName, coins); err != nil {
		return uint256.Int{}, err
	}
	if err := k.bankKeeper.SendCoinsFromModuleToAccount(ctx, types.ModuleName, addr, coins); err != nil {
		return uint256.Int{}, err
	}
	return prevBalance, nil
}

func (k *Keeper) SubBalance(ctx sdk.Context, addr sdk.AccAddress, coin sdk.Coin) (uint256.Int, error) {
	coins := sdk.NewCoins(coin)
	prevBalance := k.GetBalance(ctx, addr, coin.Denom)
	if err := k.bankKeeper.SendCoinsFromAccountToModule(ctx, addr, types.ModuleName, coins); err != nil {
		return uint256.Int{}, err
	}
	if err := k.bankKeeper.BurnCoins(ctx, types.ModuleName, coins); err != nil {
		return uint256.Int{}, err
	}
	return prevBalance, nil
}

// SetBalance reset the account's balance, mainly used by unit tests
func (k *Keeper) SetBalance(ctx sdk.Context, addr common.Address, amount uint256.Int, evmDenom string) error {
	cosmosAddr := sdk.AccAddress(addr.Bytes())
	balance := k.GetBalance(ctx, cosmosAddr, evmDenom)
	delta := new(big.Int).Sub(amount.ToBig(), balance.ToBig())
	switch delta.Sign() {
	case 1:
		coin := sdk.NewCoin(evmDenom, sdkmath.NewIntFromBigInt(delta))
		_, err := k.AddBalance(ctx, cosmosAddr, coin)
		return err
	case -1:
		coin := sdk.NewCoin(evmDenom, sdkmath.NewIntFromBigInt(new(big.Int).Abs(delta)))
		_, err := k.SubBalance(ctx, cosmosAddr, coin)
		return err
	default:
		return nil
	}
}

// SetAccount updates nonce/balance/codeHash together.
func (k *Keeper) SetAccount(ctx sdk.Context, addr common.Address, account statedb.Account) error {
	// update account
	cosmosAddr := sdk.AccAddress(addr.Bytes())
	acct := k.accountKeeper.GetAccount(ctx, cosmosAddr)
	if acct == nil {
		acct = k.accountKeeper.NewAccountWithAddress(ctx, cosmosAddr)
	}

	if err := acct.SetSequence(account.Nonce); err != nil {
		return err
	}

	codeHash := common.BytesToHash(account.CodeHash)

	if ethAcct, ok := acct.(ethermint.EthAccountI); ok {
		if err := ethAcct.SetCodeHash(codeHash); err != nil {
			return err
		}
	}

	k.accountKeeper.SetAccount(ctx, acct)

	k.debugLog(ctx, "account updated",
		"ethereum-address", addr,
		"nonce", account.Nonce,
		"codeHash", codeHash,
	)
	return nil
}

// SetState update contract storage, delete if value is empty.
// Once compact storage is enabled, leading zero bytes are trimmed, so a zero value deletes the slot.
func (k *Keeper) SetState(ctx sdk.Context, addr common.Address, key common.Hash, value []byte) {
	if len(value) > 0 && k.IsStorageCompact(ctx) {
		value = common.TrimLeftZeroes(value)
	}
	store := prefix.NewStore(ctx.KVStore(k.storeKey), types.AddressStoragePrefix(addr))
	action := "updated"
	if len(value) == 0 {
		store.Delete(key.Bytes())
		action = "deleted"
	} else {
		store.Set(key.Bytes(), value)
	}
	k.debugLog(ctx, "state",
		"action", action,
		"ethereum-address", addr,
		"key", key,
	)
}

// EnableCompactStorage applies to later writes only; values already stored stay readable in either format.
func (k *Keeper) EnableCompactStorage(ctx sdk.Context) {
	ctx.KVStore(k.storeKey).Set(types.KeyPrefixCompactStorage, []byte{1})
}

// IsStorageCompact bypasses the gas meter so writes made before the switch keep their original gas cost.
func (k *Keeper) IsStorageCompact(ctx sdk.Context) bool {
	return ctx.MultiStore().GetKVStore(k.storeKey).Has(types.KeyPrefixCompactStorage)
}

// SweepZeroStorage visits at most limit storage slots from the saved cursor and deletes those stored
// as zero bytes, which only the format before compact storage produced. It is a no-op once the sweep is done.
func (k *Keeper) SweepZeroStorage(ctx sdk.Context, limit int) {
	store := ctx.KVStore(k.storeKey)
	cursor := store.Get(types.KeyPrefixStorageSweep)
	if cursor == nil {
		return
	}

	var zeroSlots [][]byte
	it := store.Iterator(cursor, storetypes.PrefixEndBytes(types.KeyPrefixStorage))
	for i := 0; i < limit && it.Valid(); i++ {
		if len(common.TrimLeftZeroes(it.Value())) == 0 {
			zeroSlots = append(zeroSlots, bytes.Clone(it.Key()))
		}
		it.Next()
	}
	var next []byte
	if it.Valid() {
		next = bytes.Clone(it.Key())
	}
	// delete after closing, since writing to the store while iterating it is unsafe
	it.Close()

	for _, key := range zeroSlots {
		store.Delete(key)
	}
	if next == nil {
		store.Delete(types.KeyPrefixStorageSweep)
		k.Logger(ctx).Info("zero storage sweep finished", "deleted", len(zeroSlots))
		return
	}
	store.Set(types.KeyPrefixStorageSweep, next)
	k.Logger(ctx).Info("zero storage sweep progress", "deleted", len(zeroSlots), "next", hexutil.Encode(next))
}

// SetCode set contract code, delete if code is empty.
func (k *Keeper) SetCode(ctx sdk.Context, codeHash, code []byte) {
	store := prefix.NewStore(ctx.KVStore(k.storeKey), types.KeyPrefixCode)

	// store or delete code
	action := "updated"
	if len(code) == 0 {
		store.Delete(codeHash)
		action = "deleted"
	} else {
		store.Set(codeHash, code)
	}
	k.debugLog(ctx, "code",
		"action", action,
		"code-hash", codeHash,
	)
}

// DeleteAccount handles contract's suicide call:
// - remove code
// - remove states
// - remove auth account
//
// NOTE: balance should be cleared separately
func (k *Keeper) DeleteAccount(ctx sdk.Context, addr common.Address) error {
	cosmosAddr := sdk.AccAddress(addr.Bytes())
	acct := k.accountKeeper.GetAccount(ctx, cosmosAddr)
	if acct == nil {
		return nil
	}

	// NOTE: only Ethereum accounts (contracts) can be selfdestructed
	_, ok := acct.(ethermint.EthAccountI)
	if !ok {
		return errorsmod.Wrapf(types.ErrInvalidAccount, "type %T, address %s", acct, addr)
	}

	// clear storage
	k.ForEachStorage(ctx, addr, func(key, _ common.Hash) bool {
		k.SetState(ctx, addr, key, nil)
		return true
	})

	// remove auth account
	k.accountKeeper.RemoveAccount(ctx, acct)

	k.debugLog(ctx, "account suicided",
		"ethereum-address", addr,
		"cosmos-address", cosmosAddr,
	)

	return nil
}

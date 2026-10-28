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
	storetypes "github.com/cosmos/cosmos-sdk/store/v2/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	v0types "github.com/evmos/ethermint/x/evm/migrations/v0/types"
	"github.com/evmos/ethermint/x/evm/types"
)

// GetParams returns the total set of evm parameters.
func (k Keeper) GetParams(ctx sdk.Context) types.Params {
	params, ok := k.storedParams(ctx.KVStore(k.storeKey))
	if !ok {
		return k.GetLegacyParams(ctx)
	}
	return params
}

// storedParams returns false when the module store has no params, as on chains still using the legacy param store.
func (k Keeper) storedParams(store storetypes.KVStore) (types.Params, bool) {
	bz := store.Get(types.KeyPrefixParams)
	if len(bz) == 0 {
		return types.Params{}, false
	}
	var params types.Params
	k.cdc.MustUnmarshal(bz, &params)
	return params, true
}

// SetParams sets the EVM params each in their individual key for better get performance
func (k Keeper) SetParams(ctx sdk.Context, p types.Params) error {
	if err := p.Validate(); err != nil {
		return err
	}
	store := ctx.KVStore(k.storeKey)
	bz := k.cdc.MustMarshal(&p)
	store.Set(types.KeyPrefixParams, bz)
	return nil
}

// GetLegacyParams returns param set for version before migrate
func (k Keeper) GetLegacyParams(ctx sdk.Context) types.Params {
	params := v0types.V0Params{}
	k.ss.GetParamSetIfExists(ctx, &params)
	return params.ToParams()
}

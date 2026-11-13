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
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/evmos/ethermint/x/evm/types"
)

// GetParams returns the total set of evm parameters, or zero-value params when none are stored
// (before genesis, or at heights before the evm 4→5 migration).
func (k Keeper) GetParams(ctx sdk.Context) types.Params {
	var params types.Params
	bz := ctx.KVStore(k.storeKey).Get(types.KeyPrefixParams)
	if len(bz) == 0 {
		return params
	}
	k.cdc.MustUnmarshal(bz, &params)
	return params
}

// SetParams validates the EVM params and stores them under one key.
func (k Keeper) SetParams(ctx sdk.Context, p types.Params) error {
	if err := p.Validate(); err != nil {
		return err
	}
	if err := p.ChainConfig.ValidateAmsterdamDisabled(); err != nil {
		return err
	}
	store := ctx.KVStore(k.storeKey)
	bz := k.cdc.MustMarshal(&p)
	store.Set(types.KeyPrefixParams, bz)
	return nil
}

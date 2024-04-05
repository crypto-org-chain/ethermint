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

// GetParams returns the total set of evm parameters.
func (k Keeper) GetParams(ctx sdk.Context) types.Params {
	var params *types.Params
	objStore := ctx.ObjectStore(k.objectKey)
	v := objStore.Get(types.KeyPrefixObjectParams)
	if v == nil {
		store := k.storeService.OpenKVStore(ctx)
		bz, err := store.Get(types.KeyPrefixParams)
		if err != nil {
			panic(err)
		}
		params = new(types.Params)
		if bz != nil {
			k.cdc.MustUnmarshal(bz, params)
		}

		objStore.Set(types.KeyPrefixObjectParams, params)
	} else {
		params = v.(*types.Params)
	}

	return *params
}

// SetParams sets the EVM params each in their individual key for better get performance
func (k Keeper) SetParams(ctx sdk.Context, p types.Params) error {
	if err := p.Validate(); err != nil {
		return err
	}
	store := k.storeService.OpenKVStore(ctx)
	bz := k.cdc.MustMarshal(&p)
	if err := store.Set(types.KeyPrefixParams, bz); err != nil {
		return err
	}

	// set to cache as well, decode again to be compatible with the previous behavior
	var params types.Params
	k.cdc.MustUnmarshal(bz, &params)
	ctx.ObjectStore(k.objectKey).Set(types.KeyPrefixObjectParams, &params)

	return nil
}

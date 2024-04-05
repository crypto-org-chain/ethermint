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
	"math/big"

	ethermint "github.com/evmos/ethermint/types"
	"github.com/evmos/ethermint/x/feemarket/types"

	sdk "github.com/cosmos/cosmos-sdk/types"
)

// GetParams returns the total set of fee market parameters.
func (k Keeper) GetParams(ctx sdk.Context) types.Params {
	var params *types.Params
	objStore := ctx.ObjectStore(k.objectKey)
	v := objStore.Get(types.KeyPrefixObjectParams)
	if v == nil {
		params = new(types.Params)
		bz := ctx.KVStore(k.storeKey).Get(types.ParamsKey)
		if bz != nil {
			k.cdc.MustUnmarshal(bz, params)
		}
		objStore.Set(types.KeyPrefixObjectParams, params)
	} else {
		params = v.(*types.Params)
	}
	return *params
}

// SetParams sets the fee market params in a single key
func (k Keeper) SetParams(ctx sdk.Context, p types.Params) error {
	if err := p.Validate(); err != nil {
		return err
	}
	store := ctx.KVStore(k.storeKey)
	bz := k.cdc.MustMarshal(&p)
	store.Set(types.ParamsKey, bz)

	// set to cache as well, decode again to be compatible with the previous behavior
	var params types.Params
	k.cdc.MustUnmarshal(bz, &params)
	ctx.ObjectStore(k.objectKey).Set(types.KeyPrefixObjectParams, &params)

	return nil
}

// ----------------------------------------------------------------------------
// Parent Base Fee
// Required by EIP1559 base fee calculation.
// ----------------------------------------------------------------------------

// GetBaseFee gets the base fee from the store
func (k Keeper) GetBaseFee(ctx sdk.Context) *big.Int {
	params := k.GetParams(ctx)
	if params.NoBaseFee {
		return nil
	}

	baseFee := params.BaseFee.BigInt()
	if baseFee == nil || baseFee.Sign() == 0 {
		// try v1 format
		return k.GetBaseFeeV1(ctx)
	}
	return baseFee
}

// SetBaseFee set's the base fee in the store
func (k Keeper) SetBaseFee(ctx sdk.Context, baseFee *big.Int) {
	params := k.GetParams(ctx)
	params.BaseFee = ethermint.SaturatedNewInt(baseFee)
	err := k.SetParams(ctx, params)
	if err != nil {
		return
	}
}

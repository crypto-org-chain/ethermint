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
	"github.com/evmos/ethermint/types/legacyparams"
	v0types "github.com/evmos/ethermint/x/evm/migrations/v0/types"
	v4types "github.com/evmos/ethermint/x/evm/migrations/v4/types"
	"github.com/evmos/ethermint/x/evm/types"
)

// GetParams returns the total set of evm parameters. At heights before the 4→5 migration it reads where
// params were stored then, so historical queries run with that height's chain config.
func (k Keeper) GetParams(ctx sdk.Context) types.Params {
	store := ctx.KVStore(k.storeKey)
	if bz := store.Get(types.KeyPrefixParams); len(bz) != 0 {
		var params types.Params
		k.cdc.MustUnmarshal(bz, &params)
		return params
	}
	// Version 4 always stored the chain config, so one lookup tells the v4 layout from the x/params one.
	if store.Has(v0types.ParamStoreKeyChainConfig) {
		return k.getV4Params(ctx, v4types.LoadParams(store, k.cdc).ToParams())
	}
	return k.getLegacyParams(ctx)
}

// getV4Params mirrors the version 4 getters, which read x/params for each key that was absent: a false
// bool, an empty denom or empty extra EIPs. The 3→4 migration never cleared x/params, so a param turned
// off later still reads its old x/params value, as the node did at that height.
func (k Keeper) getV4Params(ctx sdk.Context, params types.Params) types.Params {
	if params.EvmDenom != "" && params.EnableCreate && params.EnableCall && params.AllowUnprotectedTxs &&
		len(params.ExtraEIPs) != 0 {
		return params
	}
	legacy := k.getLegacyParams(ctx)
	if params.EvmDenom == "" {
		params.EvmDenom = legacy.EvmDenom
	}
	params.EnableCreate = params.EnableCreate || legacy.EnableCreate
	params.EnableCall = params.EnableCall || legacy.EnableCall
	params.AllowUnprotectedTxs = params.AllowUnprotectedTxs || legacy.AllowUnprotectedTxs
	if len(params.ExtraEIPs) == 0 {
		params.ExtraEIPs = legacy.ExtraEIPs
	}
	return params
}

// getLegacyParams returns the params x/params held before the 3→4 migration, or zero-value params when
// the app passed no legacy subspace or nothing is stored (before genesis).
func (k Keeper) getLegacyParams(ctx sdk.Context) types.Params {
	var params v4types.V4Params
	k.legacyParams.GetIfExists(ctx,
		legacyparams.Pair{Key: v0types.ParamStoreKeyEVMDenom, Value: &params.EvmDenom},
		legacyparams.Pair{Key: v0types.ParamStoreKeyEnableCreate, Value: &params.EnableCreate},
		legacyparams.Pair{Key: v0types.ParamStoreKeyEnableCall, Value: &params.EnableCall},
		legacyparams.Pair{Key: v0types.ParamStoreKeyExtraEIPs, Value: &params.ExtraEIPs.EIPs},
		legacyparams.Pair{Key: v0types.ParamStoreKeyChainConfig, Value: &params.ChainConfig},
		legacyparams.Pair{Key: v0types.ParamStoreKeyAllowUnprotectedTxs, Value: &params.AllowUnprotectedTxs},
	)
	return params.ToParams()
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

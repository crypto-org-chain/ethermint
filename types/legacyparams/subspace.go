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

// Package legacyparams reads module params that the removed x/params module stored, so queries at
// heights before a module moved its params into its own store still get them.
package legacyparams

import (
	"github.com/cosmos/cosmos-sdk/codec"
	"github.com/cosmos/cosmos-sdk/store/v2/prefix"
	storetypes "github.com/cosmos/cosmos-sdk/store/v2/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
)

// Pair is one stored param: its key in the subspace and a pointer to decode the value into.
type Pair struct {
	Key   []byte
	Value any
}

// Subspace reads one module's subspace in the store x/params was mounted under. The zero value reads nothing,
// for apps that dropped that store or never had params there.
type Subspace struct {
	key    storetypes.StoreKey
	prefix []byte
	amino  *codec.LegacyAmino
}

// NewSubspace reads the subspace that x/params named after the module. The app must keep key mounted.
func NewSubspace(key storetypes.StoreKey, name string) Subspace {
	return Subspace{key: key, prefix: []byte(name + "/"), amino: codec.NewLegacyAmino()}
}

// GetIfExists decodes each stored pair into its Value and leaves missing ones unchanged. It panics on a value
// that does not decode, as x/params did.
func (s Subspace) GetIfExists(ctx sdk.Context, pairs ...Pair) {
	if s.key == nil {
		return
	}
	store := prefix.NewStore(ctx.KVStore(s.key), s.prefix)
	for _, pair := range pairs {
		bz := store.Get(pair.Key)
		if bz == nil {
			continue
		}
		if err := s.amino.UnmarshalJSON(bz, pair.Value); err != nil {
			panic(err)
		}
	}
}

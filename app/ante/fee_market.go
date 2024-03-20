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
package ante

import (
	"math/big"

	errorsmod "cosmossdk.io/errors"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/ethereum/go-ethereum/params"
	feemarkettypes "github.com/evmos/ethermint/x/feemarket/types"
)

// GasWantedDecorator keeps track of the gasWanted amount on the current block in transient store
// for BaseFee calculation.
// NOTE: This decorator does not perform any validation
type GasWantedDecorator struct {
	feeMarketKeeper FeeMarketKeeper
	ethCfg          *params.ChainConfig
	feemarketParams *feemarkettypes.Params
}

// NewGasWantedDecorator creates a new NewGasWantedDecorator
func NewGasWantedDecorator(
	feeMarketKeeper FeeMarketKeeper,
	ethCfg *params.ChainConfig,
	feemarketParams *feemarkettypes.Params,
) GasWantedDecorator {
	return GasWantedDecorator{
		feeMarketKeeper,
		ethCfg,
		feemarketParams,
	}
}

func (gwd GasWantedDecorator) AnteHandle(ctx sdk.Context, tx sdk.Tx, simulate bool, next sdk.AnteHandler) (newCtx sdk.Context, err error) {
	blockHeight := big.NewInt(ctx.BlockHeight())
	isLondon := gwd.ethCfg.IsLondon(blockHeight)

	feeTx, ok := tx.(sdk.FeeTx)
	if !ok || !isLondon {
		return next(ctx, tx, simulate)
	}

	if err := CheckGasWanted(ctx, feeTx, gwd.ethCfg, gwd.feeMarketKeeper, gwd.feemarketParams); err != nil {
		return ctx, err
	}

	return next(ctx, tx, simulate)
}

func CheckGasWanted(
	ctx sdk.Context, feeTx sdk.FeeTx,
	ethCfg *params.ChainConfig,
	feeMarketKeeper FeeMarketKeeper,
	feeMarketParams *feemarkettypes.Params,
) error {
	blockHeight := big.NewInt(ctx.BlockHeight())
	if !ethCfg.IsLondon(blockHeight) {
		return nil
	}

	gasWanted := feeTx.GetGas()
	isBaseFeeEnabled := feeMarketParams.IsBaseFeeEnabled(ctx.BlockHeight())

	// Add total gasWanted to cumulative in block transientStore in FeeMarket module
	if isBaseFeeEnabled {
		if _, err := feeMarketKeeper.AddTransientGasWanted(ctx, gasWanted); err != nil {
			return errorsmod.Wrapf(err, "failed to add gas wanted to transient store")
		}
	}

	return nil
}

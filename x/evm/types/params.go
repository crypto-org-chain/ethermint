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
package types

import (
	"fmt"
	"math"
	"math/big"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/ethereum/go-ethereum/core/vm"
	"github.com/ethereum/go-ethereum/params"
	"github.com/evmos/ethermint/types"
)

var (
	// DefaultEVMDenom defines the default EVM denomination on Ethermint
	DefaultEVMDenom = types.AttoPhoton
	// DefaultAllowUnprotectedTxs rejects all unprotected txs (i.e false)
	DefaultAllowUnprotectedTxs = false
	// DefaultEnableCreate enables contract creation (i.e true)
	DefaultEnableCreate = true
	// DefaultEnableCall enables contract calls (i.e true)
	DefaultEnableCall = true
	// DefaultHeaderHashNum defines the default number of header hash to persist.
	DefaultHeaderHashNum = uint64(256)
	// DefaultHistoryServeWindow DefaultHeaderHashNum defines the default number of hystorical value to serve for EIP2935.
	DefaultHistoryServeWindow = uint64(8191)
)

// NewParams creates a new Params instance
func NewParams(evmDenom string, allowUnprotectedTxs, enableCreate, enableCall bool, config ChainConfig, extraEIPs []int64) Params {
	return Params{
		EvmDenom:            evmDenom,
		AllowUnprotectedTxs: allowUnprotectedTxs,
		EnableCreate:        enableCreate,
		EnableCall:          enableCall,
		ExtraEIPs:           extraEIPs,
		ChainConfig:         config,
	}
}

// DefaultParams returns default evm parameters
// ExtraEIPs is empty to prevent overriding the latest hard fork instruction set
func DefaultParams() Params {
	config := DefaultChainConfig()
	return Params{
		EvmDenom:            DefaultEVMDenom,
		EnableCreate:        DefaultEnableCreate,
		EnableCall:          DefaultEnableCall,
		ChainConfig:         config,
		AllowUnprotectedTxs: DefaultAllowUnprotectedTxs,
		HeaderHashNum:       DefaultHeaderHashNum,
		HistoryServeWindow:  DefaultHistoryServeWindow,
	}
}

// Validate performs basic validation on evm parameters.
func (p Params) Validate() error {
	if err := ValidateEVMDenom(p.EvmDenom); err != nil {
		return err
	}

	if err := validateEIPs(p.ExtraEIPs); err != nil {
		return err
	}

	if err := ValidateInt64Overflow(p.HeaderHashNum); err != nil {
		return err
	}

	if err := ValidateInt64Overflow(p.HistoryServeWindow); err != nil {
		return err
	}

	return p.ChainConfig.Validate()
}

// EffectiveHistoryServeWindow returns the EIP-2935 ring size, using the default when HistoryServeWindow is unset.
func (p Params) EffectiveHistoryServeWindow() uint64 {
	if p.HistoryServeWindow == 0 {
		return DefaultHistoryServeWindow
	}
	return p.HistoryServeWindow
}

// EIPs returns the ExtraEIPS as a int slice
func (p Params) EIPs() []int {
	eips := make([]int, len(p.ExtraEIPs))
	for i, eip := range p.ExtraEIPs {
		eips[i] = int(eip)
	}
	return eips
}

func ValidateEVMDenom(denom string) error {
	return sdk.ValidateDenom(denom)
}

// _amsterdamEIPs are activateable in go-ethereum but depend on Amsterdam runtime
// support (state gas, slot number) that ethermint does not implement; keep them
// rejected for as long as amsterdam_time is.
var _amsterdamEIPs = map[int64]struct{}{
	7843: {},
	8024: {},
	8037: {},
	8038: {},
}

func validateEIPs(eips []int64) error {
	for _, eip := range eips {
		if _, ok := _amsterdamEIPs[eip]; ok {
			return fmt.Errorf("EIP %d cannot be activated: Amsterdam is not supported yet", eip)
		}
		if !vm.ValidEip(int(eip)) {
			return fmt.Errorf("EIP %d is not activateable, valid EIPS are: %s", eip, vm.ActivateableEips())
		}
	}
	return nil
}

func ValidateInt64Overflow(num uint64) error {
	if num > math.MaxInt64 {
		return fmt.Errorf("value too large: %d, maximum value is: %d", num, uint64(math.MaxInt64))
	}
	return nil
}

// IsLondon returns if london hardfork is enabled.
func IsLondon(ethConfig *params.ChainConfig, height int64) bool {
	return ethConfig.IsLondon(big.NewInt(height))
}

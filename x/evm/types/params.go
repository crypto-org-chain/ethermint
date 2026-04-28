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

const (
	// MaxMaxEthMsgsPerTx is a hard upper bound to prevent pathological lane amplification.
	MaxMaxEthMsgsPerTx = uint32(1024)
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
	// DefaultMaxEthMsgsPerTx defines the default max amount of MsgEthereumTx messages
	// allowed in one extension-options Ethereum transaction envelope.
	DefaultMaxEthMsgsPerTx = uint32(64)
)

// NewParams creates a new Params instance
func NewParams(
	evmDenom string,
	allowUnprotectedTxs, enableCreate, enableCall bool,
	config ChainConfig,
	extraEIPs []int64,
	maxEthMsgsPerTx uint32,
) Params {
	return Params{
		EvmDenom:            evmDenom,
		AllowUnprotectedTxs: allowUnprotectedTxs,
		EnableCreate:        enableCreate,
		EnableCall:          enableCall,
		ExtraEIPs:           extraEIPs,
		ChainConfig:         config,
		MaxEthMsgsPerTx:     maxEthMsgsPerTx,
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
		// Zero means use DefaultMaxEthMsgsPerTx at read time.
		MaxEthMsgsPerTx: 0,
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

	if err := ValidateBool(p.EnableCall); err != nil {
		return err
	}

	if err := ValidateBool(p.EnableCreate); err != nil {
		return err
	}

	if err := ValidateBool(p.AllowUnprotectedTxs); err != nil {
		return err
	}

	if err := ValidateInt64Overflow(p.HeaderHashNum); err != nil {
		return err
	}

	if err := ValidateInt64Overflow(p.HistoryServeWindow); err != nil {
		return err
	}

	if err := ValidateMaxEthMsgsPerTx(p.MaxEthMsgsPerTx); err != nil {
		return err
	}

	return ValidateChainConfig(p.ChainConfig)
}

// MaxEthMsgsPerTxOrDefault returns the effective max number of MsgEthereumTx
// messages allowed in one envelope.
func (p Params) MaxEthMsgsPerTxOrDefault() uint32 {
	if p.MaxEthMsgsPerTx == 0 {
		return DefaultMaxEthMsgsPerTx
	}
	return p.MaxEthMsgsPerTx
}

// EIPs returns the ExtraEIPS as a int slice
func (p Params) EIPs() []int {
	eips := make([]int, len(p.ExtraEIPs))
	for i, eip := range p.ExtraEIPs {
		eips[i] = int(eip)
	}
	return eips
}

func ValidateEVMDenom(i interface{}) error {
	denom, ok := i.(string)
	if !ok {
		return fmt.Errorf("invalid parameter EVM denom type: %T", i)
	}

	return sdk.ValidateDenom(denom)
}

func ValidateBool(i interface{}) error {
	_, ok := i.(bool)
	if !ok {
		return fmt.Errorf("invalid parameter type: %T", i)
	}
	return nil
}

func validateEIPs(i interface{}) error {
	eips, ok := i.([]int64)
	if !ok {
		return fmt.Errorf("invalid EIP slice type: %T", i)
	}

	for _, eip := range eips {
		if !vm.ValidEip(int(eip)) {
			return fmt.Errorf("EIP %d is not activateable, valid EIPS are: %s", eip, vm.ActivateableEips())
		}
	}
	return nil
}

func ValidateChainConfig(i interface{}) error {
	cfg, ok := i.(ChainConfig)
	if !ok {
		return fmt.Errorf("invalid chain config type: %T", i)
	}
	return cfg.Validate()
}

func ValidateInt64Overflow(i interface{}) error {
	num, ok := i.(uint64)
	if !ok {
		return fmt.Errorf("invalid parameter type: %T", i)
	}
	if num > math.MaxInt64 {
		return fmt.Errorf("value too large: %d, maximum value is: %d", num, uint64(math.MaxInt64))
	}
	return nil
}

func ValidateMaxEthMsgsPerTx(i interface{}) error {
	maxEthMsgsPerTx, ok := i.(uint32)
	if !ok {
		return fmt.Errorf("invalid parameter max eth msgs per tx type: %T", i)
	}
	if maxEthMsgsPerTx == 0 {
		return nil
	}
	if maxEthMsgsPerTx > MaxMaxEthMsgsPerTx {
		return fmt.Errorf("max eth msgs per tx must be between 0 and %d: %d", MaxMaxEthMsgsPerTx, maxEthMsgsPerTx)
	}
	return nil
}

// IsLondon returns if london hardfork is enabled.
func IsLondon(ethConfig *params.ChainConfig, height int64) bool {
	return ethConfig.IsLondon(big.NewInt(height))
}

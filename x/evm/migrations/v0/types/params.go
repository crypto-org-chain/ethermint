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

	"github.com/ethereum/go-ethereum/core/vm"
	currenttypes "github.com/evmos/ethermint/x/evm/types"
)

func (params V0Params) ToParams() currenttypes.Params {
	chainConfig := currenttypes.ChainConfig{
		HomesteadBlock:      params.ChainConfig.HomesteadBlock,
		DAOForkBlock:        params.ChainConfig.DAOForkBlock,
		DAOForkSupport:      params.ChainConfig.DAOForkSupport,
		EIP150Block:         params.ChainConfig.EIP150Block,
		EIP150Hash:          params.ChainConfig.EIP150Hash,
		EIP155Block:         params.ChainConfig.EIP155Block,
		EIP158Block:         params.ChainConfig.EIP158Block,
		ByzantiumBlock:      params.ChainConfig.ByzantiumBlock,
		ConstantinopleBlock: params.ChainConfig.ConstantinopleBlock,
		PetersburgBlock:     params.ChainConfig.PetersburgBlock,
		IstanbulBlock:       params.ChainConfig.IstanbulBlock,
		MuirGlacierBlock:    params.ChainConfig.MuirGlacierBlock,
		BerlinBlock:         params.ChainConfig.BerlinBlock,
		LondonBlock:         params.ChainConfig.LondonBlock,
		ArrowGlacierBlock:   params.ChainConfig.ArrowGlacierBlock,
		GrayGlacierBlock:    params.ChainConfig.GrayGlacierBlock,
		MergeNetsplitBlock:  params.ChainConfig.MergeNetsplitBlock,
	}
	return currenttypes.Params{
		EvmDenom:            params.EvmDenom,
		EnableCreate:        params.EnableCreate,
		EnableCall:          params.EnableCall,
		ExtraEIPs:           params.ExtraEIPs,
		AllowUnprotectedTxs: params.AllowUnprotectedTxs,
		ChainConfig:         chainConfig,
	}
}

// Validate performs basic validation on evm parameters.
func (p V0Params) Validate() error {
	if err := currenttypes.ValidateEVMDenom(p.EvmDenom); err != nil {
		return err
	}

	if err := validateEIPs(p.ExtraEIPs); err != nil {
		return err
	}

	if err := currenttypes.ValidateBool(p.EnableCall); err != nil {
		return err
	}

	if err := currenttypes.ValidateBool(p.EnableCreate); err != nil {
		return err
	}

	if err := currenttypes.ValidateBool(p.AllowUnprotectedTxs); err != nil {
		return err
	}

	return ValidateChainConfig(p.ChainConfig)
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
	cfg, ok := i.(V0ChainConfig)
	if !ok {
		return fmt.Errorf("invalid chain config type: %T", i)
	}
	return cfg.Validate()
}

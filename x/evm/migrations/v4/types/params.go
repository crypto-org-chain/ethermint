package types

import (
	fmt "fmt"

	"github.com/ethereum/go-ethereum/core/vm"
	v0types "github.com/evmos/ethermint/x/evm/migrations/v0/types"
	currenttypes "github.com/evmos/ethermint/x/evm/types"
)

func (params V4Params) ToParams() currenttypes.Params {
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
		ExtraEIPs:           params.ExtraEIPs.EIPs,
		AllowUnprotectedTxs: params.AllowUnprotectedTxs,
		ChainConfig:         chainConfig,
	}
}

// Validate performs basic validation on evm parameters.
func (p V4Params) Validate() error {
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

	return v0types.ValidateChainConfig(p.ChainConfig)
}

func validateEIPs(i interface{}) error {
	eips, ok := i.(ExtraEIPs)
	if !ok {
		return fmt.Errorf("invalid EIP slice type: %T", i)
	}

	for _, eip := range eips.EIPs {
		if !vm.ValidEip(int(eip)) {
			return fmt.Errorf("EIP %d is not activateable, valid EIPS are: %s", eip, vm.ActivateableEips())
		}
	}

	return nil
}

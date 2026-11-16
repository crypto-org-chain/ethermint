package types

import (
	fmt "fmt"

	"github.com/cosmos/cosmos-sdk/codec"
	storetypes "github.com/cosmos/cosmos-sdk/store/v2/types"
	"github.com/ethereum/go-ethereum/core/vm"
	v0types "github.com/evmos/ethermint/x/evm/migrations/v0/types"
	currenttypes "github.com/evmos/ethermint/x/evm/types"
)

// LoadParams reads the separate keys consensus version 4 stored params under; missing keys read as zero values.
func LoadParams(store storetypes.KVStore, cdc codec.BinaryCodec) V4Params {
	var params V4Params
	cdc.MustUnmarshal(store.Get(v0types.ParamStoreKeyChainConfig), &params.ChainConfig)
	cdc.MustUnmarshal(store.Get(v0types.ParamStoreKeyExtraEIPs), &params.ExtraEIPs)
	params.EvmDenom = string(store.Get(v0types.ParamStoreKeyEVMDenom))
	params.EnableCreate = store.Has(v0types.ParamStoreKeyEnableCreate)
	params.EnableCall = store.Has(v0types.ParamStoreKeyEnableCall)
	params.AllowUnprotectedTxs = store.Has(v0types.ParamStoreKeyAllowUnprotectedTxs)
	return params
}

func (p V4Params) ToParams() currenttypes.Params {
	chainConfig := currenttypes.ChainConfig{
		HomesteadBlock:      p.ChainConfig.HomesteadBlock,
		DAOForkBlock:        p.ChainConfig.DAOForkBlock,
		DAOForkSupport:      p.ChainConfig.DAOForkSupport,
		EIP150Block:         p.ChainConfig.EIP150Block,
		EIP150Hash:          p.ChainConfig.EIP150Hash,
		EIP155Block:         p.ChainConfig.EIP155Block,
		EIP158Block:         p.ChainConfig.EIP158Block,
		ByzantiumBlock:      p.ChainConfig.ByzantiumBlock,
		ConstantinopleBlock: p.ChainConfig.ConstantinopleBlock,
		PetersburgBlock:     p.ChainConfig.PetersburgBlock,
		IstanbulBlock:       p.ChainConfig.IstanbulBlock,
		MuirGlacierBlock:    p.ChainConfig.MuirGlacierBlock,
		BerlinBlock:         p.ChainConfig.BerlinBlock,
		LondonBlock:         p.ChainConfig.LondonBlock,
		ArrowGlacierBlock:   p.ChainConfig.ArrowGlacierBlock,
		GrayGlacierBlock:    p.ChainConfig.GrayGlacierBlock,
		MergeNetsplitBlock:  p.ChainConfig.MergeNetsplitBlock,
	}
	return currenttypes.Params{
		EvmDenom:            p.EvmDenom,
		EnableCreate:        p.EnableCreate,
		EnableCall:          p.EnableCall,
		ExtraEIPs:           p.ExtraEIPs.EIPs,
		AllowUnprotectedTxs: p.AllowUnprotectedTxs,
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

	return p.ChainConfig.Validate()
}

func validateEIPs(eips ExtraEIPs) error {
	for _, eip := range eips.EIPs {
		if !vm.ValidEip(int(eip)) {
			return fmt.Errorf("EIP %d is not activateable, valid EIPS are: %s", eip, vm.ActivateableEips())
		}
	}

	return nil
}

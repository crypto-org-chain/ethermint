package config

import (
	"github.com/cosmos/cosmos-sdk/types/module"
	"github.com/cosmos/cosmos-sdk/x/auth"
	"github.com/cosmos/cosmos-sdk/x/bank"
	distr "github.com/cosmos/cosmos-sdk/x/distribution"
	"github.com/cosmos/cosmos-sdk/x/gov"
	govclient "github.com/cosmos/cosmos-sdk/x/gov/client"
	paramsclient "github.com/cosmos/cosmos-sdk/x/params/client"
	"github.com/cosmos/cosmos-sdk/x/staking"
	"github.com/evmos/ethermint/encoding"
	"github.com/evmos/ethermint/types"
	"github.com/evmos/ethermint/x/evm"
	"github.com/evmos/ethermint/x/feemarket"
)

func MakeConfigForTest(moduleManager module.BasicManager) types.EncodingConfig {
	config := encoding.MakeConfig()
	if moduleManager == nil {
		moduleManager = module.NewBasicManager(
			auth.AppModuleBasic{},
			bank.AppModuleBasic{},
			distr.AppModuleBasic{},
			gov.NewAppModuleBasic([]govclient.ProposalHandler{paramsclient.ProposalHandler}),
			staking.AppModuleBasic{},
			// Ethermint modules
			evm.AppModuleBasic{},
			feemarket.AppModuleBasic{},
		)
	}
	moduleManager.RegisterLegacyAminoCodec(config.Amino)
	moduleManager.RegisterInterfaces(config.InterfaceRegistry)
	return config
}

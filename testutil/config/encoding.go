package config

import (
	"cosmossdk.io/x/evidence"
	feegrantmodule "cosmossdk.io/x/feegrant/module"
	"cosmossdk.io/x/upgrade"
	"github.com/cosmos/cosmos-sdk/types/module"
	"github.com/cosmos/cosmos-sdk/x/auth"
	"github.com/cosmos/cosmos-sdk/x/auth/vesting"
	authzmodule "github.com/cosmos/cosmos-sdk/x/authz/module"
	"github.com/cosmos/cosmos-sdk/x/bank"
	"github.com/cosmos/cosmos-sdk/x/consensus"
	"github.com/cosmos/cosmos-sdk/x/crisis"
	distr "github.com/cosmos/cosmos-sdk/x/distribution"
	"github.com/cosmos/cosmos-sdk/x/genutil"
	genutiltypes "github.com/cosmos/cosmos-sdk/x/genutil/types"
	"github.com/cosmos/cosmos-sdk/x/gov"
	govclient "github.com/cosmos/cosmos-sdk/x/gov/client"
	"github.com/cosmos/cosmos-sdk/x/mint"
	"github.com/cosmos/cosmos-sdk/x/params"
	paramsclient "github.com/cosmos/cosmos-sdk/x/params/client"
	"github.com/cosmos/cosmos-sdk/x/slashing"
	"github.com/cosmos/cosmos-sdk/x/staking"
	"github.com/cosmos/ibc-go/modules/capability"
	"github.com/cosmos/ibc-go/v8/modules/apps/transfer"
	ibc "github.com/cosmos/ibc-go/v8/modules/core"
	ibctm "github.com/cosmos/ibc-go/v8/modules/light-clients/07-tendermint"
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
			genutil.NewAppModuleBasic(genutiltypes.DefaultMessageValidator),
			bank.AppModuleBasic{},
			capability.AppModuleBasic{},
			staking.AppModuleBasic{},
			mint.AppModuleBasic{},
			distr.AppModuleBasic{},
			gov.NewAppModuleBasic([]govclient.ProposalHandler{paramsclient.ProposalHandler}),
			params.AppModuleBasic{},
			crisis.AppModuleBasic{},
			slashing.AppModuleBasic{},
			ibc.AppModuleBasic{},
			ibctm.AppModuleBasic{},
			authzmodule.AppModuleBasic{},
			feegrantmodule.AppModuleBasic{},
			upgrade.AppModuleBasic{},
			evidence.AppModuleBasic{},
			transfer.AppModuleBasic{},
			vesting.AppModuleBasic{},
			consensus.AppModuleBasic{},
			// Ethermint modules
			evm.AppModuleBasic{},
			feemarket.AppModuleBasic{},
		)
	}
	moduleManager.RegisterLegacyAminoCodec(config.Amino)
	moduleManager.RegisterInterfaces(config.InterfaceRegistry)
	return config
}

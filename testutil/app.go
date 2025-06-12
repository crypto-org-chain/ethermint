package testutil

import (
	"encoding/json"
	"math/rand"
	"time"

	"cosmossdk.io/log"
	abci "github.com/cometbft/cometbft/abci/types"
	cmtproto "github.com/cometbft/cometbft/proto/tendermint/types"
	tmtypes "github.com/cometbft/cometbft/types"
	dbm "github.com/cosmos/cosmos-db"
	"github.com/cosmos/cosmos-sdk/baseapp"
	"github.com/cosmos/cosmos-sdk/client/flags"
	"github.com/cosmos/cosmos-sdk/crypto/keys/ed25519"
	"github.com/cosmos/cosmos-sdk/crypto/keys/secp256k1"
	"github.com/cosmos/cosmos-sdk/server"
	simtestutil "github.com/cosmos/cosmos-sdk/testutil/sims"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/types/module"
	simtypes "github.com/cosmos/cosmos-sdk/types/simulation"
	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"
	stakingtypes "github.com/cosmos/cosmos-sdk/x/staking/types"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/evmos/ethermint/app"
	"github.com/evmos/ethermint/crypto/ethsecp256k1"
	ethermint "github.com/evmos/ethermint/types"
	evmtypes "github.com/evmos/ethermint/x/evm/types"
)

// DefaultConsensusParams defines the default Tendermint consensus params used in
// EthermintApp testing.
var DefaultConsensusParams = &cmtproto.ConsensusParams{
	Block: &cmtproto.BlockParams{
		MaxBytes: 1048576,
		MaxGas:   81500000, // default limit
	},
	Evidence: &cmtproto.EvidenceParams{
		MaxAgeNumBlocks: 302400,
		MaxAgeDuration:  504 * time.Hour, // 3 weeks is the max duration
		MaxBytes:        10000,
	},
	Validator: &cmtproto.ValidatorParams{
		PubKeyTypes: []string{
			tmtypes.ABCIPubKeyTypeEd25519,
		},
	},
}

// Setup initializes a new EthermintApp. A Nop logger is set in EthermintApp.
func Setup(isCheckTx bool, patch func(*app.EthermintApp, app.GenesisState) app.GenesisState) *app.EthermintApp {
	return SetupWithDB(isCheckTx, patch, dbm.NewMemDB())
}

func SetupWithOpts(
	isCheckTx bool,
	patch func(*app.EthermintApp, app.GenesisState) app.GenesisState,
	appOptions simtestutil.AppOptionsMap,
) *app.EthermintApp {
	return SetupWithDBAndOpts(isCheckTx, patch, dbm.NewMemDB(), appOptions)
}

const ChainID = "ethermint_9000-1"

func SetupWithDB(isCheckTx bool, patch func(*app.EthermintApp, app.GenesisState) app.GenesisState, db dbm.DB) *app.EthermintApp {
	return SetupWithDBAndOpts(isCheckTx, patch, db, nil)
}

// SetupWithDBAndOpts initializes a new EthermintApp. A Nop logger is set in EthermintApp.
func SetupWithDBAndOpts(
	isCheckTx bool,
	patch func(*app.EthermintApp, app.GenesisState) app.GenesisState,
	db dbm.DB,
	appOptions simtestutil.AppOptionsMap,
) *app.EthermintApp {
	if appOptions == nil {
		appOptions = make(simtestutil.AppOptionsMap, 0)
	}
	appOptions[server.FlagInvCheckPeriod] = 5
	appOptions[flags.FlagHome] = app.DefaultNodeHome
	app := app.NewEthermintApp(log.NewNopLogger(),
		db,
		nil,
		true,
		appOptions,
		baseapp.SetChainID(ChainID),
	)

	if !isCheckTx {
		// init chain must be called to stop deliverState from being nil
		genesisState := NewTestGenesisState(app.AppCodec(), app.DefaultGenesis())
		if patch != nil {
			genesisState = patch(app, genesisState)
		}

		stateBytes, err := json.MarshalIndent(genesisState, "", " ")
		if err != nil {
			panic(err)
		}

		// Initialize the chain
		consensusParams := DefaultConsensusParams
		initialHeight := app.LastBlockHeight() + 1
		consensusParams.Abci = &cmtproto.ABCIParams{VoteExtensionsEnableHeight: initialHeight}
		if _, err := app.InitChain(
			&abci.RequestInitChain{
				ChainId:         ChainID,
				Validators:      []abci.ValidatorUpdate{},
				ConsensusParams: consensusParams,
				AppStateBytes:   stateBytes,
				InitialHeight:   initialHeight,
			},
		); err != nil {
			panic(err)
		}
	}
	if _, err := app.FinalizeBlock(&abci.RequestFinalizeBlock{
		Height: app.LastBlockHeight() + 1,
		Hash:   app.LastCommitID().Hash,
	}); err != nil {
		panic(err)
	}
	return app
}

// RandomGenesisAccounts is used by the auth module to create random genesis accounts in simulation when a genesis.json is not specified.
// In contrast, the default auth module's RandomGenesisAccounts implementation creates only base accounts and vestings accounts.
func RandomGenesisAccounts(simState *module.SimulationState) authtypes.GenesisAccounts {
	emptyCodeHash := crypto.Keccak256(nil)
	genesisAccs := make(authtypes.GenesisAccounts, len(simState.Accounts))
	for i, acc := range simState.Accounts {
		bacc := authtypes.NewBaseAccountWithAddress(acc.Address)

		ethacc := &ethermint.EthAccount{
			BaseAccount: bacc,
			CodeHash:    common.BytesToHash(emptyCodeHash).String(),
		}
		genesisAccs[i] = ethacc
	}

	return genesisAccs
}

// RandomAccounts creates random accounts with an ethsecp256k1 private key
// TODO: replace secp256k1.GenPrivKeyFromSecret() with similar function in go-ethereum
func RandomAccounts(r *rand.Rand, n int) []simtypes.Account {
	accs := make([]simtypes.Account, n)

	for i := 0; i < n; i++ {
		// don't need that much entropy for simulation
		privkeySeed := make([]byte, 15)
		_, _ = r.Read(privkeySeed)

		prv := secp256k1.GenPrivKeyFromSecret(privkeySeed)
		ethPrv := &ethsecp256k1.PrivKey{}
		_ = ethPrv.UnmarshalAmino(prv.Bytes()) // UnmarshalAmino simply copies the bytes and assigns them to ethPrv.Key
		accs[i].PrivKey = ethPrv
		accs[i].PubKey = accs[i].PrivKey.PubKey()
		accs[i].Address = sdk.AccAddress(accs[i].PubKey.Address())

		accs[i].ConsKey = ed25519.GenPrivKeyFromSecret(privkeySeed)
	}

	return accs
}

// StateFn returns the initial application state using a genesis or the simulation parameters.
// It is a wrapper of simapp.AppStateFn to replace evm param EvmDenom with staking param BondDenom.
func StateFn(a *app.EthermintApp) simtypes.AppStateFn {
	var bondDenom string
	return simtestutil.AppStateFnWithExtendedCbs(
		a.AppCodec(),
		a.SimulationManager(),
		a.DefaultGenesis(),
		func(moduleName string, genesisState interface{}) {
			if moduleName == stakingtypes.ModuleName {
				stakingState := genesisState.(*stakingtypes.GenesisState)
				bondDenom = stakingState.Params.BondDenom
			}
		},
		func(rawState map[string]json.RawMessage) {
			evmStateBz, ok := rawState[evmtypes.ModuleName]
			if !ok {
				panic("evm genesis state is missing")
			}

			evmState := new(evmtypes.GenesisState)
			a.AppCodec().MustUnmarshalJSON(evmStateBz, evmState)

			// we should replace the EvmDenom with BondDenom
			evmState.Params.EvmDenom = bondDenom

			// change appState back
			rawState[evmtypes.ModuleName] = a.AppCodec().MustMarshalJSON(evmState)
		},
	)
}

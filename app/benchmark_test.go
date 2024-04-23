package app_test

import (
	"encoding/json"
	"io"
	"testing"

	"cosmossdk.io/log"
	abci "github.com/cometbft/cometbft/abci/types"
	dbm "github.com/cosmos/cosmos-db"
	"github.com/cosmos/cosmos-sdk/baseapp"
	simtestutil "github.com/cosmos/cosmos-sdk/testutil/sims"
	"github.com/evmos/ethermint/app"
	"github.com/evmos/ethermint/testutil"
)

func BenchmarkEthermintApp_ExportAppStateAndValidators(b *testing.B) {
	db := dbm.NewMemDB()
	app1 := app.NewEthermintApp(
		log.NewLogger(io.Discard),
		db,
		nil,
		true,
		simtestutil.NewAppOptionsWithFlagHome(app.DefaultNodeHome),
		baseapp.SetChainID(testutil.ChainID),
	)

	genesisState := testutil.NewTestGenesisState(app1.AppCodec(), app1.DefaultGenesis())
	stateBytes, err := json.MarshalIndent(genesisState, "", "  ")
	if err != nil {
		b.Fatal(err)
	}

	// Initialize the chain
	app1.InitChain(
		&abci.RequestInitChain{
			ChainId:       testutil.ChainID,
			Validators:    []abci.ValidatorUpdate{},
			AppStateBytes: stateBytes,
		},
	)
	app1.Commit()

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		// Making a new app object with the db, so that initchain hasn't been called
		app2 := app.NewEthermintApp(
			log.NewLogger(io.Discard),
			db,
			nil,
			true,
			simtestutil.NewAppOptionsWithFlagHome(app.DefaultNodeHome),
			baseapp.SetChainID(testutil.ChainID),
		)
		if _, err := app2.ExportAppStateAndValidators(false, []string{}, []string{}); err != nil {
			b.Fatal(err)
		}
	}
}

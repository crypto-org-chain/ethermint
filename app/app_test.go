package app_test

import (
	"os"
	"testing"

	"github.com/evmos/ethermint/app"
	"github.com/evmos/ethermint/testutil"
	"github.com/stretchr/testify/require"

	"cosmossdk.io/log"
	dbm "github.com/cosmos/cosmos-db"

	"github.com/cosmos/cosmos-sdk/baseapp"
	simtestutil "github.com/cosmos/cosmos-sdk/testutil/sims"
)

func TestEthermintAppExport(t *testing.T) {
	db := dbm.NewMemDB()
	ethApp := testutil.SetupWithDB(false, nil, db)
	ethApp.Commit()

	// Making a new app object with the db, so that initchain hasn't been called
	ethApp2 := app.NewEthermintApp(
		log.NewLogger(os.Stdout),
		db,
		nil,
		true,
		simtestutil.NewAppOptionsWithFlagHome(app.DefaultNodeHome),
		baseapp.SetChainID(testutil.ChainID),
	)
	_, err := ethApp2.ExportAppStateAndValidators(false, []string{}, []string{})
	require.NoError(t, err, "ExportAppStateAndValidators should not have an error")
}

package evmd_test

import (
	"os"
	"testing"

	"github.com/cosmos/cosmos-sdk/client"
	"github.com/evmos/ethermint/encoding"
	"github.com/evmos/ethermint/evmd"
	"github.com/evmos/ethermint/testutil"
	"github.com/stretchr/testify/require"

	"cosmossdk.io/log/v2"
	dbm "github.com/cosmos/cosmos-db"

	"github.com/cosmos/cosmos-sdk/baseapp"
	simtestutil "github.com/cosmos/cosmos-sdk/testutil/sims"
)

func TestEthermintAppExport(t *testing.T) {
	db := dbm.NewMemDB()
	ethApp := testutil.SetupWithDB(false, nil, db)
	ethApp.Commit()

	// Making a new app object with the db, so that initchain hasn't been called
	ethApp2 := evmd.NewEthermintApp(
		log.NewLogger(os.Stdout),
		db,
		true,
		simtestutil.NewAppOptionsWithFlagHome(evmd.DefaultNodeHome),
		baseapp.SetChainID(testutil.ChainID),
	)
	_, err := ethApp2.ExportAppStateAndValidators(false, []string{}, []string{})
	require.NoError(t, err, "ExportAppStateAndValidators should not have an error")

	ethApp3 := evmd.NewEthermintApp(
		log.NewLogger(os.Stdout),
		db,
		true,
		simtestutil.NewAppOptionsWithFlagHome(evmd.DefaultNodeHome),
		baseapp.SetChainID(testutil.ChainID),
	)

	// Test for zero height
	if _, err := ethApp3.ExportAppStateAndValidators(true, []string{}, []string{}); err != nil {
		t.Fatal(err)
	}
}

func TestRegisterTxService(t *testing.T) {
	db := dbm.NewMemDB()
	ethApp := testutil.SetupWithDB(false, nil, db)

	encodingConfig := encoding.MakeConfig()
	clientCtx := client.Context{}.WithTxConfig(encodingConfig.TxConfig)

	ethApp.RegisterTxService(clientCtx)

	ethApp.RegisterTendermintService(clientCtx)

}

func TestRegisterTendermintService(t *testing.T) {
	db := dbm.NewMemDB()
	ethApp := testutil.SetupWithDB(false, nil, db)

	encodingConfig := encoding.MakeConfig()
	clientCtx := client.Context{}.WithTxConfig(encodingConfig.TxConfig)

	ethApp.RegisterTendermintService(clientCtx)

}

package evmd_test

import (
	"os"
	"testing"
	"time"

	abci "github.com/cometbft/cometbft/abci/types"
	cryptoenc "github.com/cometbft/cometbft/crypto/encoding"
	tmproto "github.com/cometbft/cometbft/proto/tendermint/types"
	"github.com/cosmos/cosmos-sdk/client"
	"github.com/evmos/ethermint/encoding"
	"github.com/evmos/ethermint/evmd"
	"github.com/evmos/ethermint/testutil"
	"github.com/stretchr/testify/require"

	"cosmossdk.io/log/v2"
	dbm "github.com/cosmos/cosmos-db"

	"github.com/cosmos/cosmos-sdk/baseapp"
	"github.com/cosmos/cosmos-sdk/crypto/keys/ed25519"
	cryptotypes "github.com/cosmos/cosmos-sdk/crypto/types"
	simtestutil "github.com/cosmos/cosmos-sdk/testutil/sims"
	sdk "github.com/cosmos/cosmos-sdk/types"
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

// setupWithPendingRotation schedules a consensus key rotation for the only validator on a committed chain.
func setupWithPendingRotation(t *testing.T, db dbm.DB) (*evmd.EthermintApp, time.Time, cryptotypes.PubKey, cryptotypes.PubKey) {
	app := testutil.SetupWithDB(false, nil, db)
	_, err := app.Commit()
	require.NoError(t, err)

	blockTime := time.Now().UTC()
	ctx := app.NewUncachedContext(false, tmproto.Header{Height: app.LastBlockHeight(), Time: blockTime}) //nolint:staticcheck
	vals, err := app.StakingKeeper.GetAllValidators(ctx)
	require.NoError(t, err)
	require.Len(t, vals, 1)
	valAddr, err := sdk.ValAddressFromBech32(vals[0].GetOperator())
	require.NoError(t, err)
	oldPubKey, err := vals[0].ConsPubKey()
	require.NoError(t, err)
	newPubKey := ed25519.GenPrivKey().PubKey()
	require.NoError(t, app.StakingKeeper.SetConsKeyRotation(ctx, valAddr, oldPubKey, newPubKey))
	return app, blockTime, oldPubKey, newPubKey
}

func finalizeAndCommit(t *testing.T, app *evmd.EthermintApp, blockTime time.Time, proposer cryptotypes.PubKey) {
	_, err := app.FinalizeBlock(&abci.RequestFinalizeBlock{
		Height:          app.LastBlockHeight() + 1,
		Hash:            app.LastCommitID().Hash,
		Time:            blockTime,
		ProposerAddress: proposer.Address(),
	})
	require.NoError(t, err)
	_, err = app.Commit()
	require.NoError(t, err)
}

func TestRotatedInProposer(t *testing.T) {
	app, blockTime, oldPubKey, newPubKey := setupWithPendingRotation(t, dbm.NewMemDB())

	// CometBFT switches to the new key one block before staking indexes it, and the evm
	// BeginBlock must still resolve the coinbase for that block.
	finalizeAndCommit(t, app, blockTime, oldPubKey)
	finalizeAndCommit(t, app, blockTime, newPubKey)
	finalizeAndCommit(t, app, blockTime, newPubKey)
}

func TestExportZeroHeightWithPendingRotation(t *testing.T) {
	newApp := func(db dbm.DB) *evmd.EthermintApp {
		return evmd.NewEthermintApp(
			log.NewNopLogger(),
			db,
			true,
			simtestutil.NewAppOptionsWithFlagHome(evmd.DefaultNodeHome),
			baseapp.SetChainID(testutil.ChainID),
		)
	}

	db := dbm.NewMemDB()
	app, blockTime, oldPubKey, _ := setupWithPendingRotation(t, db)
	// The rotation applies two blocks after it is scheduled, so one more block leaves it pending.
	finalizeAndCommit(t, app, blockTime, oldPubKey)

	exportApp := newApp(db)
	pending, err := exportApp.StakingKeeper.PendingConsKeyRotations(
		exportApp.NewUncachedContext(false, tmproto.Header{Height: exportApp.LastBlockHeight()}), //nolint:staticcheck
	)
	require.NoError(t, err)
	require.Len(t, pending, 1, "the rotation must still be pending at export")

	exported, err := exportApp.ExportAppStateAndValidators(true, []string{}, []string{})
	require.NoError(t, err)

	validators := make([]abci.ValidatorUpdate, len(exported.Validators))
	for i, val := range exported.Validators {
		pubKey, err := cryptoenc.PubKeyToProto(val.PubKey)
		require.NoError(t, err)
		validators[i] = abci.ValidatorUpdate{PubKey: pubKey, Power: val.Power}
	}

	// InitChain rejects a genesis whose validators differ from the ones staking returns.
	_, err = newApp(dbm.NewMemDB()).InitChain(&abci.RequestInitChain{
		ChainId:         testutil.ChainID,
		Time:            blockTime,
		Validators:      validators,
		ConsensusParams: testutil.DefaultConsensusParams,
		AppStateBytes:   exported.AppState,
		InitialHeight:   1,
	})
	require.NoError(t, err)
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

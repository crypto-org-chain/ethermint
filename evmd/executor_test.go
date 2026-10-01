package evmd_test

import (
	"testing"

	dbm "github.com/cosmos/cosmos-db"
	"github.com/stretchr/testify/require"

	"cosmossdk.io/log/v2"

	"github.com/cosmos/cosmos-sdk/baseapp"
	"github.com/cosmos/cosmos-sdk/baseapp/blockexec"
	"github.com/cosmos/cosmos-sdk/server"
	"github.com/cosmos/cosmos-sdk/server/config"
	storetypes "github.com/cosmos/cosmos-sdk/store/v2/types"
	simtestutil "github.com/cosmos/cosmos-sdk/testutil/sims"
	sdk "github.com/cosmos/cosmos-sdk/types"

	"github.com/evmos/ethermint/evmd"
)

func TestPatchedTxRunnerExposesParallelRunner(t *testing.T) {
	for _, tc := range []struct {
		executor string
		parallel bool
	}{
		{config.BlockExecutorBlockSTM, true},
		{config.BlockExecutorSequential, false},
	} {
		t.Run(tc.executor, func(t *testing.T) {
			bApp := baseapp.NewBaseApp("executor-test", log.NewNopLogger(), dbm.NewMemDB(), nil)
			blockexec.Apply(
				bApp,
				simtestutil.AppOptionsMap{server.FlagBlockExecutor: tc.executor},
				nil, nil,
				func(storetypes.MultiStore) string { return sdk.DefaultBondDenom },
				blockexec.WithRunnerWrap(func(inner sdk.TxRunner) sdk.TxRunner { return evmd.NewPatchedTxRunner(inner) }),
			)

			enableBlockGasMeter := func() { bApp.SetDisableBlockGasMeter(false) }
			if tc.parallel {
				require.Panics(t, enableBlockGasMeter)
			} else {
				require.NotPanics(t, enableBlockGasMeter)
			}
		})
	}
}

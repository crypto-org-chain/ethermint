package evmd_test

import (
	"testing"
	"time"

	tmproto "github.com/cometbft/cometbft/proto/tendermint/types"
	"github.com/stretchr/testify/require"

	sdkmath "cosmossdk.io/math"
	sdk "github.com/cosmos/cosmos-sdk/types"
	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"
	stakingtypes "github.com/cosmos/cosmos-sdk/x/staking/types"
	upgradetypes "github.com/cosmos/cosmos-sdk/x/upgrade/types"

	"github.com/evmos/ethermint/evmd"
	"github.com/evmos/ethermint/testutil"
	evmtypes "github.com/evmos/ethermint/x/evm/types"
)

func TestEnsureModuleAccount(t *testing.T) {
	name := stakingtypes.KeyRotationFeePoolName
	poolAddr := authtypes.NewModuleAddress(name)

	// Genesis creates the pool; an upgraded chain starts without it.
	removePool := func(app *evmd.EthermintApp, ctx sdk.Context) {
		app.AccountKeeper.RemoveAccount(ctx, app.AccountKeeper.GetAccount(ctx, poolAddr))
	}

	testCases := []struct {
		name     string
		malleate func(t *testing.T, app *evmd.EthermintApp, ctx sdk.Context) sdk.Coins
	}{
		{
			name: "no account yet",
			malleate: func(_ *testing.T, app *evmd.EthermintApp, ctx sdk.Context) sdk.Coins {
				removePool(app, ctx)
				return sdk.NewCoins()
			},
		},
		{
			// Anyone can create a plain account here before the upgrade, e.g. by an EVM transfer.
			name: "plain account squatting the address",
			malleate: func(t *testing.T, app *evmd.EthermintApp, ctx sdk.Context) sdk.Coins {
				removePool(app, ctx)
				coins := sdk.NewCoins(sdk.NewCoin(evmtypes.DefaultEVMDenom, sdkmath.NewInt(7)))
				require.NoError(t, app.BankKeeper.MintCoins(ctx, evmtypes.ModuleName, coins))
				require.NoError(t, app.BankKeeper.SendCoins(ctx, authtypes.NewModuleAddress(evmtypes.ModuleName), poolAddr, coins))
				_, isModule := app.AccountKeeper.GetAccount(ctx, poolAddr).(sdk.ModuleAccountI)
				require.False(t, isModule)
				return coins
			},
		},
		{
			name: "module account already exists",
			malleate: func(*testing.T, *evmd.EthermintApp, sdk.Context) sdk.Coins {
				return sdk.NewCoins()
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			app := testutil.Setup(false, nil)
			ctx := app.NewUncachedContext(false, tmproto.Header{}) //nolint:staticcheck
			balance := tc.malleate(t, app, ctx)
			var accNum uint64
			if acc := app.AccountKeeper.GetAccount(ctx, poolAddr); acc != nil {
				accNum = acc.GetAccountNumber()
			}

			evmd.EnsureModuleAccount(ctx, app.AccountKeeper, name)

			acc := app.AccountKeeper.GetAccount(ctx, poolAddr)
			macc, ok := acc.(sdk.ModuleAccountI)
			require.True(t, ok, "must be a module account")
			require.Equal(t, name, macc.GetName())
			if accNum != 0 {
				require.Equal(t, accNum, acc.GetAccountNumber(), "account number must be kept")
			}
			require.Equal(t, balance.String(), app.BankKeeper.GetAllBalances(ctx, poolAddr).String(), "balance must be kept")
			require.NoError(t, app.BankKeeper.BurnCoins(ctx, name, balance), "the pool must accept module transfers")
		})
	}
}

func TestSDK55UpgradeMaturesUnindexedUnbonding(t *testing.T) {
	app := testutil.Setup(false, nil)
	ctx := app.NewUncachedContext(false, tmproto.Header{Height: 5, Time: time.Now().UTC()}) //nolint:staticcheck
	sk := app.StakingKeeper

	vals, err := sk.GetAllValidators(ctx)
	require.NoError(t, err)
	valAddr, err := sdk.ValAddressFromBech32(vals[0].GetOperator())
	require.NoError(t, err)
	dels, err := sk.GetValidatorDelegations(ctx, valAddr)
	require.NoError(t, err)
	delAddr := sdk.MustAccAddressFromBech32(dels[0].DelegatorAddress)

	completion, _, err := sk.Undelegate(ctx, delAddr, valAddr, sdkmath.LegacyNewDecWithPrec(5, 1))
	require.NoError(t, err)

	// Mirror state written by a pre-index SDK binary: the queue entry without its index.
	store := ctx.KVStore(app.GetKey(stakingtypes.StoreKey))
	store.Delete(stakingtypes.UBDQueuePendingSlotsKey)
	store.Delete(stakingtypes.RedelegationQueuePendingSlotsKey)
	store.Delete(stakingtypes.ValidatorQueuePendingSlotsKey)

	mature := ctx.WithBlockTime(completion.Add(time.Second)).WithBlockHeight(6)
	cacheCtx, _ := mature.CacheContext()
	_, err = sk.EndBlocker(cacheCtx)
	require.NoError(t, err)
	_, err = sk.GetUnbondingDelegation(cacheCtx, delAddr, valAddr)
	require.NoError(t, err, "without the index the unbonding must stay pending")

	plan := upgradetypes.Plan{Name: "sdk55", Height: 5}
	require.NoError(t, app.UpgradeKeeper.ScheduleUpgrade(ctx, plan))
	require.NoError(t, app.UpgradeKeeper.ApplyUpgrade(ctx, plan))

	_, err = sk.EndBlocker(mature)
	require.NoError(t, err)
	_, err = sk.GetUnbondingDelegation(mature, delAddr, valAddr)
	require.ErrorIs(t, err, stakingtypes.ErrNoUnbondingDelegation, "the upgrade must index the unbonding so it matures")
}

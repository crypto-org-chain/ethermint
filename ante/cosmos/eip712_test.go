package cosmos_test

import (
	"math/big"
	"testing"

	sdkmath "cosmossdk.io/math"
	abci "github.com/cometbft/cometbft/abci/types"
	tmproto "github.com/cometbft/cometbft/proto/tendermint/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	stakingtypes "github.com/cosmos/cosmos-sdk/x/staking/types"
	"github.com/evmos/ethermint/crypto/ethsecp256k1"
	"github.com/evmos/ethermint/testutil"
	utiltx "github.com/evmos/ethermint/testutil/tx"
	"github.com/stretchr/testify/require"
)

func TestLegacyEIP712MixedMsg(t *testing.T) {
	app := testutil.Setup(false, nil)
	ctx := app.BaseApp.NewUncachedContext(false, tmproto.Header{ChainID: testutil.ChainID})
	app.FeeMarketKeeper.SetBaseFee(ctx, big.NewInt(1))

	privKey, err := ethsecp256k1.GenerateKey()
	require.NoError(t, err)
	delegator := sdk.AccAddress(privKey.PubKey().Address().Bytes())

	acc := app.AccountKeeper.NewAccountWithAddress(ctx, delegator)
	app.AccountKeeper.SetAccount(ctx, acc)

	bondDenom, err := app.StakingKeeper.BondDenom(ctx)
	require.NoError(t, err)
	evmDenom := app.EvmKeeper.GetParams(ctx).EvmDenom
	gas := uint64(500000)
	delegationAmount := sdk.NewCoin(bondDenom, sdkmath.NewInt(100))
	feeAmount := sdk.NewCoins(sdk.NewCoin(evmDenom, sdkmath.NewInt(100*int64(gas))))

	require.NoError(t, testutil.FundAccount(
		app.BankKeeper,
		ctx,
		delegator,
		sdk.NewCoins(
			sdk.NewCoin(bondDenom, delegationAmount.Amount.MulRaw(10)),
			sdk.NewCoin(evmDenom, feeAmount.AmountOf(evmDenom).MulRaw(2)),
		),
	))

	var valAddr sdk.ValAddress
	err = app.StakingKeeper.IterateValidators(ctx, func(_ int64, val stakingtypes.ValidatorI) bool {
		bz, err := app.StakingKeeper.ValidatorAddressCodec().StringToBytes(val.GetOperator())
		require.NoError(t, err)
		valAddr = sdk.ValAddress(bz)
		return true
	})
	require.NoError(t, err)
	require.NotEmpty(t, valAddr)

	msgs := []sdk.Msg{
		stakingtypes.NewMsgDelegate(delegator.String(), valAddr.String(), delegationAmount),
		stakingtypes.NewMsgUndelegate(delegator.String(), valAddr.String(), delegationAmount),
	}

	txArgs := utiltx.EIP712TxArgs{
		CosmosTxArgs: utiltx.CosmosTxArgs{
			TxCfg:   app.TxConfig(),
			Priv:    privKey,
			ChainID: testutil.ChainID,
			Gas:     gas,
			Fees:    feeAmount,
			Msgs:    msgs,
		},
		UseLegacyExtension: true,
		UseLegacyTypedData: true,
	}

	tx, err := utiltx.CreateEIP712CosmosTx(ctx, app, txArgs)
	require.NoError(t, err)

	txBytes, err := app.TxConfig().TxEncoder()(tx)
	require.NoError(t, err)
	height := app.LastBlockHeight() + 1
	res, err := app.FinalizeBlock(&abci.RequestFinalizeBlock{
		Height: height,
		Txs:    [][]byte{txBytes},
	})
	require.NoError(t, err)
	require.Len(t, res.TxResults, 1)

	require.NotZero(t, res.TxResults[0].Code, "expected tx to fail with mixed message types")
	require.Contains(t, res.TxResults[0].Log, "different types of messages detected",
		"expected error about different message types")
}

// TestLegacyEIP712SameMsgType tests that a legacy EIP-712 transaction with
// multiple messages of the same type succeeds on-chain.
func TestLegacyEIP712SameMsgType(t *testing.T) {
	app := testutil.Setup(false, nil)
	ctx := app.BaseApp.NewUncachedContext(false, tmproto.Header{ChainID: testutil.ChainID})
	app.FeeMarketKeeper.SetBaseFee(ctx, big.NewInt(1))

	privKey, err := ethsecp256k1.GenerateKey()
	require.NoError(t, err)
	delegator := sdk.AccAddress(privKey.PubKey().Address().Bytes())

	acc := app.AccountKeeper.NewAccountWithAddress(ctx, delegator)
	app.AccountKeeper.SetAccount(ctx, acc)

	bondDenom, err := app.StakingKeeper.BondDenom(ctx)
	require.NoError(t, err)
	evmDenom := app.EvmKeeper.GetParams(ctx).EvmDenom
	gas := uint64(500000)
	delegationAmount := sdk.NewCoin(bondDenom, sdkmath.NewInt(100))
	feeAmount := sdk.NewCoins(sdk.NewCoin(evmDenom, sdkmath.NewInt(100*int64(gas))))

	require.NoError(t, testutil.FundAccount(
		app.BankKeeper,
		ctx,
		delegator,
		sdk.NewCoins(
			sdk.NewCoin(bondDenom, delegationAmount.Amount.MulRaw(10)),
			sdk.NewCoin(evmDenom, feeAmount.AmountOf(evmDenom).MulRaw(2)),
		),
	))

	var valAddr sdk.ValAddress
	err = app.StakingKeeper.IterateValidators(ctx, func(_ int64, val stakingtypes.ValidatorI) bool {
		bz, err := app.StakingKeeper.ValidatorAddressCodec().StringToBytes(val.GetOperator())
		require.NoError(t, err)
		valAddr = sdk.ValAddress(bz)
		return true
	})
	require.NoError(t, err)
	require.NotEmpty(t, valAddr)

	_, err = app.StakingKeeper.GetDelegation(ctx, delegator, valAddr)
	require.Error(t, err, "delegation should not exist before transaction")

	msgs := []sdk.Msg{
		stakingtypes.NewMsgDelegate(delegator.String(), valAddr.String(), delegationAmount),
		stakingtypes.NewMsgDelegate(delegator.String(), valAddr.String(), delegationAmount),
	}

	txArgs := utiltx.EIP712TxArgs{
		CosmosTxArgs: utiltx.CosmosTxArgs{
			TxCfg:   app.TxConfig(),
			Priv:    privKey,
			ChainID: testutil.ChainID,
			Gas:     gas,
			Fees:    feeAmount,
			Msgs:    msgs,
		},
		UseLegacyExtension: true,
		UseLegacyTypedData: true,
	}

	tx, err := utiltx.CreateEIP712CosmosTx(ctx, app, txArgs)
	require.NoError(t, err)

	txBytes, err := app.TxConfig().TxEncoder()(tx)
	require.NoError(t, err)
	height := app.LastBlockHeight() + 1
	res, err := app.FinalizeBlock(&abci.RequestFinalizeBlock{
		Height: height,
		Txs:    [][]byte{txBytes},
	})
	require.NoError(t, err)
	require.Len(t, res.TxResults, 1)
	require.Zero(t, res.TxResults[0].Code, "expected tx to succeed with same message types")

	_, err = app.Commit()
	require.NoError(t, err)

	queryCtx := app.NewUncachedContext(false, tmproto.Header{ChainID: testutil.ChainID, Height: height + 1})
	delegation, err := app.StakingKeeper.GetDelegation(queryCtx, delegator, valAddr)
	require.NoError(t, err)

	require.True(t, delegation.Shares.IsPositive(),
		"expected positive delegation shares, got %s", delegation.Shares)

	validator, err := app.StakingKeeper.GetValidator(queryCtx, valAddr)
	require.NoError(t, err)
	expectedShares, err := validator.SharesFromTokens(delegationAmount.Amount.MulRaw(2))
	require.NoError(t, err)
	require.True(t, delegation.Shares.Equal(expectedShares),
		"expected delegation shares %s, got %s", expectedShares, delegation.Shares)
}

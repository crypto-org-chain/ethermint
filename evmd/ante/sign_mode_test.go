package ante_test

import (
	"math/big"
	"testing"

	sdkmath "cosmossdk.io/math"
	abci "github.com/cometbft/cometbft/abci/types"
	tmproto "github.com/cometbft/cometbft/proto/tendermint/types"
	clienttx "github.com/cosmos/cosmos-sdk/client/tx"
	kmultisig "github.com/cosmos/cosmos-sdk/crypto/keys/multisig"
	"github.com/cosmos/cosmos-sdk/crypto/keys/secp256k1"
	cryptotypes "github.com/cosmos/cosmos-sdk/crypto/types"
	storetypes "github.com/cosmos/cosmos-sdk/store/v2/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	errortypes "github.com/cosmos/cosmos-sdk/types/errors"
	"github.com/cosmos/cosmos-sdk/types/tx/signing"
	authsigning "github.com/cosmos/cosmos-sdk/x/auth/signing"
	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"
	banktypes "github.com/cosmos/cosmos-sdk/x/bank/types"
	"github.com/stretchr/testify/require"

	"github.com/evmos/ethermint/crypto/ethsecp256k1"
	"github.com/evmos/ethermint/evmd/ante"
	"github.com/evmos/ethermint/testutil"
)

func TestSigVerificationGasConsumerSignModes(t *testing.T) {
	ethKey := func(t *testing.T) cryptotypes.PubKey {
		priv, err := ethsecp256k1.GenerateKey()
		require.NoError(t, err)
		return priv.PubKey()
	}
	cosmosKey := func(*testing.T) cryptotypes.PubKey {
		return secp256k1.GenPrivKey().PubKey()
	}
	single := func(mode signing.SignMode) signing.SignatureData {
		return &signing.SingleSignatureData{SignMode: mode, Signature: []byte{1}}
	}
	// multi builds multisig data where modes[i] is the sign mode of member i,
	// and an unspecified mode means member i did not sign.
	multi := func(modes ...signing.SignMode) *signing.MultiSignatureData {
		bits := cryptotypes.NewCompactBitArray(len(modes))
		sigs := make([]signing.SignatureData, 0, len(modes))
		for i, m := range modes {
			if m == signing.SignMode_SIGN_MODE_UNSPECIFIED {
				continue
			}
			bits.SetIndex(i, true)
			sigs = append(sigs, single(m))
		}
		return &signing.MultiSignatureData{BitArray: bits, Signatures: sigs}
	}
	mixedMultisig := func(t *testing.T) cryptotypes.PubKey {
		return kmultisig.NewLegacyAminoPubKey(1, []cryptotypes.PubKey{cosmosKey(t), ethKey(t)})
	}
	const (
		none   = signing.SignMode_SIGN_MODE_UNSPECIFIED
		direct = signing.SignMode_SIGN_MODE_DIRECT
		amino  = signing.SignMode_SIGN_MODE_LEGACY_AMINO_JSON
		aux    = signing.SignMode_SIGN_MODE_DIRECT_AUX
	)

	testCases := []struct {
		name    string
		pubKey  func(*testing.T) cryptotypes.PubKey
		sigData signing.SignatureData
		expErr  bool
	}{
		{name: "eth key, direct", pubKey: ethKey, sigData: single(direct)},
		{name: "eth key, legacy amino json", pubKey: ethKey, sigData: single(amino)},
		{name: "eth key, unspecified (simulation)", pubKey: ethKey, sigData: single(none)},
		{name: "eth key, direct aux", pubKey: ethKey, sigData: single(aux), expErr: true},
		{name: "eth key, multisig data", pubKey: ethKey, sigData: multi(direct), expErr: true},
		{name: "secp256k1 key, direct aux", pubKey: cosmosKey, sigData: single(aux)},
		{name: "multisig, eth member signs direct aux", pubKey: mixedMultisig, sigData: multi(none, aux), expErr: true},
		{name: "multisig, eth member amino, cosmos member direct aux", pubKey: mixedMultisig, sigData: multi(aux, amino)},
		{name: "multisig, only cosmos member signs direct aux", pubKey: mixedMultisig, sigData: multi(aux, none)},
		{
			name: "nested multisig, eth member signs direct aux",
			pubKey: func(t *testing.T) cryptotypes.PubKey {
				return kmultisig.NewLegacyAminoPubKey(1, []cryptotypes.PubKey{cosmosKey(t), mixedMultisig(t)})
			},
			sigData: func() signing.SignatureData {
				bits := cryptotypes.NewCompactBitArray(2)
				bits.SetIndex(1, true)
				return &signing.MultiSignatureData{BitArray: bits, Signatures: []signing.SignatureData{multi(none, aux)}}
			}(),
			expErr: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := ante.DefaultSigVerificationGasConsumer(
				storetypes.NewInfiniteGasMeter(),
				signing.SignatureV2{PubKey: tc.pubKey(t), Data: tc.sigData},
				authtypes.DefaultParams(),
			)
			if tc.expErr {
				require.ErrorIs(t, err, errortypes.ErrNotSupported)
				return
			}
			require.NoError(t, err)
		})
	}
}

func TestEthSecp256k1SignModeAnteHandler(t *testing.T) {
	testCases := []struct {
		name     string
		signMode signing.SignMode
		// labelMode overrides the sign mode recorded in the tx after signing.
		labelMode *signing.SignMode
		expCode   uint32
		expLog    string
	}{
		{name: "direct", signMode: signing.SignMode_SIGN_MODE_DIRECT},
		{
			// A fee payer cannot sign DIRECT_AUX, so relabel a DIRECT signature to reach the ante check.
			name:      "direct signature labelled direct aux",
			signMode:  signing.SignMode_SIGN_MODE_DIRECT,
			labelMode: func() *signing.SignMode { m := signing.SignMode_SIGN_MODE_DIRECT_AUX; return &m }(),
			expCode:   errortypes.ErrNotSupported.ABCICode(),
			expLog:    "is not supported for eth_secp256k1 keys",
		},
		{
			name:      "direct signature labelled unspecified",
			signMode:  signing.SignMode_SIGN_MODE_DIRECT,
			labelMode: func() *signing.SignMode { m := signing.SignMode_SIGN_MODE_UNSPECIFIED; return &m }(),
			expCode:   errortypes.ErrUnauthorized.ABCICode(),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			app := testutil.Setup(false, nil)
			ctx := app.BaseApp.NewUncachedContext(false, tmproto.Header{ChainID: testutil.ChainID})
			app.FeeMarketKeeper.SetBaseFee(ctx, big.NewInt(1))

			priv, err := ethsecp256k1.GenerateKey()
			require.NoError(t, err)
			signer := sdk.AccAddress(priv.PubKey().Address())
			app.AccountKeeper.SetAccount(ctx, app.AccountKeeper.NewAccountWithAddress(ctx, signer))

			evmDenom := app.EvmKeeper.GetParams(ctx).EvmDenom
			gas := uint64(200000)
			fees := sdk.NewCoins(sdk.NewCoin(evmDenom, sdkmath.NewIntFromUint64(gas)))
			require.NoError(t, testutil.FundAccount(app.BankKeeper, ctx, signer, fees.MulInt(sdkmath.NewInt(2))))

			txCfg := app.TxConfig()
			builder := txCfg.NewTxBuilder()
			require.NoError(t, builder.SetMsgs(banktypes.NewMsgSend(signer, signer, sdk.NewCoins(sdk.NewCoin(evmDenom, sdkmath.NewInt(1))))))
			builder.SetGasLimit(gas)
			builder.SetFeeAmount(fees)

			acc := app.AccountKeeper.GetAccount(ctx, signer)
			require.NoError(t, builder.SetSignatures(signing.SignatureV2{
				PubKey:   priv.PubKey(),
				Data:     &signing.SingleSignatureData{SignMode: tc.signMode},
				Sequence: acc.GetSequence(),
			}))
			sig, err := clienttx.SignWithPrivKey(ctx, tc.signMode, authsigning.SignerData{
				ChainID:       testutil.ChainID,
				AccountNumber: acc.GetAccountNumber(),
				Sequence:      acc.GetSequence(),
				PubKey:        priv.PubKey(),
				Address:       signer.String(),
			}, builder, priv, txCfg, acc.GetSequence())
			require.NoError(t, err)
			if tc.labelMode != nil {
				sig.Data.(*signing.SingleSignatureData).SignMode = *tc.labelMode
			}
			require.NoError(t, builder.SetSignatures(sig))

			txBytes, err := txCfg.TxEncoder()(builder.GetTx())
			require.NoError(t, err)

			res, err := app.FinalizeBlock(&abci.RequestFinalizeBlock{
				Height: app.LastBlockHeight() + 1,
				Txs:    [][]byte{txBytes},
			})
			require.NoError(t, err)
			require.Len(t, res.TxResults, 1)
			require.Equal(t, tc.expCode, res.TxResults[0].Code, res.TxResults[0].Log)
			require.Contains(t, res.TxResults[0].Log, tc.expLog)
		})
	}
}

// TestEthSecp256k1SimulateUnspecifiedSignMode covers gas estimation, where
// clients send the signer's pubkey with SIGN_MODE_UNSPECIFIED and no signature.
func TestEthSecp256k1SimulateUnspecifiedSignMode(t *testing.T) {
	app := testutil.Setup(false, nil)
	ctx := app.BaseApp.NewUncachedContext(false, tmproto.Header{ChainID: testutil.ChainID})
	app.FeeMarketKeeper.SetBaseFee(ctx, big.NewInt(1))

	priv, err := ethsecp256k1.GenerateKey()
	require.NoError(t, err)
	signer := sdk.AccAddress(priv.PubKey().Address())
	acc := app.AccountKeeper.NewAccountWithAddress(ctx, signer)
	require.NoError(t, acc.SetPubKey(priv.PubKey()))
	app.AccountKeeper.SetAccount(ctx, acc)

	evmDenom := app.EvmKeeper.GetParams(ctx).EvmDenom
	gas := uint64(200000)
	fees := sdk.NewCoins(sdk.NewCoin(evmDenom, sdkmath.NewIntFromUint64(gas)))
	require.NoError(t, testutil.FundAccount(app.BankKeeper, ctx, signer, fees.MulInt(sdkmath.NewInt(2))))
	_, err = app.FinalizeBlock(&abci.RequestFinalizeBlock{Height: app.LastBlockHeight() + 1})
	require.NoError(t, err)
	_, err = app.Commit()
	require.NoError(t, err)

	txCfg := app.TxConfig()
	builder := txCfg.NewTxBuilder()
	require.NoError(t, builder.SetMsgs(banktypes.NewMsgSend(signer, signer, sdk.NewCoins(sdk.NewCoin(evmDenom, sdkmath.NewInt(1))))))
	builder.SetGasLimit(gas)
	builder.SetFeeAmount(fees)
	require.NoError(t, builder.SetSignatures(signing.SignatureV2{
		PubKey:   priv.PubKey(),
		Data:     &signing.SingleSignatureData{SignMode: signing.SignMode_SIGN_MODE_UNSPECIFIED},
		Sequence: acc.GetSequence(),
	}))

	txBytes, err := txCfg.TxEncoder()(builder.GetTx())
	require.NoError(t, err)

	_, _, err = app.Simulate(txBytes)
	require.NoError(t, err)
}

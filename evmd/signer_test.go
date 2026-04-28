package evmd

import (
	"math/big"
	"testing"

	sdkmath "cosmossdk.io/math"
	codectypes "github.com/cosmos/cosmos-sdk/codec/types"
	cryptotypes "github.com/cosmos/cosmos-sdk/crypto/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	mempool "github.com/cosmos/cosmos-sdk/types/mempool"
	authtx "github.com/cosmos/cosmos-sdk/x/auth/tx"
	banktypes "github.com/cosmos/cosmos-sdk/x/bank/types"
	ethtypes "github.com/ethereum/go-ethereum/core/types"
	"github.com/evmos/ethermint/encoding"
	"github.com/evmos/ethermint/tests"
	evmtypes "github.com/evmos/ethermint/x/evm/types"
	"github.com/stretchr/testify/require"
)

type stubSignerExtractionAdapter struct {
	called  bool
	signers []mempool.SignerData
	err     error
}

func (s *stubSignerExtractionAdapter) GetSigners(_ sdk.Tx) ([]mempool.SignerData, error) {
	s.called = true
	return s.signers, s.err
}

func TestGetSignersReturnsAllInnerPairs(t *testing.T) {
	chainID := big.NewInt(9000)
	ethSigner := ethtypes.LatestSignerForChainID(chainID)

	addr1, priv1 := tests.NewAddrKey()
	addr2, priv2 := tests.NewAddrKey()

	msg1 := newSignedEthereumMsg(t, ethSigner, chainID, 7, addr1.Bytes(), priv1)
	msg2 := newSignedEthereumMsg(t, ethSigner, chainID, 3, addr2.Bytes(), priv2)
	msg3 := newSignedEthereumMsg(t, ethSigner, chainID, 7, addr1.Bytes(), priv1) // duplicate lane

	tx := buildEthEnvelopeTx(t, msg1, msg2, msg3)
	signerExtractor := NewEthSignerExtractionAdapter(nil)

	signers, err := signerExtractor.GetSigners(tx)
	require.NoError(t, err)
	require.Equal(
		t,
		[]mempool.SignerData{
			mempool.NewSignerData(msg1.GetFrom(), msg1.AsTransaction().Nonce()),
			mempool.NewSignerData(msg2.GetFrom(), msg2.AsTransaction().Nonce()),
		},
		signers,
	)
}

func TestGetSignersFallsBackWithoutInnerEthereumMsgs(t *testing.T) {
	chainID := big.NewInt(9000)
	ethSigner := ethtypes.LatestSignerForChainID(chainID)

	addr1, priv1 := tests.NewAddrKey()
	addr2, _ := tests.NewAddrKey()

	msg := newSignedEthereumMsg(t, ethSigner, chainID, 7, addr1.Bytes(), priv1)
	tx := buildEthEnvelopeTxWithMsgs(
		t,
		[]*evmtypes.MsgEthereumTx{msg},
		banktypes.NewMsgSend(
			sdk.AccAddress(addr1.Bytes()),
			sdk.AccAddress(addr2.Bytes()),
			sdk.NewCoins(sdk.NewCoin(evmtypes.DefaultEVMDenom, sdkmath.NewInt(1))),
		),
	)

	expected := []mempool.SignerData{mempool.NewSignerData(sdk.AccAddress(addr2.Bytes()), 11)}
	fallback := &stubSignerExtractionAdapter{signers: expected}
	signerExtractor := NewEthSignerExtractionAdapter(fallback)

	signers, err := signerExtractor.GetSigners(tx)
	require.NoError(t, err)
	require.True(t, fallback.called)
	require.Equal(t, expected, signers)
}

func TestGetSignersErrorsWhenFallbackIsNil(t *testing.T) {
	addr1, _ := tests.NewAddrKey()
	addr2, _ := tests.NewAddrKey()

	tx := buildEthEnvelopeTxWithMsgs(
		t,
		nil,
		banktypes.NewMsgSend(
			sdk.AccAddress(addr1.Bytes()),
			sdk.AccAddress(addr2.Bytes()),
			sdk.NewCoins(sdk.NewCoin(evmtypes.DefaultEVMDenom, sdkmath.NewInt(1))),
		),
	)

	signerExtractor := NewEthSignerExtractionAdapter(nil)
	signers, err := signerExtractor.GetSigners(tx)
	require.Error(t, err)
	require.Nil(t, signers)
	require.Contains(t, err.Error(), "fallback signer extraction adapter is nil")
}

func newSignedEthereumMsg(
	t *testing.T,
	ethSigner ethtypes.Signer,
	chainID *big.Int,
	nonce uint64,
	from []byte,
	privKey cryptotypes.PrivKey,
) *evmtypes.MsgEthereumTx {
	t.Helper()

	msg := evmtypes.NewTxContract(chainID, nonce, big.NewInt(10), 1000, big.NewInt(1), nil, nil, nil, nil)
	msg.From = from
	require.NoError(t, msg.Sign(ethSigner, tests.NewSigner(privKey)))
	return msg
}

func buildEthEnvelopeTx(t *testing.T, msgs ...*evmtypes.MsgEthereumTx) sdk.Tx {
	t.Helper()
	return buildEthEnvelopeTxWithMsgs(t, msgs, msgsToSDKMsgs(msgs)...)
}

func msgsToSDKMsgs(msgs []*evmtypes.MsgEthereumTx) []sdk.Msg {
	out := make([]sdk.Msg, 0, len(msgs))
	for _, msg := range msgs {
		out = append(out, msg)
	}
	return out
}

func buildEthEnvelopeTxWithMsgs(t *testing.T, ethMsgs []*evmtypes.MsgEthereumTx, msgs ...sdk.Msg) sdk.Tx {
	t.Helper()

	encCfg := encoding.MakeConfig()
	txBuilder := encCfg.TxConfig.NewTxBuilder()
	builder, ok := txBuilder.(authtx.ExtensionOptionsTxBuilder)
	require.True(t, ok)

	option, err := codectypes.NewAnyWithValue(&evmtypes.ExtensionOptionsEthereumTx{})
	require.NoError(t, err)
	builder.SetExtensionOptions(option)

	txFee := sdk.Coins{}
	txGasLimit := uint64(0)

	for _, msg := range ethMsgs {
		txGasLimit += msg.GetGas()
		txFee = txFee.Add(sdk.Coin{
			Denom:  evmtypes.DefaultEVMDenom,
			Amount: sdkmath.NewIntFromBigInt(msg.GetFee()),
		})
	}

	require.NoError(t, builder.SetMsgs(msgs...))
	builder.SetFeeAmount(txFee)
	builder.SetGasLimit(txGasLimit)

	return txBuilder.GetTx()
}

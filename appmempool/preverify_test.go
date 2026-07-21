package appmempool_test

import (
	"math/big"
	"testing"

	sdkmath "cosmossdk.io/math"
	codectypes "github.com/cosmos/cosmos-sdk/codec/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	authtx "github.com/cosmos/cosmos-sdk/x/auth/tx"
	banktypes "github.com/cosmos/cosmos-sdk/x/bank/types"
	ethtypes "github.com/ethereum/go-ethereum/core/types"
	"github.com/stretchr/testify/require"

	"github.com/evmos/ethermint/ante/cache"
	"github.com/evmos/ethermint/appmempool"
	"github.com/evmos/ethermint/tests"
	testutilconfig "github.com/evmos/ethermint/testutil/config"
	ethermint "github.com/evmos/ethermint/types"
	evmtypes "github.com/evmos/ethermint/x/evm/types"
)

const testChainID = "ethermint_9000-1"

func TestEVMSigPreVerifier(t *testing.T) {
	encodingConfig := testutilconfig.MakeConfigForTest(nil)
	decoder := encodingConfig.TxConfig.TxDecoder()
	encoder := encodingConfig.TxConfig.TxEncoder()

	// Unparseable chain ID yields nil: caller keeps admission fully locked.
	require.Nil(t, appmempool.NewEVMSigPreVerifier("garbage", decoder, nil))

	hook := appmempool.NewEVMSigPreVerifier(testChainID, decoder, nil)
	require.NotNil(t, hook)

	chainID, err := ethermint.ParseChainID(testChainID)
	require.NoError(t, err)
	ethSigner := ethtypes.LatestSignerForChainID(chainID)

	addr, priv := tests.NewAddrKey()
	to := tests.GenerateAddress()

	signAndBuild := func(msg *evmtypes.MsgEthereumTx) []byte {
		require.NoError(t, msg.Sign(ethSigner, tests.NewSigner(priv)))
		builder := encodingConfig.TxConfig.NewTxBuilder()
		opt, aerr := codectypes.NewAnyWithValue(&evmtypes.ExtensionOptionsEthereumTx{})
		require.NoError(t, aerr)
		builder.(authtx.ExtensionOptionsTxBuilder).SetExtensionOptions(opt)
		require.NoError(t, builder.SetMsgs(msg))
		builder.SetGasLimit(msg.GetGas())
		builder.SetFeeAmount(sdk.NewCoins(sdk.NewCoin(evmtypes.DefaultEVMDenom, sdkmath.NewIntFromBigInt(msg.GetFee()))))
		bz, berr := encoder(builder.GetTx())
		require.NoError(t, berr)
		return bz
	}

	// Valid signature on a pure-EVM tx passes pre-verification.
	msg := evmtypes.NewTx(chainID, 0, &to, big.NewInt(10), 100000, big.NewInt(1), nil, nil, nil, nil)
	msg.From = addr.Bytes()
	require.NoError(t, hook(signAndBuild(msg)))

	// Undecodable bytes defer to the locked path (nil, not a reject).
	require.NoError(t, hook([]byte("not a tx")))

	// Non-EVM (cosmos) tx defers to the locked path.
	send := banktypes.NewMsgSend(addr.Bytes(), to.Bytes(), sdk.NewCoins(sdk.NewCoin(evmtypes.DefaultEVMDenom, sdkmath.NewInt(1))))
	cosmosTxBuilder := encodingConfig.TxConfig.NewTxBuilder()
	require.NoError(t, cosmosTxBuilder.SetMsgs(send))
	cosmosBz, err := encoder(cosmosTxBuilder.GetTx())
	require.NoError(t, err)
	require.NoError(t, hook(cosmosBz))

	// Tampered sender on a pure-EVM tx is rejected early: sign correctly, then
	// overwrite From so the recovered signer no longer matches.
	badMsg := evmtypes.NewTx(chainID, 0, &to, big.NewInt(10), 100000, big.NewInt(1), nil, nil, nil, nil)
	badMsg.From = addr.Bytes()
	require.NoError(t, badMsg.Sign(ethSigner, tests.NewSigner(priv)))
	badMsg.From = tests.GenerateAddress().Bytes()
	builder := encodingConfig.TxConfig.NewTxBuilder()
	opt, err := codectypes.NewAnyWithValue(&evmtypes.ExtensionOptionsEthereumTx{})
	require.NoError(t, err)
	builder.(authtx.ExtensionOptionsTxBuilder).SetExtensionOptions(opt)
	require.NoError(t, builder.SetMsgs(badMsg))
	builder.SetGasLimit(badMsg.GetGas())
	builder.SetFeeAmount(sdk.NewCoins(sdk.NewCoin(evmtypes.DefaultEVMDenom, sdkmath.NewIntFromBigInt(badMsg.GetFee()))))
	badBz, err := encoder(builder.GetTx())
	require.NoError(t, err)
	require.Error(t, hook(badBz))
}

func TestEVMSigPreVerifier_PopulatesAndHitsSenderCache(t *testing.T) {
	encodingConfig := testutilconfig.MakeConfigForTest(nil)
	decoder := encodingConfig.TxConfig.TxDecoder()
	encoder := encodingConfig.TxConfig.TxEncoder()

	senderCache := cache.NewSenderCache(64)
	hook := appmempool.NewEVMSigPreVerifier(testChainID, decoder, senderCache)
	require.NotNil(t, hook)

	chainID, err := ethermint.ParseChainID(testChainID)
	require.NoError(t, err)
	ethSigner := ethtypes.LatestSignerForChainID(chainID)

	addr, priv := tests.NewAddrKey()
	to := tests.GenerateAddress()

	msg := evmtypes.NewTx(chainID, 0, &to, big.NewInt(10), 100000, big.NewInt(1), nil, nil, nil, nil)
	msg.From = addr.Bytes()
	require.NoError(t, msg.Sign(ethSigner, tests.NewSigner(priv)))
	builder := encodingConfig.TxConfig.NewTxBuilder()
	opt, aerr := codectypes.NewAnyWithValue(&evmtypes.ExtensionOptionsEthereumTx{})
	require.NoError(t, aerr)
	builder.(authtx.ExtensionOptionsTxBuilder).SetExtensionOptions(opt)
	require.NoError(t, builder.SetMsgs(msg))
	builder.SetGasLimit(msg.GetGas())
	builder.SetFeeAmount(sdk.NewCoins(sdk.NewCoin(evmtypes.DefaultEVMDenom, sdkmath.NewIntFromBigInt(msg.GetFee()))))
	txBz, berr := encoder(builder.GetTx())
	require.NoError(t, berr)

	require.NoError(t, hook(txBz))

	require.NoError(t, hook(txBz))
	hits, misses := senderCache.Stats()
	require.Equal(t, uint64(1), hits, "second call through the hook should hit senderCache instead of re-running ecrecover")
	require.Equal(t, uint64(1), misses, "only the first call should miss, populating the cache")

	cached, ok := senderCache.Get(msg.AsTransaction(), ethSigner)
	require.True(t, ok, "hook should have populated senderCache on first verification")
	require.Equal(t, addr, cached)
}

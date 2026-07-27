package ante_test

import (
	"math/big"
	"testing"

	codectypes "github.com/cosmos/cosmos-sdk/codec/types"
	"github.com/ethereum/go-ethereum/common"
	ethtypes "github.com/ethereum/go-ethereum/core/types"
	"github.com/stretchr/testify/require"

	"github.com/evmos/ethermint/ante"
	"github.com/evmos/ethermint/ante/cache"
	"github.com/evmos/ethermint/tests"
	evmtypes "github.com/evmos/ethermint/x/evm/types"
)

func TestVerifyEthSig_ForgedFromRejectedEvenOnCacheHit(t *testing.T) {
	chainID := big.NewInt(9000)
	signer := ethtypes.LatestSignerForChainID(chainID)
	senderCache := cache.NewSenderCache(64)

	realAddr, privKey := tests.NewAddrKey()
	to := tests.GenerateAddress()

	realTx := evmtypes.NewTx(chainID, 0, &to, big.NewInt(10), 100000, big.NewInt(1), nil, nil, nil, nil)
	realTx.From = realAddr.Bytes()
	require.NoError(t, realTx.Sign(signer, tests.NewSigner(privKey)))

	require.NoError(t, ante.VerifyEthSig(realTx, signer, senderCache))

	forgedAddr := tests.GenerateAddress()
	forgedTx := &evmtypes.MsgEthereumTx{Raw: realTx.Raw, From: forgedAddr.Bytes()}

	err := ante.VerifyEthSig(forgedTx, signer, senderCache)
	require.Error(t, err, "cache hit must not bypass the From check")

	cached, ok := senderCache.Get(realTx.AsTransaction(), signer)
	require.True(t, ok)
	require.Equal(t, common.BytesToAddress(realAddr.Bytes()), cached)
}

func TestVerifyEthSig_NilAsTransactionRejected(t *testing.T) {
	chainID := big.NewInt(9000)
	signer := ethtypes.LatestSignerForChainID(chainID)

	// Raw is unset, and Data holds an Any with no cached value, so
	// AsTransaction()'s UnpackTxData fallback fails and returns nil.
	msg := &evmtypes.MsgEthereumTx{Data: &codectypes.Any{TypeUrl: "/unknown.TxData"}}
	require.Nil(t, msg.AsTransaction())

	err := ante.VerifyEthSig(msg, signer, nil)
	require.Error(t, err, "nil AsTransaction() must be rejected, not panic or pass")
}

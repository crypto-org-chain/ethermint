package ante_test

import (
	"context"
	"fmt"
	"math/big"
	"runtime"
	"sync/atomic"
	"testing"
	"time"

	"cosmossdk.io/log/v2"
	"github.com/cosmos/cosmos-sdk/store/v2/rootmulti"
	storetypes "github.com/cosmos/cosmos-sdk/store/v2/types"
	abci "github.com/cometbft/cometbft/abci/types"
	dbm "github.com/cosmos/cosmos-db"
	"github.com/cosmos/cosmos-sdk/baseapp/txnrunner"
	"github.com/cosmos/cosmos-sdk/client"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/ethereum/go-ethereum/common"
	ethtypes "github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/params"
	"github.com/stretchr/testify/require"

	evmante "github.com/evmos/ethermint/ante"
	"github.com/evmos/ethermint/crypto/ethsecp256k1"
	testutilconfig "github.com/evmos/ethermint/testutil/config"
	"github.com/evmos/ethermint/tests"
	evmtypes "github.com/evmos/ethermint/x/evm/types"
)

func newSTMMultiStore(t *testing.T, storeKeys []storetypes.StoreKey) storetypes.CommitMultiStore {
	t.Helper()
	db := dbm.NewMemDB()
	cms := rootmulti.NewStore(db, log.NewNopLogger())
	for _, key := range storeKeys {
		cms.MountStoreWithDB(key, storetypes.StoreTypeIAVL, nil)
	}
	require.NoError(t, cms.LoadLatestVersion())
	return cms
}

func buildSignedEthTxBytes(
	t *testing.T,
	txConfig interface {
		NewTxBuilder() client.TxBuilder
		TxEncoder() sdk.TxEncoder
	},
	chainID *big.Int,
	signer ethtypes.Signer,
	privKey *ethsecp256k1.PrivKey,
	from common.Address,
	nonce uint64,
) []byte {
	t.Helper()
	msg := evmtypes.NewTxContract(
		chainID, nonce, big.NewInt(0),
		params.TxGasContractCreation,
		big.NewInt(1), big.NewInt(1), big.NewInt(1),
		[]byte("data"), nil,
	)
	msg.From = from.Bytes()
	require.NoError(t, msg.Sign(signer, tests.NewSigner(privKey)))
	builder := txConfig.NewTxBuilder()
	require.NoError(t, builder.SetMsgs(msg))
	txBz, err := txConfig.TxEncoder()(builder.GetTx())
	require.NoError(t, err)
	return txBz
}

// countingSigner wraps an ethtypes.Signer and counts how many times the
// expensive Sender() method (ecrecover) is actually invoked. When go-ethereum's
// sigCache is hit, ethtypes.Sender() returns early without calling
// signer.Sender(tx), so the count stays the same.
type countingSigner struct {
	inner ethtypes.Signer
	count atomic.Int64
}

func (s *countingSigner) Sender(tx *ethtypes.Transaction) (common.Address, error) {
	s.count.Add(1)
	return s.inner.Sender(tx)
}

func (s *countingSigner) SignatureValues(tx *ethtypes.Transaction, sig []byte) (r, s2, v *big.Int, err error) {
	return s.inner.SignatureValues(tx, sig)
}

func (s *countingSigner) ChainID() *big.Int                        { return s.inner.ChainID() }
func (s *countingSigner) Hash(tx *ethtypes.Transaction) common.Hash { return s.inner.Hash(tx) }

func (s *countingSigner) Equal(other ethtypes.Signer) bool {
	// Unwrap the other countingSigner so the underlying signer's Equal
	// can compare by type+chainID. Without this, the cache check
	// (sigCache.signer.Equal(signer)) always fails because modernSigner
	// doesn't recognize *countingSigner as an equal type.
	if cs, ok := other.(*countingSigner); ok {
		return s.inner.Equal(cs.inner)
	}
	return s.inner.Equal(other)
}

// TestSTMRunnerSigCachePerformanceGain measures the cost of VerifyEthSig
// with and without the sigCache in a realistic Block-STM scenario.
// The first incarnation populates the cache; simulated re-verifications
// should be measurably cheaper.
func TestSTMRunnerSigCachePerformanceGain(t *testing.T) {
	encCfg := testutilconfig.MakeConfigForTest(nil)
	txConfig := encCfg.TxConfig
	txDecoder := txConfig.TxDecoder()

	chainID := big.NewInt(1)
	signer := ethtypes.LatestSignerForChainID(chainID)

	privKey, err := ethsecp256k1.GenerateKey()
	require.NoError(t, err)
	from := common.BytesToAddress(privKey.PubKey().Address())

	const txCount = 4
	txs := make([][]byte, txCount)
	for i := 0; i < txCount; i++ {
		txs[i] = buildSignedEthTxBytes(t, txConfig, chainID, signer, privKey, from, uint64(i))
	}

	storeKeys := []storetypes.StoreKey{
		storetypes.NewKVStoreKey("acc"),
		storetypes.NewKVStoreKey("bank"),
	}
	cms := newSTMMultiStore(t, storeKeys)

	runner := txnrunner.NewSTMRunner(
		txDecoder, storeKeys, runtime.GOMAXPROCS(0), true,
		func(_ storetypes.MultiStore) string { return evmtypes.DefaultEVMDenom },
	)

	var (
		firstCallTotal  atomic.Int64
		cachedCallTotal atomic.Int64
	)
	const cachedReps = 50

	deliverTx := func(txBytes []byte, memTx sdk.Tx, ms storetypes.MultiStore, txIndex int, cache map[string]any) *abci.ExecTxResult {
		if memTx == nil {
			return &abci.ExecTxResult{Code: 1, Log: "nil memTx"}
		}

		// First call: may or may not hit cache depending on whether another
		// incarnation already ran. We time it regardless.
		start := time.Now()
		if err := evmante.VerifyEthSig(memTx, signer); err != nil {
			return &abci.ExecTxResult{Code: 1, Log: err.Error()}
		}
		firstCallTotal.Add(time.Since(start).Nanoseconds())

		// Subsequent calls: definitely cached
		start = time.Now()
		for i := 0; i < cachedReps; i++ {
			if err := evmante.VerifyEthSig(memTx, signer); err != nil {
				return &abci.ExecTxResult{Code: 1, Log: err.Error()}
			}
		}
		cachedCallTotal.Add(time.Since(start).Nanoseconds())

		return &abci.ExecTxResult{Code: 0}
	}

	results, err := runner.Run(context.Background(), cms, txs, deliverTx)
	require.NoError(t, err)
	for i, result := range results {
		require.Equal(t, uint32(0), result.Code, "tx %d failed: %s", i, result.Log)
	}

	avgFirst := time.Duration(firstCallTotal.Load() / int64(txCount))
	avgCached := time.Duration(cachedCallTotal.Load() / int64(txCount*cachedReps))
	t.Logf("avg first VerifyEthSig: %v, avg cached (%d reps): %v, speedup: %.1fx",
		avgFirst, cachedReps, avgCached, float64(avgFirst)/float64(avgCached))

	// The cached path should be significantly faster (at least 2x).
	// On most machines the speedup is 50-500x.
	if avgFirst > 0 && avgCached > 0 {
		require.Greater(t, float64(avgFirst)/float64(avgCached), 2.0,
			"cached VerifyEthSig should be at least 2x faster than the first call")
	}
}

// TestSTMRunnerSigCacheEcrecoverCount provides definitive proof that go-ethereum's
// sigCache is working. It wraps the signer with a countingSigner that tracks how
// many times the expensive signer.Sender() (ecrecover) is actually called.
//
// Flow inside ethtypes.Sender():
//
//	if sigCache := tx.from.Load(); sigCache != nil && sigCache.signer.Equal(signer) {
//	    return sigCache.from, nil   // <-- cache HIT: signer.Sender() NOT called
//	}
//	addr, _ := signer.Sender(tx)   // <-- cache MISS: signer.Sender() IS called (ecrecover)
//	tx.from.Store(&sigCache{...})
//
// So if we call VerifyEthSig N times on the same *Transaction with the same
// signer, signer.Sender() should be called exactly 1 time (the first call),
// and the remaining N-1 calls return from the cache.
func TestSTMRunnerSigCacheEcrecoverCount(t *testing.T) {
	encCfg := testutilconfig.MakeConfigForTest(nil)
	txConfig := encCfg.TxConfig
	txDecoder := txConfig.TxDecoder()

	chainID := big.NewInt(1)
	realSigner := ethtypes.LatestSignerForChainID(chainID)

	privKey, err := ethsecp256k1.GenerateKey()
	require.NoError(t, err)
	from := common.BytesToAddress(privKey.PubKey().Address())

	const txCount = 8
	txs := make([][]byte, txCount)
	for i := 0; i < txCount; i++ {
		txs[i] = buildSignedEthTxBytes(t, txConfig, chainID, realSigner, privKey, from, uint64(i))
	}

	storeKeys := []storetypes.StoreKey{
		storetypes.NewKVStoreKey("acc"),
		storetypes.NewKVStoreKey("bank"),
	}
	cms := newSTMMultiStore(t, storeKeys)

	runner := txnrunner.NewSTMRunner(
		txDecoder, storeKeys, runtime.GOMAXPROCS(0), true,
		func(_ storetypes.MultiStore) string { return evmtypes.DefaultEVMDenom },
	)

	// Wrap the signer with our counting proxy.
	// IMPORTANT: we must use the same countingSigner instance for all calls
	// so that go-ethereum's Equal() check on the cached signer returns true.
	counting := &countingSigner{inner: realSigner}

	const verifyCalls = 10
	var totalVerifyCalls atomic.Int64

	deliverTx := func(txBytes []byte, memTx sdk.Tx, ms storetypes.MultiStore, txIndex int, cache map[string]any) *abci.ExecTxResult {
		if memTx == nil {
			return &abci.ExecTxResult{Code: 1, Log: "nil memTx"}
		}

		// Call VerifyEthSig multiple times on the same memTx pointer
		for i := 0; i < verifyCalls; i++ {
			if err := evmante.VerifyEthSig(memTx, counting); err != nil {
				return &abci.ExecTxResult{Code: 1, Log: fmt.Sprintf("verify %d failed: %v", i, err)}
			}
		}
		totalVerifyCalls.Add(verifyCalls)

		return &abci.ExecTxResult{Code: 0}
	}

	results, err := runner.Run(context.Background(), cms, txs, deliverTx)
	require.NoError(t, err)
	for i, result := range results {
		require.Equal(t, uint32(0), result.Code, "tx %d failed: %s", i, result.Log)
	}

	ecrecoverCalls := counting.count.Load()
	totalCalls := totalVerifyCalls.Load()

	t.Logf("total VerifyEthSig calls: %d, actual ecrecover (signer.Sender) calls: %d",
		totalCalls, ecrecoverCalls)

	// Each unique *Transaction pointer should cause exactly 1 ecrecover.
	// With txCount=8 transactions, we expect at most txCount ecrecover calls,
	// but totalCalls = txCount * verifyCalls = 80.
	require.LessOrEqual(t, ecrecoverCalls, int64(txCount),
		"ecrecover should be called at most once per tx, got %d for %d txs", ecrecoverCalls, txCount)

	require.Greater(t, totalCalls, ecrecoverCalls,
		"total VerifyEthSig calls (%d) should far exceed ecrecover calls (%d)", totalCalls, ecrecoverCalls)
}

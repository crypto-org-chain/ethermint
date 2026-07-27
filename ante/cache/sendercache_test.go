package cache_test

import (
	"math/big"
	"sync"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	ethtypes "github.com/ethereum/go-ethereum/core/types"

	"github.com/evmos/ethermint/ante/cache"

	"github.com/stretchr/testify/require"
)

var (
	signer      = ethtypes.LatestSignerForChainID(big.NewInt(1))
	otherSigner = ethtypes.LatestSignerForChainID(big.NewInt(2))
)

func newTx(nonce uint64) *ethtypes.Transaction {
	to := common.BigToAddress(big.NewInt(1))
	return ethtypes.NewTx(&ethtypes.LegacyTx{
		Nonce:    nonce,
		To:       &to,
		Value:    big.NewInt(0),
		Gas:      21000,
		GasPrice: big.NewInt(1),
	})
}

func TestSenderCache_SetAndGet(t *testing.T) {
	sendercache := cache.NewSenderCache(64)
	tx := newTx(1)
	addr := common.BigToAddress(big.NewInt(2))

	sendercache.Set(tx, signer, addr)

	got, ok := sendercache.Get(tx, signer)
	require.True(t, ok)
	require.Equal(t, addr, got)
}

func TestSenderCache_MissOnUnknownHash(t *testing.T) {
	sendercache := cache.NewSenderCache(64)

	_, ok := sendercache.Get(newTx(999), signer)
	require.False(t, ok)
}

func TestSenderCache_MissOnSignerMismatch(t *testing.T) {
	sendercache := cache.NewSenderCache(64)
	tx := newTx(1)
	addr := common.BigToAddress(big.NewInt(2))

	sendercache.Set(tx, signer, addr)

	_, ok := sendercache.Get(tx, otherSigner)
	require.False(t, ok, "entry cached with a different signer must not be reused")
}

func TestSenderCache_Stats(t *testing.T) {
	sendercache := cache.NewSenderCache(64)
	tx := newTx(1)
	addr := common.BigToAddress(big.NewInt(2))

	sendercache.Get(tx, signer) // miss
	sendercache.Set(tx, signer, addr)
	sendercache.Get(tx, signer) // hit

	hits, misses := sendercache.Stats()
	require.Equal(t, uint64(1), hits)
	require.Equal(t, uint64(1), misses)
}

func TestSenderCache_ConcurrentAccess(t *testing.T) {
	sendercache := cache.NewSenderCache(256)
	var wg sync.WaitGroup

	for i := range 100 {
		wg.Add(2)
		go func(i int) {
			defer wg.Done()
			sendercache.Set(newTx(uint64(i)), signer, common.BigToAddress(big.NewInt(int64(i))))
		}(i)
		go func(i int) {
			defer wg.Done()
			sendercache.Get(newTx(uint64(i)), signer)
		}(i)
	}
	wg.Wait()
}

func TestSenderCache_EvictsOldestAtCapacity(t *testing.T) {
	sendercache := cache.NewSenderCache(16)
	const n = 4096

	txs := make([]*ethtypes.Transaction, n)
	for i := range n {
		txs[i] = newTx(uint64(i + 1))
		sendercache.Set(txs[i], signer, common.BigToAddress(big.NewInt(int64(i+1))))
	}

	addr, ok := sendercache.Get(txs[n-1], signer)
	require.True(t, ok, "most recently inserted entry should still be cached")
	require.Equal(t, common.BigToAddress(big.NewInt(n)), addr)

	_, ok = sendercache.Get(txs[0], signer)
	require.False(t, ok, "earliest entry should have been evicted by later inserts")
}

func TestSenderCache_NoOpWhenMaxTxIsNotPositive(t *testing.T) {
	for _, maxTx := range []int{0, -1} {
		sendercache := cache.NewSenderCache(maxTx)
		tx := newTx(1)
		addr := common.BigToAddress(big.NewInt(2))

		sendercache.Set(tx, signer, addr)

		_, ok := sendercache.Get(tx, signer)
		require.False(t, ok, "maxTx <= 0 means the cache is a no-op")
	}
}

func TestSenderCache_NilCacheIsNoOp(t *testing.T) {
	var sendercache *cache.SenderCache

	_, ok := sendercache.Get(newTx(1), signer)
	require.False(t, ok)

	sendercache.Set(newTx(1), signer, common.BigToAddress(big.NewInt(2)))

	hits, misses := sendercache.Stats()
	require.Equal(t, uint64(0), hits)
	require.Equal(t, uint64(0), misses)
}

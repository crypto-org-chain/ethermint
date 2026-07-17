package cache_test

import (
	"math/big"
	"sync"
	"testing"

	"github.com/ethereum/go-ethereum/common"

	"github.com/evmos/ethermint/ante/cache"

	"github.com/stretchr/testify/require"
)

func TestSenderCache_SetAndGet(t *testing.T) {
	sendercache := cache.NewSenderCache(64)
	hash := common.BigToHash(big.NewInt(1))
	addr := common.BigToAddress(big.NewInt(2))

	sendercache.Set(hash, addr)

	got, ok := sendercache.Get(hash)
	require.True(t, ok)
	require.Equal(t, addr, got)
}

func TestSenderCache_MissOnUnknownHash(t *testing.T) {
	sendercache := cache.NewSenderCache(64)

	_, ok := sendercache.Get(common.BigToHash(big.NewInt(999)))
	require.False(t, ok)
}

func TestSenderCache_Stats(t *testing.T) {
	sendercache := cache.NewSenderCache(64)
	hash := common.BigToHash(big.NewInt(1))
	addr := common.BigToAddress(big.NewInt(2))

	sendercache.Get(hash) // miss
	sendercache.Set(hash, addr)
	sendercache.Get(hash) // hit

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
			sendercache.Set(common.BigToHash(big.NewInt(int64(i))), common.BigToAddress(big.NewInt(int64(i))))
		}(i)
		go func(i int) {
			defer wg.Done()
			sendercache.Get(common.BigToHash(big.NewInt(int64(i))))
		}(i)
	}
	wg.Wait()
}

func TestSenderCache_EvictsOldestPerShardAtCapacity(t *testing.T) {
	sendercache := cache.NewSenderCache(16)
	const n = 4096

	hashes := make([]common.Hash, n)
	for i := range n {
		hashes[i] = common.BigToHash(big.NewInt(int64(i + 1)))
		sendercache.Set(hashes[i], common.BigToAddress(big.NewInt(int64(i+1))))
	}

	addr, ok := sendercache.Get(hashes[n-1])
	require.True(t, ok, "most recently inserted entry should still be cached")
	require.Equal(t, common.BigToAddress(big.NewInt(n)), addr)

	_, ok = sendercache.Get(hashes[0])
	require.False(t, ok, "earliest entry should have been evicted by later inserts into the same shard")
}

func TestSenderCache_NilCacheIsNoOp(t *testing.T) {
	var sendercache *cache.SenderCache

	_, ok := sendercache.Get(common.BigToHash(big.NewInt(1)))
	require.False(t, ok)

	sendercache.Set(common.BigToHash(big.NewInt(1)), common.BigToAddress(big.NewInt(2)))

	hits, misses := sendercache.Stats()
	require.Equal(t, uint64(0), hits)
	require.Equal(t, uint64(0), misses)
}

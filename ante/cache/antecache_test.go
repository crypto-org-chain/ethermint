package cache_test

import (
	"testing"

	"github.com/evmos/ethermint/ante/cache"

	"github.com/stretchr/testify/require"
)

func TestAnteCache_SetAndExists(t *testing.T) {
	antecache := cache.NewAnteCache(0)
	address := "cosmos1huydeevpz37sd9shv2gqf9p8unc0j89x59cn3c"

	nonce := uint64(42)
	antecache.Set(address, nonce)

	exists := antecache.Exists(address, nonce)
	require.True(t, exists, "expected TxNonce to exist after Set")
}

func TestAnteCache_Delete(t *testing.T) {
	antecache := cache.NewAnteCache(0)
	address := "cosmos1huydeevpz37sd9shv2gqf9p8unc0j89x59cn3c"
	nonce := uint64(42)
	antecache.Set(address, nonce)
	antecache.Delete(address, nonce)

	exists := antecache.Exists(address, nonce)
	require.False(t, exists, "expected TxNonce to not exist after Delete")
}

func TestAnteCache_ExistsForNonExistentNonce(t *testing.T) {
	antecache := cache.NewAnteCache(0)
	address := "cosmos1huydeevpz37sd9shv2gqf9p8unc0j89x59cn3c"

	exists := antecache.Exists(address, 99)
	require.False(t, exists, "expected TxNonce to not exist when not set")
}

func TestAnteCache_ConcurrentAccess(t *testing.T) {
	antecache := cache.NewAnteCache(0)
	address := "cosmos1huydeevpz37sd9shv2gqf9p8unc0j89x59cn3c"

	nonce := uint64(100)
	done := make(chan bool)

	// Writer
	go func() {
		for i := 0; i < 1000; i++ {
			antecache.Set(address, nonce+uint64(i))
		}
		done <- true
	}()

	// Reader
	go func() {
		for i := 0; i < 1000; i++ {
			antecache.Exists(address, nonce+uint64(i))
		}
		done <- true
	}()

	<-done
	<-done
}

// bounded caches should continue tracking the latest nonce
// even after they reach capacity. Right now Set simply returns when
// len(cache) >= maxTx without signalling failure, so callers assume the nonce
// was cached. When that happens, a replacement tx never sees its nonce in the
// cache and gets rejected with ErrInvalidSequence, effectively disabling nonce
// replacement once a node hits maxTx and amplifying the leak documented above.
func TestAnteCache_DropNewEntriesWhenFull(t *testing.T) {
	antecache := cache.NewAnteCache(1)
	address := "cosmos1huydeevpz37sd9shv2gqf9p8unc0j89x59cn3c"

	antecache.Set(address, 1)
	antecache.Set(address, 2)

	require.True(t, antecache.Exists(address, 2), "cache should keep track of replacement nonce even when full")
}

func TestAnteCache_MultipleNoncesPerAddress(t *testing.T) {
	antecache := cache.NewAnteCache(0)
	address := "cosmos1huydeevpz37sd9shv2gqf9p8unc0j89x59cn3c"

	antecache.Set(address, 1)
	antecache.Set(address, 2)

	require.True(t, antecache.Exists(address, 1))
	require.True(t, antecache.Exists(address, 2))
	require.Equal(t, 2, antecache.Size())
}

func TestAnteCache_ExistsShortcut(t *testing.T) {
	antecache := cache.NewAnteCache(0)
	address := "cosmos1huydeevpz37sd9shv2gqf9p8unc0j89x59cn3c"

	require.False(t, antecache.Exists(address, 1))

	antecache.Set(address, 1)
	require.True(t, antecache.Exists(address, 1))

	antecache.Delete(address, 1)
	require.False(t, antecache.Exists(address, 1))
}

package cache_test

import (
	"github.com/evmos/ethermint/ante/cache"
	"testing"

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

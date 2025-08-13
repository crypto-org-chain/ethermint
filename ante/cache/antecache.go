package cache

import (
	"sync"
)

// TxNonce structure for a pair sender and nonce
type TxNonce struct {
	Address string
	Nonce   uint64
}

// AnteCache is a cache used to check that a specific TxNonce exists in a mempool
// Currently it is not possible to read the mempool to check if a transaction-nonce exists already
// We use a cache to track the "potential transaction" in the mempool
// TODO to be removed once we have a better solution implemented in cosmos sdk
type AnteCache struct {
	mu    sync.RWMutex
	cache map[TxNonce]bool
	// - if maxTx == 0, there is no cap on the number of transactions in the cache
	// - if maxTx > 0, the cache will cap the number of transactions it stores,
	// - if maxTx < 0, the cache is a no-op cache.
	maxTx int
}

func NewAnteCache(mempoolMaxTxs int) *AnteCache {
	return &AnteCache{
		cache: make(map[TxNonce]bool),
		maxTx: mempoolMaxTxs,
	}
}

// Set the TxNonce
func (c *AnteCache) Set(address string, nonce uint64) {
	if c.maxTx < 0 {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.maxTx > 0 && len(c.cache) >= c.maxTx {
		return
	}
	key := TxNonce{address, nonce}
	c.cache[key] = true
}

// Delete the TxNonce
func (c *AnteCache) Delete(address string, nonce uint64) {
	if c.maxTx < 0 {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	key := TxNonce{address, nonce}
	delete(c.cache, key)
}

// Exists check if the TxNonce exists
func (c *AnteCache) Exists(address string, nonce uint64) bool {
	if c.maxTx < 0 {
		return false
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	key := TxNonce{address, nonce}
	_, ok := c.cache[key]
	return ok
}

func (c *AnteCache) Size() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.cache)
}

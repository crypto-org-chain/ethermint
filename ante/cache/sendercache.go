package cache

import (
	"sync/atomic"

	"github.com/ethereum/go-ethereum/common"
	lru "github.com/hashicorp/golang-lru/v2"
)

// SenderCache is a process-wide cache mapping an Ethereum tx hash to its
// recovered sender address.
type SenderCache struct {
	lru    *lru.Cache[common.Hash, common.Address]
	hits   atomic.Uint64
	misses atomic.Uint64
}

func NewSenderCache(size int) *SenderCache {
	l, _ := lru.New[common.Hash, common.Address](max(size, 1)) // size >= 1, never errors
	return &SenderCache{lru: l}
}

func (c *SenderCache) Get(hash common.Hash) (common.Address, bool) {
	if c == nil {
		return common.Address{}, false
	}
	addr, ok := c.lru.Get(hash)
	if !ok {
		c.misses.Add(1)
		return common.Address{}, false
	}
	c.hits.Add(1)
	return addr, true
}

func (c *SenderCache) Set(hash common.Hash, addr common.Address) {
	if c == nil {
		return
	}
	c.lru.Add(hash, addr)
}

func (c *SenderCache) Stats() (hits, misses uint64) {
	if c == nil {
		return 0, 0
	}
	return c.hits.Load(), c.misses.Load()
}

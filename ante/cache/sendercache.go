package cache

import (
	"sync/atomic"

	"github.com/ethereum/go-ethereum/common"
	ethtypes "github.com/ethereum/go-ethereum/core/types"
	lru "github.com/hashicorp/golang-lru/v2"
)

type senderCacheKey struct {
	txHash   common.Hash
	signerID common.Hash
}

// SenderCache is a process-wide cache mapping an Ethereum tx to its
// recovered sender address.
type SenderCache struct {
	lru    *lru.Cache[senderCacheKey, common.Address]
	hits   atomic.Uint64
	misses atomic.Uint64
}

func NewSenderCache(mempoolMaxTxs int) *SenderCache {
	if mempoolMaxTxs <= 0 {
		return &SenderCache{}
	}
	l, _ := lru.New[senderCacheKey, common.Address](mempoolMaxTxs)
	return &SenderCache{lru: l}
}

func senderCacheKeyFor(tx *ethtypes.Transaction, signer ethtypes.Signer) senderCacheKey {
	return senderCacheKey{txHash: tx.Hash(), signerID: signer.Hash(tx)}
}

func (c *SenderCache) Get(tx *ethtypes.Transaction, signer ethtypes.Signer) (common.Address, bool) {
	if c == nil || c.lru == nil {
		return common.Address{}, false
	}
	addr, ok := c.lru.Get(senderCacheKeyFor(tx, signer))
	if !ok {
		c.misses.Add(1)
		return common.Address{}, false
	}
	c.hits.Add(1)
	return addr, true
}

func (c *SenderCache) Set(tx *ethtypes.Transaction, signer ethtypes.Signer, addr common.Address) {
	if c == nil || c.lru == nil {
		return
	}
	c.lru.Add(senderCacheKeyFor(tx, signer), addr)
}

func (c *SenderCache) Stats() (hits, misses uint64) {
	if c == nil {
		return 0, 0
	}
	return c.hits.Load(), c.misses.Load()
}

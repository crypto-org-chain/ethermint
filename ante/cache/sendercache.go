package cache

import (
	"crypto/rand"
	"encoding/binary"
	"sync/atomic"

	"github.com/ethereum/go-ethereum/common"
	lru "github.com/hashicorp/golang-lru/v2"
)

const senderCacheShardCount = 16

var senderCacheShardSeed = func() uint32 {
	var b [4]byte
	if _, err := rand.Read(b[:]); err != nil {
		return 0
	}
	return binary.LittleEndian.Uint32(b[:])
}()

// SenderCache is a process-wide cache mapping an Ethereum tx hash to its
// recovered sender address. It's sharded across independent LRUs to reduce
// lock contention under concurrent admission/block execution.
type SenderCache struct {
	shards [senderCacheShardCount]*lru.Cache[common.Hash, common.Address]
	hits   atomic.Uint64
	misses atomic.Uint64
}

func NewSenderCache(size int) *SenderCache {
	perShard := max(size/senderCacheShardCount, 1)
	c := &SenderCache{}
	for i := range c.shards {
		shard, _ := lru.New[common.Hash, common.Address](perShard)
		c.shards[i] = shard
	}
	return c
}

func (c *SenderCache) shardFor(hash common.Hash) *lru.Cache[common.Hash, common.Address] {
	prefix := binary.BigEndian.Uint32(hash[:4]) ^ senderCacheShardSeed
	return c.shards[prefix%senderCacheShardCount]
}

func (c *SenderCache) Get(hash common.Hash) (common.Address, bool) {
	if c == nil {
		return common.Address{}, false
	}
	addr, ok := c.shardFor(hash).Get(hash)
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
	c.shardFor(hash).Add(hash, addr)
}

func (c *SenderCache) Stats() (hits, misses uint64) {
	if c == nil {
		return 0, 0
	}
	return c.hits.Load(), c.misses.Load()
}

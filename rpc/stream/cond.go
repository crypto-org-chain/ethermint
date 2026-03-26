package stream

import (
	"context"
	"sync"
)

// Cond implements conditional variable with a channel
type Cond struct {
	mu sync.Mutex // guards ch
	ch chan struct{}
}

func NewCond() *Cond {
	return &Cond{ch: make(chan struct{})}
}

// WaitChan returns the current wait channel.
// Capture this while holding relevant locks to avoid missing broadcasts
// between releasing the lock and starting to wait.
func (c *Cond) WaitChan() <-chan struct{} {
	c.mu.Lock()
	ch := c.ch
	c.mu.Unlock()
	return ch
}

// WaitOnChan waits on a previously captured channel, returns true if signaled, false if canceled.
func (c *Cond) WaitOnChan(ctx context.Context, ch <-chan struct{}) bool {
	select {
	case <-ch:
		return true
	case <-ctx.Done():
		return false
	}
}

func (c *Cond) Broadcast() {
	c.mu.Lock()
	defer c.mu.Unlock()
	close(c.ch)
	c.ch = make(chan struct{})
}

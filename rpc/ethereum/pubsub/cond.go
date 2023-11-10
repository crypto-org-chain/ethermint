package pubsub

import (
	"context"
)

// Cond implements conditional variable with a channel
type Cond struct {
	ch chan struct{}
}

func NewCond() *Cond {
	return &Cond{make(chan struct{})}
}

// Wait returns true if the condition is signaled, false if the context is canceled
func (c *Cond) Wait(ctx context.Context) bool {
	select {
	case <-c.ch:
		return true
	case <-ctx.Done():
		return false
	}
}

func (c *Cond) Broadcast() {
	close(c.ch)
	c.ch = make(chan struct{})
}

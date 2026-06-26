package appmempool_test

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/evmos/ethermint/appmempool"
)

func TestPreVerifierRegistry(t *testing.T) {
	reject := errors.New("reject")

	t.Run("empty registry defers", func(t *testing.T) {
		var r appmempool.PreVerifierRegistry
		require.NoError(t, r.Verify(nil))
	})

	t.Run("nil verifier ignored", func(t *testing.T) {
		var r appmempool.PreVerifierRegistry
		r.Register(nil)
		require.NoError(t, r.Verify(nil))
	})

	t.Run("first rejection wins, later verifiers not run", func(t *testing.T) {
		var r appmempool.PreVerifierRegistry
		ran := false
		r.Register(func([]byte) error { return reject })
		r.Register(func([]byte) error { ran = true; return nil })
		require.ErrorIs(t, r.Verify(nil), reject)
		require.False(t, ran, "verifier after a rejection must not run")
	})

	t.Run("all defer", func(t *testing.T) {
		var r appmempool.PreVerifierRegistry
		count := 0
		r.Register(func([]byte) error { count++; return nil })
		r.Register(func([]byte) error { count++; return nil })
		require.NoError(t, r.Verify(nil))
		require.Equal(t, 2, count)
	})
}

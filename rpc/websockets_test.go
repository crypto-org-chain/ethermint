package rpc

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestBuildOriginAllowlist(t *testing.T) {
	t.Run("empty", func(t *testing.T) {
		allowAll, origins, errs := buildOriginAllowlist(nil)
		require.False(t, allowAll)
		require.Len(t, origins, 0)
		require.Empty(t, errs)
	})

	t.Run("star", func(t *testing.T) {
		allowAll, origins, errs := buildOriginAllowlist([]string{"*"})
		require.True(t, allowAll)
		require.Nil(t, origins)
		require.Empty(t, errs)
	})

	t.Run("starMixedWithOthers", func(t *testing.T) {
		allowAll, origins, errs := buildOriginAllowlist([]string{"*", "http://example.com"})
		require.False(t, allowAll)
		require.NotNil(t, origins)
		require.Len(t, origins, 1)
		require.NotEmpty(t, errs)
	})

	t.Run("normalizes", func(t *testing.T) {
		allowAll, origins, errs := buildOriginAllowlist([]string{" HTTP://Example.COM ", "http://example.com/"})
		require.False(t, allowAll)
		require.Len(t, origins, 1)
		_, ok := origins["http://example.com"]
		require.True(t, ok)
		require.Empty(t, errs)
	})

	t.Run("invalidOrigin", func(t *testing.T) {
		allowAll, origins, errs := buildOriginAllowlist([]string{"not a url"})
		require.False(t, allowAll)
		require.NotNil(t, origins)
		require.Len(t, origins, 0)
		require.NotEmpty(t, errs)
	})
}

func TestIsOriginAllowed(t *testing.T) {
	t.Run("emptyOriginAllowedWhenNoAllowlist", func(t *testing.T) {
		s := &websocketsServer{
			wsOriginAllowAll: false,
			wsOrigins:        map[string]struct{}{},
		}
		require.True(t, s.isOriginAllowed(""))
		require.False(t, s.isOriginAllowed("http://example.com"))
	})

	t.Run("allowlistEnforced", func(t *testing.T) {
		s := &websocketsServer{
			wsOriginAllowAll: false,
			wsOrigins: map[string]struct{}{
				"http://allowed.example": {},
			},
		}
		require.True(t, s.isOriginAllowed(""))
		require.True(t, s.isOriginAllowed("http://allowed.example"))
		require.True(t, s.isOriginAllowed("HTTP://ALLOWED.EXAMPLE:80/"))
		require.False(t, s.isOriginAllowed("http://blocked.example"))
	})

	t.Run("allowAll", func(t *testing.T) {
		s := &websocketsServer{
			wsOriginAllowAll: true,
		}
		require.True(t, s.isOriginAllowed(""))
		require.True(t, s.isOriginAllowed("http://any.example"))
	})
}

func TestNamespaceAllowed(t *testing.T) {
	t.Run("allowed", func(t *testing.T) {
		s := &websocketsServer{
			allowedAPIs: buildAllowedAPIs([]string{"eth", "net"}),
		}
		require.True(t, s.namespaceAllowed("eth"))
		require.True(t, s.namespaceAllowed("ETH"))
		require.False(t, s.namespaceAllowed("web3"))
	})

	t.Run("empty", func(t *testing.T) {
		s := &websocketsServer{}
		require.False(t, s.namespaceAllowed("eth"))
	})
}

func TestBatchContainsEthSubscription(t *testing.T) {
	t.Run("containsSubscribe", func(t *testing.T) {
		raw := []byte(`[{"id":1,"method":"eth_subscribe","params":["newHeads"]}]`)
		require.True(t, batchContainsEthSubscription(raw))
	})

	t.Run("containsUnsubscribe", func(t *testing.T) {
		raw := []byte(`[{"id":1,"method":"eth_unsubscribe","params":["0x1"]}]`)
		require.True(t, batchContainsEthSubscription(raw))
	})

	t.Run("noSubscribe", func(t *testing.T) {
		raw := []byte(`[{"id":1,"method":"net_version"}]`)
		require.False(t, batchContainsEthSubscription(raw))
	})

	t.Run("mixedBatch", func(t *testing.T) {
		raw := []byte(`[{"id":1,"method":"net_version"},{"id":2,"method":"eth_subscribe","params":["newHeads"]}]`)
		require.True(t, batchContainsEthSubscription(raw))
	})

	t.Run("invalidJson", func(t *testing.T) {
		raw := []byte(`[{]`)
		require.False(t, batchContainsEthSubscription(raw))
	})
}

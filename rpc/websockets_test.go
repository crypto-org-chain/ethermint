package rpc

import (
	"context"
	"encoding/json"
	"net"
	"testing"

	"cosmossdk.io/log/v2"
	"github.com/stretchr/testify/require"
)

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

func TestFilterBatchEthSubscriptions(t *testing.T) {
	t.Run("noSubscriptions_returnedUnchanged", func(t *testing.T) {
		raw := []byte(`[{"id":1,"method":"net_version"}]`)
		got, blocked, hasItems := filterBatchEthSubscriptions(raw)
		require.True(t, hasItems)
		require.Equal(t, 0, blocked)
		require.Equal(t, raw, got)
	})

	t.Run("onlySubscribe_allFiltered", func(t *testing.T) {
		raw := []byte(`[{"id":1,"method":"eth_subscribe","params":["newHeads"]}]`)
		_, blocked, hasItems := filterBatchEthSubscriptions(raw)
		require.False(t, hasItems)
		require.Equal(t, 1, blocked)
	})

	t.Run("onlyUnsubscribe_allFiltered", func(t *testing.T) {
		raw := []byte(`[{"id":1,"method":"eth_unsubscribe","params":["0x1"]}]`)
		_, blocked, hasItems := filterBatchEthSubscriptions(raw)
		require.False(t, hasItems)
		require.Equal(t, 1, blocked)
	})

	t.Run("mixed_subscribeFiltered_othersForwarded", func(t *testing.T) {
		raw := []byte(`[{"id":1,"method":"net_version"},{"id":2,"method":"eth_subscribe","params":["newHeads"]}]`)
		got, blocked, hasItems := filterBatchEthSubscriptions(raw)
		require.True(t, hasItems)
		require.Equal(t, 1, blocked)

		var items []json.RawMessage
		require.NoError(t, json.Unmarshal(got, &items))
		require.Len(t, items, 1)
		var item map[string]interface{}
		require.NoError(t, json.Unmarshal(items[0], &item))
		require.Equal(t, "net_version", item["method"])
	})

	t.Run("invalidJson_returnedUnchanged", func(t *testing.T) {
		raw := []byte(`[{]`)
		got, blocked, hasItems := filterBatchEthSubscriptions(raw)
		require.True(t, hasItems)
		require.Equal(t, 0, blocked)
		require.Equal(t, raw, got)
	})
}

// TestWebsocketsServerStartBindError ensures Start() returns bind failures
// to the caller instead of swallowing them in the serve goroutine.
func TestWebsocketsServerStartBindError(t *testing.T) {
	t.Run("bind failure is returned", func(t *testing.T) {
		// occupy an ephemeral port, then point the WS server at the same address
		occupied, err := net.Listen("tcp", "127.0.0.1:0")
		require.NoError(t, err)
		t.Cleanup(func() { _ = occupied.Close() })

		s := &websocketsServer{
			wsAddr: occupied.Addr().String(),
			logger: log.NewNopLogger(),
		}
		require.Error(t, s.Start())
	})

	t.Run("successful bind returns nil", func(t *testing.T) {
		s := &websocketsServer{
			wsAddr: "127.0.0.1:0", // ephemeral free port
			logger: log.NewNopLogger(),
		}
		require.NoError(t, s.Start())
		// Stop() unwinds the serve goroutine so it doesn't leak
		t.Cleanup(func() {
			_ = s.Stop(context.Background())
		})
	})

	t.Run("Stop before Start is a no-op", func(t *testing.T) {
		s := &websocketsServer{logger: log.NewNopLogger()}
		require.NoError(t, s.Stop(context.Background()))
	})

	t.Run("bad TLS keypair is returned", func(t *testing.T) {
		// a bad cert/key is loaded inside ServeTLS; ensure Start() surfaces it
		// synchronously instead of swallowing it in the serve goroutine
		s := &websocketsServer{
			wsAddr:   "127.0.0.1:0", // ephemeral free port
			certFile: "/nonexistent/cert.pem",
			keyFile:  "/nonexistent/key.pem",
			logger:   log.NewNopLogger(),
		}
		require.Error(t, s.Start())
	})
}

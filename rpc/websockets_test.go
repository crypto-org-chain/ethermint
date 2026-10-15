package rpc

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"math/big"
	"net"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"cosmossdk.io/log/v2"
	"github.com/ethereum/go-ethereum/common"
	"github.com/gorilla/websocket"
	pkgerrors "github.com/pkg/errors"
	"github.com/stretchr/testify/require"

	rpcfilters "github.com/evmos/ethermint/rpc/namespaces/ethereum/eth/filters"
	"github.com/evmos/ethermint/rpc/types"
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

func TestReadLoop_ErrorResponses(t *testing.T) {
	hexes := func(n int) []interface{} {
		out := make([]interface{}, n)
		for i := range out {
			out[i] = common.BigToHash(big.NewInt(int64(i + 1))).Hex()
		}
		return out
	}
	addresses := make([]interface{}, rpcfilters.MaxLogQueryEntries+1)
	for i := range addresses {
		addresses[i] = common.BigToAddress(big.NewInt(int64(i + 1))).Hex()
	}
	subscribeLogs := func(id interface{}, crit map[string]interface{}) map[string]interface{} {
		return map[string]interface{}{
			"jsonrpc": "2.0",
			"id":      id,
			"method":  methodEthSubscribe,
			"params":  []interface{}{"logs", crit},
		}
	}

	tests := []struct {
		name     string
		req      map[string]interface{}
		wantMsg  string
		wantCode int64
		wantID   string // "null" when the id is not echoed
	}{
		{
			"addresses over limit",
			subscribeLogs(1, map[string]interface{}{"address": addresses}),
			"exceed max addresses", types.ErrCodeInvalidParams, "1",
		},
		{
			"topic positions over limit",
			subscribeLogs(2, map[string]interface{}{"topics": hexes(rpcfilters.MaxTopics + 1)}),
			"exceed max topics", types.ErrCodeInvalidParams, "2",
		},
		{
			"sub-topics over limit",
			subscribeLogs(3, map[string]interface{}{"topics": []interface{}{nil, hexes(rpcfilters.MaxLogQueryEntries + 1)}}),
			"exceed max topics per search position", types.ErrCodeInvalidParams, "3",
		},
		{
			"large integer id echoed exactly",
			subscribeLogs(1e19, map[string]interface{}{"address": addresses}),
			"exceed max addresses", types.ErrCodeInvalidParams, "10000000000000000000",
		},
		{
			"non-integer id rejected",
			subscribeLogs(1.5, map[string]interface{}{"address": addresses}),
			"connection ID must be an integer", errCodeInvalidRequest, "null",
		},
		{
			"numeric string id",
			subscribeLogs("7", map[string]interface{}{"address": addresses}),
			"exceed max addresses", types.ErrCodeInvalidParams, "7",
		},
		{
			"NaN id rejected",
			subscribeLogs("NaN", map[string]interface{}{"address": addresses}),
			"connection ID must be an integer", errCodeInvalidRequest, "null",
		},
		{
			"infinite id rejected",
			subscribeLogs("-Inf", map[string]interface{}{"address": addresses}),
			"connection ID must be an integer", errCodeInvalidRequest, "null",
		},
		{
			"subscribe params not a list",
			map[string]interface{}{"jsonrpc": "2.0", "id": 4, "method": methodEthSubscribe, "params": "logs"},
			"invalid parameters", errCodeInvalidRequest, "4",
		},
		{
			"subscribe empty params",
			map[string]interface{}{"jsonrpc": "2.0", "id": 5, "method": methodEthSubscribe, "params": []interface{}{}},
			"empty parameters", errCodeInvalidRequest, "5",
		},
		{
			"unsubscribe non-string id",
			map[string]interface{}{"jsonrpc": "2.0", "id": 6, "method": methodEthUnsubscribe, "params": []interface{}{1}},
			"invalid parameters", errCodeInvalidRequest, "6",
		},
		{
			// rpcAddr is unset, so forwarding to the HTTP server fails
			"forwarded method fails",
			map[string]interface{}{"jsonrpc": "2.0", "id": 8, "method": "eth_blockNumber", "params": []interface{}{}},
			"Could not perform request", errCodeInvalidRequest, "8",
		},
	}

	// events nil: validation must reject before subscribing
	srv := httptest.NewServer(&websocketsServer{
		api:              &pubSubAPI{logger: log.NewNopLogger()},
		logger:           log.NewNopLogger(),
		wsOriginAllowAll: true,
		allowedAPIs:      map[string]struct{}{"eth": {}},
	})
	defer srv.Close()

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			conn, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(srv.URL, "http"), nil)
			require.NoError(t, err)
			defer conn.Close()

			require.NoError(t, conn.WriteJSON(tc.req))
			require.NoError(t, conn.SetReadDeadline(time.Now().Add(5*time.Second)))
			var res ErrorResponseJSON
			require.NoError(t, conn.ReadJSON(&res))
			require.Contains(t, res.Error.Message, tc.wantMsg)
			require.Equal(t, tc.wantCode, res.Error.Code.Int64())
			gotID := "null"
			if res.ID != nil {
				gotID = res.ID.String()
			}
			require.Equal(t, tc.wantID, gotID)
		})
	}
}

func TestErrCode(t *testing.T) {
	invalidParams := &types.InvalidParamsError{Message: "bad"}

	tests := []struct {
		name string
		err  error
		want int
	}{
		{"rpc error", invalidParams, types.ErrCodeInvalidParams},
		{"wrapped with fmt", fmt.Errorf("subscribe: %w", invalidParams), types.ErrCodeInvalidParams},
		{"wrapped with pkg/errors", pkgerrors.Wrap(invalidParams, "subscribe"), types.ErrCodeInvalidParams},
		{"plain error", errors.New("plain"), errCodeInvalidRequest},
		{"wrapped plain error", fmt.Errorf("subscribe: %w", errors.New("plain")), errCodeInvalidRequest},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, errCode(tc.err))
		})
	}
}

func TestBigID(t *testing.T) {
	tests := []struct {
		name string
		id   float64
		want string // "null" for a nil result
	}{
		{"zero", 0, "0"},
		{"small integer", 7, "7"},
		{"negative integer", -3, "-3"},
		{"beyond int64", 1e19, "10000000000000000000"},
		{"NaN", math.NaN(), "null"},
		{"positive infinity", math.Inf(1), "null"},
		{"negative infinity", math.Inf(-1), "null"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := "null"
			require.NotPanics(t, func() {
				if n := bigID(tc.id); n != nil {
					got = n.String()
				}
			})
			require.Equal(t, tc.want, got)
		})
	}
}

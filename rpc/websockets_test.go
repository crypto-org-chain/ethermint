package rpc

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"cosmossdk.io/log/v2"
	coretypes "github.com/cometbft/cometbft/rpc/core/types"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/rpc"
	"github.com/gorilla/websocket"
	pkgerrors "github.com/pkg/errors"
	"github.com/stretchr/testify/require"

	rpcfilters "github.com/evmos/ethermint/rpc/namespaces/ethereum/eth/filters"
	"github.com/evmos/ethermint/rpc/stream"
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

func TestFilterBatch(t *testing.T) {
	ethDisabled := (&websocketsServer{allowedAPIs: buildAllowedAPIs([]string{"net"})}).rejectionFor
	ethEnabled := (&websocketsServer{allowedAPIs: buildAllowedAPIs([]string{"eth"})}).rejectionFor
	errCodes := func(t *testing.T, rejected []json.RawMessage) map[string]int64 {
		t.Helper()
		out := make(map[string]int64, len(rejected))
		for _, r := range rejected {
			var res struct {
				ID    json.RawMessage `json:"id"`
				Error struct {
					Code int64 `json:"code"`
				} `json:"error"`
			}
			require.NoError(t, json.Unmarshal(r, &res))
			out[string(res.ID)] = res.Error.Code
		}
		return out
	}

	t.Run("nothing rejected: returned unchanged", func(t *testing.T) {
		raw := []byte(`[{"id":1,"method":"net_version"},{"id":2,"method":"eth_subscribe"}]`)
		got, rejected := filterBatch(raw, ethEnabled)
		require.Empty(t, rejected)
		require.Equal(t, raw, got)
	})

	t.Run("all rejected: nothing forwarded", func(t *testing.T) {
		raw := []byte(`[{"id":1,"method":"eth_subscribe","params":["newHeads"]},{"id":2,"method":"eth_unsubscribe","params":["0x1"]}]`)
		got, rejected := filterBatch(raw, ethDisabled)
		require.Nil(t, got)
		require.Equal(t, map[string]int64{"1": errCodeInvalidRequest, "2": errCodeInvalidRequest}, errCodes(t, rejected))
	})

	t.Run("both rules applied in one pass", func(t *testing.T) {
		raw := []byte(`[{"id":1,"method":"net_version"},{"id":2,"method":"eth_subscribe","params":["newHeads"]},` +
			`{"id":3,"method":"eth_sendRawTransactionSync","params":["0x00"]},{"method":"eth_sendRawTransactionSync"},` +
			`{"id":null,"method":"eth_sendRawTransactionSync"}]`)
		got, rejected := filterBatch(raw, ethDisabled)
		require.Equal(t,
			map[string]int64{"2": errCodeInvalidRequest, "3": _errCodeMethodNotFound, "null": _errCodeMethodNotFound},
			errCodes(t, rejected), "only the item without an id is a notification")

		var items []map[string]interface{}
		require.NoError(t, json.Unmarshal(got, &items))
		require.Len(t, items, 1)
		require.Equal(t, "net_version", items[0]["method"])
	})

	t.Run("invalid json: returned unchanged", func(t *testing.T) {
		raw := []byte(`[{]`)
		got, rejected := filterBatch(raw, ethDisabled)
		require.Empty(t, rejected)
		require.Equal(t, raw, got)
	})
}

// TestReadLoop_BatchRejectsSyncSend checks that blocked batch items are answered
// inside the batch response array with their original ids.
func TestReadLoop_BatchRejectsSyncSend(t *testing.T) {
	rpcSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var reqs []map[string]json.RawMessage
		if err := json.NewDecoder(r.Body).Decode(&reqs); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		for _, req := range reqs {
			if string(req["method"]) == `"trigger_batch_error"` {
				_, _ = w.Write([]byte(`{"jsonrpc":"2.0","id":null,"error":{"code":-32600,"message":"batch too large"}}`))
				return
			}
		}
		if len(reqs) == 0 {
			_, _ = w.Write([]byte(`{"jsonrpc":"2.0","id":null,"error":{"code":-32600,"message":"empty batch"}}`))
			return
		}
		res := make([]map[string]json.RawMessage, 0, len(reqs))
		for _, req := range reqs {
			if req["id"] == nil {
				continue
			}
			res = append(res, map[string]json.RawMessage{
				"jsonrpc": json.RawMessage(`"2.0"`),
				"id":      req["id"],
				"result":  json.RawMessage(`"0x1"`),
			})
		}
		if len(res) == 0 {
			// A batch of only notifications gets no response body.
			return
		}
		_ = json.NewEncoder(w).Encode(res)
	}))
	defer rpcSrv.Close()

	srv := httptest.NewServer(&websocketsServer{
		rpcAddr:          strings.TrimPrefix(rpcSrv.URL, "http://"),
		api:              &pubSubAPI{logger: log.NewNopLogger()},
		logger:           log.NewNopLogger(),
		wsOriginAllowAll: true,
		allowedAPIs:      map[string]struct{}{"eth": {}},
	})
	defer srv.Close()

	conn, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(srv.URL, "http"), nil)
	require.NoError(t, err)
	defer conn.Close()
	require.NoError(t, conn.SetReadDeadline(time.Now().Add(5*time.Second)))

	type response struct {
		ID     json.RawMessage `json:"id"`
		Result json.RawMessage `json:"result"`
		Error  *struct {
			Code    int64  `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}

	t.Run("all blocked", func(t *testing.T) {
		require.NoError(t, conn.WriteJSON([]map[string]interface{}{
			{"jsonrpc": "2.0", "id": 1, "method": _methodEthSendRawTransactionSync, "params": []interface{}{"0x00"}},
			{"jsonrpc": "2.0", "id": "a", "method": _methodEthSendRawTransactionSync, "params": []interface{}{"0x01"}},
			{"jsonrpc": "2.0", "method": _methodEthSendRawTransactionSync, "params": []interface{}{"0x02"}},
		}))
		var res []response
		require.NoError(t, conn.ReadJSON(&res))
		require.Len(t, res, 2)
		for i, wantID := range []string{"1", `"a"`} {
			require.Equal(t, wantID, string(res[i].ID))
			require.NotNil(t, res[i].Error)
			require.Equal(t, _errMsgSyncOverWS, res[i].Error.Message)
			require.Equal(t, int64(_errCodeMethodNotFound), res[i].Error.Code)
		}
	})

	t.Run("mixed", func(t *testing.T) {
		require.NoError(t, conn.WriteJSON([]map[string]interface{}{
			{"jsonrpc": "2.0", "id": 1, "method": "eth_blockNumber"},
			{"jsonrpc": "2.0", "id": 2, "method": _methodEthSendRawTransactionSync, "params": []interface{}{"0x00"}},
		}))
		var res []response
		require.NoError(t, conn.ReadJSON(&res))
		require.Len(t, res, 2)
		byID := make(map[string]response, len(res))
		for _, r := range res {
			byID[string(r.ID)] = r
		}
		require.Equal(t, `"0x1"`, string(byID["1"].Result))
		require.NotNil(t, byID["2"].Error)
		require.Equal(t, int64(_errCodeMethodNotFound), byID["2"].Error.Code)
	})

	t.Run("forwarded items are all notifications", func(t *testing.T) {
		require.NoError(t, conn.WriteJSON([]map[string]interface{}{
			{"jsonrpc": "2.0", "id": 1, "method": _methodEthSendRawTransactionSync, "params": []interface{}{"0x00"}},
			{"jsonrpc": "2.0", "method": "eth_blockNumber"},
		}))
		var res []response
		require.NoError(t, conn.ReadJSON(&res))
		require.Len(t, res, 1)
		require.Equal(t, "1", string(res[0].ID))
		require.NotNil(t, res[0].Error)
		require.Equal(t, int64(_errCodeMethodNotFound), res[0].Error.Code)
	})

	t.Run("empty batch is forwarded", func(t *testing.T) {
		require.NoError(t, conn.WriteMessage(websocket.TextMessage, []byte(`[]`)))
		var res response
		require.NoError(t, conn.ReadJSON(&res))
		require.NotNil(t, res.Error)
		require.Equal(t, "empty batch", res.Error.Message)
	})

	t.Run("escaped method name and key in batch", func(t *testing.T) {
		require.NoError(t, conn.WriteMessage(websocket.TextMessage, []byte(
			"[{\"jsonrpc\":\"2.0\",\"id\":7,\"method\":\"eth_sendRawTransaction\\u0053ync\",\"params\":[\"0x00\"]},"+
				"{\"jsonrpc\":\"2.0\",\"id\":8,\"meth\\u006fd\":\"eth_sendRawTransactionSync\",\"params\":[\"0x00\"]}]")))
		var res []response
		require.NoError(t, conn.ReadJSON(&res))
		require.Len(t, res, 2)
		for i, wantID := range []string{"7", "8"} {
			require.Equal(t, wantID, string(res[i].ID))
			require.NotNil(t, res[i].Error)
			require.Equal(t, int64(_errCodeMethodNotFound), res[i].Error.Code)
		}
	})

	t.Run("single request with a duplicate method key the server ignores", func(t *testing.T) {
		for _, req := range []string{
			`{"jsonrpc":"2.0","id":"x","method":"eth_sendRawTransactionSync","Method":"net_version","params":["0x00"]}`,
			`{"jsonrpc":"2.0","id":"x","method":"eth_sendRawTransactionSync","method":0,"params":["0x00"]}`,
		} {
			require.NoError(t, conn.WriteMessage(websocket.TextMessage, []byte(req)))
			var res response
			require.NoError(t, conn.ReadJSON(&res))
			require.Equal(t, `"x"`, string(res.ID), req)
			require.NotNil(t, res.Error, req)
			require.Equal(t, int64(_errCodeMethodNotFound), res.Error.Code, req)
		}
	})

	t.Run("whole-batch server error is the only reply", func(t *testing.T) {
		require.NoError(t, conn.WriteJSON([]map[string]interface{}{
			{"jsonrpc": "2.0", "id": 1, "method": "trigger_batch_error"},
			{"jsonrpc": "2.0", "id": 2, "method": _methodEthSendRawTransactionSync, "params": []interface{}{"0x00"}},
		}))
		var res response
		require.NoError(t, conn.ReadJSON(&res))
		require.NotNil(t, res.Error)
		require.Equal(t, "batch too large", res.Error.Message)

		// The next reply must belong to the next request, not to leftover rejections.
		require.NoError(t, conn.WriteMessage(websocket.TextMessage, []byte(`[]`)))
		require.NoError(t, conn.ReadJSON(&res))
		require.Equal(t, "empty batch", res.Error.Message)
	})
}

// TestReadLoop_RoutesOnCheckedMethod checks that the method the rejection rule sees is
// the one the RPC server would dispatch, which only reads the exact "method" key.
func TestReadLoop_RoutesOnCheckedMethod(t *testing.T) {
	rpcSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"jsonrpc":"2.0","id":1,"result":"forwarded"}`))
	}))
	defer rpcSrv.Close()

	srv := httptest.NewServer(&websocketsServer{
		rpcAddr:          strings.TrimPrefix(rpcSrv.URL, "http://"),
		api:              &pubSubAPI{logger: log.NewNopLogger()},
		logger:           log.NewNopLogger(),
		wsOriginAllowAll: true,
		allowedAPIs:      buildAllowedAPIs([]string{"net"}),
	})
	defer srv.Close()

	conn, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(srv.URL, "http"), nil)
	require.NoError(t, err)
	defer conn.Close()
	require.NoError(t, conn.SetReadDeadline(time.Now().Add(5*time.Second)))

	type response struct {
		Result string `json:"result"`
		Error  *struct {
			Message string `json:"message"`
		} `json:"error"`
	}

	t.Run("a case-variant key does not hide eth_subscribe", func(t *testing.T) {
		require.NoError(t, conn.WriteMessage(websocket.TextMessage,
			[]byte(`{"jsonrpc":"2.0","id":1,"method":"eth_subscribe","Method":"net_version","params":["newHeads"]}`)))
		var res response
		require.NoError(t, conn.ReadJSON(&res))
		require.NotNil(t, res.Error)
		require.Equal(t, "eth namespace is disabled", res.Error.Message)
	})

	t.Run("a case-variant method key alone is not a method", func(t *testing.T) {
		require.NoError(t, conn.WriteMessage(websocket.TextMessage,
			[]byte(`{"jsonrpc":"2.0","id":2,"Method":"eth_subscribe","params":["newHeads"]}`)))
		var res response
		require.NoError(t, conn.ReadJSON(&res))
		require.Equal(t, "forwarded", res.Result, "forwarded for the server to reject")
	})
}

func TestParseWSRequest(t *testing.T) {
	tests := []struct {
		name       string
		raw        string
		wantOK     bool
		wantMethod string
		wantID     string
	}{
		{"plain", `{"id":1,"method":"eth_call"}`, true, "eth_call", "1"},
		{"exact keys only", `{"ID":1,"Method":"eth_call"}`, true, "", ""},
		{"last method wins", `{"method":"a","method":"b"}`, true, "b", ""},
		{"non-string method ignored", `{"method":"a","method":0}`, true, "a", ""},
		{"null method ignored", `{"method":"a","method":null}`, true, "a", ""},
		{"escaped key", "{\"meth\\u006fd\":\"a\"}", true, "a", ""},
		{"escaped value", "{\"method\":\"\\u0061\"}", true, "a", ""},
		{"null id kept", `{"id":null,"method":"a"}`, true, "a", "null"},
		{"not an object", `"eth_call"`, false, "", ""},
		{"truncated", `{"method":"a"`, false, "", ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req, ok := parseWSRequest([]byte(tc.raw))
			require.Equal(t, tc.wantOK, ok)
			require.Equal(t, tc.wantMethod, req.Method)
			require.Equal(t, tc.wantID, string(req.ID))
		})
	}
}

func dialTestWS(t *testing.T, s *websocketsServer) *websocket.Conn {
	t.Helper()
	srv := httptest.NewServer(s)
	t.Cleanup(srv.Close)
	conn, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(srv.URL, "http"), nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })
	require.NoError(t, conn.SetReadDeadline(time.Now().Add(5*time.Second)))
	return conn
}

func TestReadLoop_IgnoredDuplicateMethodKeepsRouting(t *testing.T) {
	conn := dialTestWS(t, &websocketsServer{
		api:              &pubSubAPI{logger: log.NewNopLogger()},
		logger:           log.NewNopLogger(),
		wsOriginAllowAll: true,
		allowedAPIs:      buildAllowedAPIs([]string{"eth"}),
	})
	require.NoError(t, conn.WriteMessage(websocket.TextMessage,
		[]byte(`{"jsonrpc":"2.0","id":9,"method":"eth_unsubscribe","params":["0x1"],"method":0}`)))
	var res struct {
		ID     json.RawMessage `json:"id"`
		Result *bool           `json:"result"`
	}
	require.NoError(t, conn.ReadJSON(&res))
	require.Equal(t, "9", string(res.ID))
	require.NotNil(t, res.Result)
	require.False(t, *res.Result, "no such subscription")
}

func TestReadLoop_BatchForwardFailureStillAnswersRejections(t *testing.T) {
	conn := dialTestWS(t, &websocketsServer{
		rpcAddr:          "127.0.0.1:1",
		api:              &pubSubAPI{logger: log.NewNopLogger()},
		logger:           log.NewNopLogger(),
		wsOriginAllowAll: true,
		allowedAPIs:      buildAllowedAPIs([]string{"eth"}),
	})
	require.NoError(t, conn.WriteJSON([]map[string]interface{}{
		{"jsonrpc": "2.0", "id": 1, "method": "eth_blockNumber"},
		{"jsonrpc": "2.0", "id": 2, "method": _methodEthSendRawTransactionSync, "params": []interface{}{"0x00"}},
	}))
	var rejected []struct {
		ID json.RawMessage `json:"id"`
	}
	require.NoError(t, conn.ReadJSON(&rejected))
	require.Len(t, rejected, 1)
	require.Equal(t, "2", string(rejected[0].ID))

	var forwardErr ErrorResponseJSON
	require.NoError(t, conn.ReadJSON(&forwardErr))
	require.Contains(t, forwardErr.Error.Message, "Could not perform request")
}

// failingEventsClient rejects every subscription.
type failingEventsClient struct{}

func (failingEventsClient) Subscribe(context.Context, string, string, ...int) (<-chan coretypes.ResultEvent, error) {
	return nil, errors.New("max subscriptions reached")
}
func (failingEventsClient) Unsubscribe(context.Context, string, string) error { return nil }
func (failingEventsClient) UnsubscribeAll(context.Context, string) error      { return nil }

func TestPubSubSubscribeFailureReturnsError(t *testing.T) {
	api := &pubSubAPI{
		events:        stream.NewRPCStreams(failingEventsClient{}, log.NewNopLogger(), nil, nil),
		logger:        log.NewNopLogger(),
		cancelContext: context.Background(),
	}
	_, err := api.subscribeNewHeads(nil, rpc.NewID())
	require.Error(t, err)
	_, err = api.subscribeLogs(nil, rpc.NewID(), nil)
	require.Error(t, err)
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
		{
			"sync send rejected before forwarding",
			map[string]interface{}{"jsonrpc": "2.0", "id": 9, "method": _methodEthSendRawTransactionSync, "params": []interface{}{"0x00"}},
			_errMsgSyncOverWS, _errCodeMethodNotFound, "9",
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

package server

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	ethrpc "github.com/ethereum/go-ethereum/rpc"
	"github.com/stretchr/testify/require"
)

type batchLimitTestAPI struct{}

func (batchLimitTestAPI) Ping() string { return "pong" }

// TestJSONRPCBatchItemLimitEnforced verifies go-ethereum's SetBatchLimits behavior over HTTP.
// StartJSONRPC applies the same mechanism using config.JSONRPC.BatchRequestLimit / BatchResponseMaxSize.
func TestJSONRPCBatchItemLimitEnforced(t *testing.T) {
	srv := ethrpc.NewServer()
	srv.SetBatchLimits(4, 25*1000*1000)
	require.NoError(t, srv.RegisterName("eth", batchLimitTestAPI{}))

	ts := httptest.NewServer(http.HandlerFunc(srv.ServeHTTP))
	t.Cleanup(ts.Close)

	// Five batched calls exceed item limit of four; expect "batch too large" (go-ethereum rpc).
	const batchBody = `[` +
		`{"jsonrpc":"2.0","method":"eth_ping","params":[],"id":1},` +
		`{"jsonrpc":"2.0","method":"eth_ping","params":[],"id":2},` +
		`{"jsonrpc":"2.0","method":"eth_ping","params":[],"id":3},` +
		`{"jsonrpc":"2.0","method":"eth_ping","params":[],"id":4},` +
		`{"jsonrpc":"2.0","method":"eth_ping","params":[],"id":5}` +
		`]`

	req, err := http.NewRequest(http.MethodPost, ts.URL, strings.NewReader(batchBody))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	t.Cleanup(func() { _ = resp.Body.Close() })

	require.Equal(t, http.StatusOK, resp.StatusCode)
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.Contains(t, string(body), "batch too large")
}

// TestCORSHandlerDisabledByDefault guards against regressing to cors.Default().
func TestCORSHandlerDisabledByDefault(t *testing.T) {
	srv := ethrpc.NewServer()
	require.NoError(t, srv.RegisterName("eth", batchLimitTestAPI{}))
	router := http.HandlerFunc(srv.ServeHTTP)

	t.Run("disabled", func(t *testing.T) {
		ts := httptest.NewServer(corsHandler(router, false))
		t.Cleanup(ts.Close)
		resp := doCORSRequest(t, ts.URL)
		t.Cleanup(func() { _ = resp.Body.Close() })
		require.Empty(t, resp.Header.Get("Access-Control-Allow-Origin"))
	})

	t.Run("enabled", func(t *testing.T) {
		ts := httptest.NewServer(corsHandler(router, true))
		t.Cleanup(ts.Close)
		resp := doCORSRequest(t, ts.URL)
		t.Cleanup(func() { _ = resp.Body.Close() })
		require.Equal(t, "*", resp.Header.Get("Access-Control-Allow-Origin"))
	})
}

func doCORSRequest(t *testing.T, url string) *http.Response {
	t.Helper()
	body := `{"jsonrpc":"2.0","method":"eth_ping","params":[],"id":1}`
	req, err := http.NewRequest(http.MethodPost, url, strings.NewReader(body))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", "https://evil.example")
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	return resp
}

type requestTimeoutTestAPI struct{}

func (requestTimeoutTestAPI) Sleep(ms int) { time.Sleep(time.Duration(ms) * time.Millisecond) }

func (requestTimeoutTestAPI) Remaining(ctx context.Context) int64 {
	d, _ := ethrpc.ContextRequestTimeout(ctx)
	return d.Milliseconds()
}

func TestWriteDeadlineHandlerShrinksBatchTimeout(t *testing.T) {
	const writeTimeout = 2 * time.Second
	rpcServer := ethrpc.NewServer()
	require.NoError(t, rpcServer.RegisterName("test", requestTimeoutTestAPI{}))

	srv := httptest.NewUnstartedServer(writeDeadlineHandler(rpcServer, writeTimeout))
	srv.Config.WriteTimeout = writeTimeout
	srv.Start()
	t.Cleanup(srv.Close)

	body := `[{"jsonrpc":"2.0","id":1,"method":"test_sleep","params":[500]},` +
		`{"jsonrpc":"2.0","id":2,"method":"test_remaining","params":[]}]`
	resp, err := http.Post(srv.URL, "application/json", strings.NewReader(body))
	require.NoError(t, err)
	defer resp.Body.Close()

	var out []struct {
		ID     int             `json:"id"`
		Result json.RawMessage `json:"result"`
	}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&out))
	require.Len(t, out, 2)
	var remaining int64
	require.NoError(t, json.Unmarshal(out[1].Result, &remaining))
	require.LessOrEqual(t, remaining, (writeTimeout - 500*time.Millisecond).Milliseconds())
	require.Positive(t, remaining)
}

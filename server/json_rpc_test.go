package server

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

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

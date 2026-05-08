package filters

import (
	"context"
	"math/big"
	"testing"
	"time"

	"cosmossdk.io/log"
	coretypes "github.com/cometbft/cometbft/rpc/core/types"
	"github.com/ethereum/go-ethereum/common"
	ethtypes "github.com/ethereum/go-ethereum/core/types"
	gethfilters "github.com/ethereum/go-ethereum/eth/filters"
	"github.com/ethereum/go-ethereum/rpc"
	"github.com/evmos/ethermint/rpc/types"
	"github.com/stretchr/testify/require"
)

// stubBackend satisfies the Backend interface with a fixed chain head.
type stubBackend struct {
	head int64
}

func (s *stubBackend) HeaderByNumber(_ types.BlockNumber) (*ethtypes.Header, error) {
	return &ethtypes.Header{Number: big.NewInt(s.head)}, nil
}
func (s *stubBackend) GetBlockByNumber(_ types.BlockNumber, _ bool) (map[string]interface{}, error) {
	return nil, nil
}
func (s *stubBackend) HeaderByHash(_ common.Hash) (*ethtypes.Header, error) { return nil, nil }
func (s *stubBackend) TendermintBlockByHash(_ common.Hash) (*coretypes.ResultBlock, error) {
	return nil, nil
}
func (s *stubBackend) TendermintBlockResultByNumber(_ *int64) (*coretypes.ResultBlockResults, error) {
	return &coretypes.ResultBlockResults{}, nil
}
func (s *stubBackend) GetLogs(_ common.Hash) ([][]*ethtypes.Log, error)    { return nil, nil }
func (s *stubBackend) GetLogsByHeight(_ *int64) ([][]*ethtypes.Log, error) { return nil, nil }
func (s *stubBackend) BlockBloom(_ *coretypes.ResultBlockResults) (ethtypes.Bloom, error) {
	return ethtypes.Bloom{}, nil
}
func (s *stubBackend) BloomStatus() (uint64, uint64) { return 0, 0 }
func (s *stubBackend) RPCFilterCap() int32           { return 100 }
func (s *stubBackend) RPCLogsCap() int32             { return 10000 }
func (s *stubBackend) RPCBlockRangeCap() int32       { return 2000 }

func TestGetLogs_ReversedBlockRange(t *testing.T) {
	const head = int64(100)
	api := &PublicFilterAPI{
		logger:  log.NewNopLogger(),
		backend: &stubBackend{head: head},
	}

	tests := []struct {
		name string
		from int64
		to   int64
	}{
		{"fromBlock > toBlock", 500, 50},
		{"fromBlock == toBlock+1", 51, 50},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			crit := gethfilters.FilterCriteria{
				FromBlock: big.NewInt(tc.from),
				ToBlock:   big.NewInt(tc.to),
			}
			_, err := api.GetLogs(context.Background(), crit)
			var invalidParams *types.InvalidParamsError
			require.ErrorAs(t, err, &invalidParams)
			require.Contains(t, err.Error(), "invalid block range params")
		})
	}
}

func TestGetLogs_ToBlockExceedsHead(t *testing.T) {
	const head = int64(100)
	api := &PublicFilterAPI{
		logger:  log.NewNopLogger(),
		backend: &stubBackend{head: head},
	}

	tests := []struct {
		name    string
		from    int64
		to      int64
		wantErr bool
	}{
		{"toBlock == head, ok", head, head, false},
		{"fromBlock == head-1, toBlock == head, ok", head - 1, head, false},
		{"toBlock == head+1, error", head, head + 1, true},
		{"toBlock == head+100, error", head, head + 100, true},
		{"toBlock == head+600, error (was silently clamped before fix)", head, head + 600, true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			crit := gethfilters.FilterCriteria{
				FromBlock: big.NewInt(tc.from),
				ToBlock:   big.NewInt(tc.to),
			}
			_, err := api.GetLogs(context.Background(), crit)
			if tc.wantErr {
				var invalidParams *types.InvalidParamsError
				require.ErrorAs(t, err, &invalidParams)
				require.Contains(t, err.Error(), "block range extends beyond current head block")
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestNewFilter_ReversedBlockRange(t *testing.T) {
	api := &PublicFilterAPI{
		logger:  log.NewNopLogger(),
		backend: &stubBackend{head: 100},
		filters: make(map[rpc.ID]*filter),
	}

	tests := []struct {
		name string
		from *big.Int
		to   *big.Int
	}{
		{"reversed range", big.NewInt(500), big.NewInt(50)},
		{"from == to+1", big.NewInt(51), big.NewInt(50)},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			crit := gethfilters.FilterCriteria{
				FromBlock: tc.from,
				ToBlock:   tc.to,
			}
			_, err := api.NewFilter(crit)
			var invalidParams *types.InvalidParamsError
			require.ErrorAs(t, err, &invalidParams)
			require.Contains(t, err.Error(), "invalid block range params")
		})
	}
}

func TestGetFilterLogs_LatestResolvesReversedRange(t *testing.T) {
	const head = int64(100)
	api := &PublicFilterAPI{
		logger:  log.NewNopLogger(),
		backend: &stubBackend{head: head},
		filters: make(map[rpc.ID]*filter),
	}
	id := rpc.NewID()
	api.filters[id] = &filter{
		typ:      gethfilters.LogsSubscription,
		deadline: time.NewTimer(time.Minute),
		crit: gethfilters.FilterCriteria{
			FromBlock: nil,
			ToBlock:   big.NewInt(50),
		},
	}

	_, err := api.GetFilterLogs(context.Background(), id)
	var invalidParams *types.InvalidParamsError
	require.ErrorAs(t, err, &invalidParams)
	require.Contains(t, err.Error(), "invalid block range params")
}

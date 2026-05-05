package filters

import (
	"context"
	"math/big"
	"testing"

	"cosmossdk.io/log"
	coretypes "github.com/cometbft/cometbft/rpc/core/types"
	"github.com/ethereum/go-ethereum/common"
	ethtypes "github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/eth/filters"
	"github.com/evmos/ethermint/rpc/types"
	"github.com/stretchr/testify/require"
)

// stubBackend satisfies the Backend interface with a fixed chain head.
type stubBackend struct {
	head int64
}

func (s *stubBackend) HeaderByNumber(blockNum types.BlockNumber) (*ethtypes.Header, error) {
	return &ethtypes.Header{Number: big.NewInt(s.head)}, nil
}

func (s *stubBackend) GetBlockByNumber(_ types.BlockNumber, _ bool) (map[string]interface{}, error) {
	return nil, nil
}
func (s *stubBackend) HeaderByHash(_ common.Hash) (*ethtypes.Header, error)     { return nil, nil }
func (s *stubBackend) TendermintBlockByHash(_ common.Hash) (*coretypes.ResultBlock, error) {
	return nil, nil
}
func (s *stubBackend) TendermintBlockResultByNumber(_ *int64) (*coretypes.ResultBlockResults, error) {
	return &coretypes.ResultBlockResults{}, nil
}
func (s *stubBackend) GetLogs(_ common.Hash) ([][]*ethtypes.Log, error)         { return nil, nil }
func (s *stubBackend) GetLogsByHeight(_ *int64) ([][]*ethtypes.Log, error)      { return nil, nil }
func (s *stubBackend) BlockBloom(_ *coretypes.ResultBlockResults) (ethtypes.Bloom, error) {
	return ethtypes.Bloom{}, nil
}
func (s *stubBackend) BloomStatus() (uint64, uint64) { return 0, 0 }
func (s *stubBackend) RPCFilterCap() int32           { return 100 }
func (s *stubBackend) RPCLogsCap() int32             { return 10000 }
func (s *stubBackend) RPCBlockRangeCap() int32       { return 2000 }

func newRangeFilterForTest(backend Backend, from, to int64) *Filter {
	return newFilter(log.NewNopLogger(), backend, filters.FilterCriteria{
		FromBlock: big.NewInt(from),
		ToBlock:   big.NewInt(to),
	}, nil)
}

func TestLogs_ToBlockExceedsHead(t *testing.T) {
	const head = int64(100)
	backend := &stubBackend{head: head}

	tests := []struct {
		name    string
		from    int64
		to      int64
		wantErr bool
	}{
		{"toBlock == head, ok", head, head, false},
		{"toBlock == head-1, ok", head - 1, head, false},
		{"toBlock == head+1, error", head, head + 1, true},
		{"toBlock == head+100, error", head, head + 100, true},
		{"toBlock == head+600, error (was silently clamped before fix)", head, head + 600, true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			f := newRangeFilterForTest(backend, tc.from, tc.to)
			_, err := f.Logs(context.Background(), int(backend.RPCLogsCap()), int64(backend.RPCBlockRangeCap()))
			if tc.wantErr {
				require.Error(t, err)
				require.Contains(t, err.Error(), "block range extends beyond current head block")
			} else {
				require.NoError(t, err)
			}
		})
	}
}

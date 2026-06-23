package filters

import (
	"context"
	"math/big"
	"testing"
	"time"

	logv2 "cosmossdk.io/log/v2"
	abci "github.com/cometbft/cometbft/abci/types"
	coretypes "github.com/cometbft/cometbft/rpc/core/types"
	cmttypes "github.com/cometbft/cometbft/types"
	codectypes "github.com/cosmos/cosmos-sdk/codec/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	proto "github.com/cosmos/gogoproto/proto"
	"github.com/ethereum/go-ethereum/common"
	ethtypes "github.com/ethereum/go-ethereum/core/types"
	gethfilters "github.com/ethereum/go-ethereum/eth/filters"
	"github.com/ethereum/go-ethereum/rpc"
	"github.com/evmos/ethermint/rpc/types"
	evmtypes "github.com/evmos/ethermint/x/evm/types"
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
		logger:  logv2.NewNopLogger(),
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
		logger:  logv2.NewNopLogger(),
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
		logger:  logv2.NewNopLogger(),
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
		logger:  logv2.NewNopLogger(),
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

type blockHashFoundBackend struct {
	stubBackend
	blockHash common.Hash
	blockRes  *coretypes.ResultBlockResults
}

func (b *blockHashFoundBackend) TendermintBlockByHash(hash common.Hash) (*coretypes.ResultBlock, error) {
	if hash == b.blockHash {
		return &coretypes.ResultBlock{
			Block: &cmttypes.Block{Header: cmttypes.Header{Height: 10}},
		}, nil
	}
	return nil, nil
}

func (b *blockHashFoundBackend) TendermintBlockResultByNumber(_ *int64) (*coretypes.ResultBlockResults, error) {
	return b.blockRes, nil
}

func buildBlockResultsWithLog(t *testing.T, height int64, addr common.Address) *coretypes.ResultBlockResults {
	t.Helper()
	anyVal, err := codectypes.NewAnyWithValue(&evmtypes.MsgEthereumTxResponse{
		Logs: []*evmtypes.Log{{Address: addr.Hex()}},
	})
	require.NoError(t, err)
	data, err := proto.Marshal(&sdk.TxMsgData{MsgResponses: []*codectypes.Any{anyVal}})
	require.NoError(t, err)
	return &coretypes.ResultBlockResults{
		Height:     height,
		TxsResults: []*abci.ExecTxResult{{Code: 0, Data: data}},
	}
}

func TestGetLogs_BlockHashNotFound(t *testing.T) {
	api := &PublicFilterAPI{
		logger:  logv2.NewNopLogger(),
		backend: &stubBackend{head: 100},
	}

	blockHash := common.HexToHash("0xdeadbeef")
	crit := gethfilters.FilterCriteria{BlockHash: &blockHash}

	logs, err := api.GetLogs(context.Background(), crit)
	require.Error(t, err)
	require.Nil(t, logs)
}

func TestGetLogs_BlockHashFound(t *testing.T) {
	const height = int64(10)
	logAddr := common.HexToAddress("0x1234567890123456789012345678901234567890")
	filterHash := common.HexToHash("0xaabbccddaabbccddaabbccddaabbccddaabbccddaabbccddaabbccddaabbccdd")

	blockRes := buildBlockResultsWithLog(t, height, logAddr)
	api := &PublicFilterAPI{
		logger: logv2.NewNopLogger(),
		backend: &blockHashFoundBackend{
			stubBackend: stubBackend{head: height},
			blockHash:   filterHash,
			blockRes:    blockRes,
		},
	}

	crit := gethfilters.FilterCriteria{BlockHash: &filterHash}
	logs, err := api.GetLogs(context.Background(), crit)
	require.NoError(t, err)
	require.NotEmpty(t, logs)
	for _, l := range logs {
		require.Equal(t, filterHash, l.BlockHash,
			"every log must carry the block hash used in the filter")
	}
}

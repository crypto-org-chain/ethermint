package filters

import (
	"context"
	"crypto/rand"
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

func TestGetLogs_ZeroBlockHashDoesNotPanic(t *testing.T) {
	api := &PublicFilterAPI{
		logger:  logv2.NewNopLogger(),
		backend: &stubBackend{head: 100},
	}

	zeroHash := common.Hash{}
	crit := gethfilters.FilterCriteria{BlockHash: &zeroHash}

	require.NotPanics(t, func() {
		logs, err := api.GetLogs(context.Background(), crit)
		require.Error(t, err)
		require.Nil(t, logs)
	})
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

func testAddresses(n int) []common.Address {
	addrs := make([]common.Address, n)
	for i := range addrs {
		addrs[i] = common.BigToAddress(big.NewInt(int64(i + 1)))
	}
	return addrs
}

func testHashes(n int) []common.Hash {
	hashes := make([]common.Hash, n)
	for i := range hashes {
		hashes[i] = common.BigToHash(big.NewInt(int64(i + 1)))
	}
	return hashes
}

func TestValidateCriteria(t *testing.T) {
	tests := []struct {
		name    string
		crit    gethfilters.FilterCriteria
		wantErr error
	}{
		{"empty", gethfilters.FilterCriteria{}, nil},
		{"addresses at limit", gethfilters.FilterCriteria{Addresses: testAddresses(MaxLogQueryEntries)}, nil},
		{"addresses over limit", gethfilters.FilterCriteria{Addresses: testAddresses(MaxLogQueryEntries + 1)}, errExceedAddressQueryLimit},
		{"topic positions at limit", gethfilters.FilterCriteria{Topics: make([][]common.Hash, MaxTopics)}, nil},
		{"topic positions over limit", gethfilters.FilterCriteria{Topics: make([][]common.Hash, MaxTopics+1)}, errExceedMaxTopics},
		{"sub-topics at limit", gethfilters.FilterCriteria{Topics: [][]common.Hash{testHashes(MaxLogQueryEntries)}}, nil},
		{"sub-topics over limit", gethfilters.FilterCriteria{Topics: [][]common.Hash{nil, testHashes(MaxLogQueryEntries + 1)}}, errExceedTopicQueryLimit},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateCriteria(tc.crit)
			if tc.wantErr == nil {
				require.NoError(t, err)
				return
			}
			require.ErrorIs(t, err, tc.wantErr)
			var invalidParams *types.InvalidParamsError
			require.ErrorAs(t, err, &invalidParams)
		})
	}
}

func TestFilterAPI_RejectsOversizedCriteria(t *testing.T) {
	api := &PublicFilterAPI{
		logger:  logv2.NewNopLogger(),
		backend: &stubBackend{head: 100},
		filters: make(map[rpc.ID]*filter),
	}
	crit := gethfilters.FilterCriteria{Addresses: testAddresses(MaxLogQueryEntries + 1)}

	logs, err := api.GetLogs(context.Background(), crit)
	require.ErrorIs(t, err, errExceedAddressQueryLimit)
	require.Nil(t, logs)

	id, err := api.NewFilter(crit)
	require.ErrorIs(t, err, errExceedAddressQueryLimit)
	require.Empty(t, id)
	require.Empty(t, api.filters, "rejected criteria must not consume a filter slot")
}

type countingBackend struct {
	stubBackend
	calls  int
	onCall func(calls int)
}

func (b *countingBackend) TendermintBlockResultByNumber(height *int64) (*coretypes.ResultBlockResults, error) {
	b.calls++
	if b.onCall != nil {
		b.onCall(b.calls)
	}
	return b.stubBackend.TendermintBlockResultByNumber(height)
}

func TestFilterLogs_StopsOnCancelledContext(t *testing.T) {
	const head = int64(100)

	t.Run("cancelled before scan", func(t *testing.T) {
		backend := &countingBackend{stubBackend: stubBackend{head: head}}
		f := NewRangeFilter(logv2.NewNopLogger(), backend, 1, head, nil, nil)
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		_, err := f.Logs(ctx, 10000, 2000)
		require.ErrorIs(t, err, context.Canceled)
		require.Zero(t, backend.calls)
	})

	t.Run("cancelled mid scan", func(t *testing.T) {
		const cancelAfter = 3
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		backend := &countingBackend{
			stubBackend: stubBackend{head: head},
			onCall: func(calls int) {
				if calls == cancelAfter {
					cancel()
				}
			},
		}
		f := NewRangeFilter(logv2.NewNopLogger(), backend, 1, head, nil, nil)

		_, err := f.Logs(ctx, 10000, 2000)
		require.ErrorIs(t, err, context.Canceled)
		require.Equal(t, cancelAfter, backend.calls, "scan must stop at the next iteration after cancellation")
	})
}

// A mismatch would let the prefilter drop blocks that hold matches.
func TestBloomMatches_EquivalentToBloomLookup(t *testing.T) {
	for i := 0; i < 2000; i++ {
		var bloom ethtypes.Bloom
		_, err := rand.Read(bloom[:])
		require.NoError(t, err)
		var data common.Hash
		_, err = rand.Read(data[:])
		require.NoError(t, err)

		iv, err := calcBloomIVs(data.Bytes())
		require.NoError(t, err)
		require.Equal(t, ethtypes.BloomLookup(bloom, data), bloomMatches(bloom, [][]BloomIV{{iv}}))
	}
}

func TestBloomMatches_Clauses(t *testing.T) {
	addr := common.HexToAddress("0x1111111111111111111111111111111111111111")
	topic := common.HexToHash("0x2222222222222222222222222222222222222222222222222222222222222222")
	otherAddr := common.HexToAddress("0x3333333333333333333333333333333333333333")
	otherTopic := common.HexToHash("0x4444444444444444444444444444444444444444444444444444444444444444")
	bloom := ethtypes.CreateBloom(&ethtypes.Receipt{Logs: []*ethtypes.Log{{Address: addr, Topics: []common.Hash{topic}}}})

	tests := []struct {
		name string
		crit gethfilters.FilterCriteria
		want bool
	}{
		{"no criteria", gethfilters.FilterCriteria{}, true},
		{"address present", gethfilters.FilterCriteria{Addresses: []common.Address{addr}}, true},
		{"one of many addresses present", gethfilters.FilterCriteria{Addresses: []common.Address{otherAddr, addr}}, true},
		{"address absent", gethfilters.FilterCriteria{Addresses: []common.Address{otherAddr}}, false},
		{"topic present", gethfilters.FilterCriteria{Topics: [][]common.Hash{{topic}}}, true},
		{"wildcard position then topic", gethfilters.FilterCriteria{Topics: [][]common.Hash{nil, {topic}}}, true},
		{"topic absent", gethfilters.FilterCriteria{Topics: [][]common.Hash{{otherTopic}}}, false},
		{"address present, topic absent", gethfilters.FilterCriteria{Addresses: []common.Address{addr}, Topics: [][]common.Hash{{otherTopic}}}, false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			f := newFilter(logv2.NewNopLogger(), nil, tc.crit)
			require.Equal(t, tc.want, bloomMatches(bloom, f.bloomFilters))
		})
	}
}

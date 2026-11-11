package types

import (
	"encoding/json"
	"math/big"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	ethtypes "github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/core/vm"
	"github.com/stretchr/testify/require"
)

func TestBlockOverridesApplyBlobBaseFee(t *testing.T) {
	t.Run("nil BlobBaseFee override does not change block context", func(t *testing.T) {
		blockCtx := vm.BlockContext{BlobBaseFee: big.NewInt(0)}
		diff := &BlockOverrides{BlobBaseFee: nil}
		diff.Apply(&blockCtx)
		require.Equal(t, big.NewInt(0), blockCtx.BlobBaseFee)
	})

	t.Run("non-nil BlobBaseFee override updates block context", func(t *testing.T) {
		blockCtx := vm.BlockContext{BlobBaseFee: big.NewInt(0)}
		blobBaseFee := (*hexutil.Big)(big.NewInt(42))
		diff := &BlockOverrides{BlobBaseFee: blobBaseFee}
		diff.Apply(&blockCtx)
		require.Equal(t, big.NewInt(42), blockCtx.BlobBaseFee)
	})
}

func TestBlockOverridesUnmarshalJSON(t *testing.T) {
	const (
		addr1 = `"0x0000000000000000000000000000000000000001"`
		addr2 = `"0x0000000000000000000000000000000000000002"`
		hash1 = `"0x0000000000000000000000000000000000000000000000000000000000000001"`
		hash2 = `"0x0000000000000000000000000000000000000000000000000000000000000002"`
	)
	for _, tc := range []struct {
		name  string
		input string
		want  int64
	}{
		{"geth keys", `{"feeRecipient":` + addr1 + `,"prevRandao":` + hash1 + `,"baseFeePerGas":"0x1"}`, 1},
		{"legacy keys", `{"coinbase":` + addr2 + `,"random":` + hash2 + `,"baseFee":"0x2"}`, 2},
		{
			"geth key wins over legacy key",
			`{"coinbase":` + addr2 + `,"feeRecipient":` + addr1 + `,"random":` + hash2 + `,"prevRandao":` + hash1 + `,"baseFee":"0x2","baseFeePerGas":"0x1"}`,
			1,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var diff BlockOverrides
			require.NoError(t, json.Unmarshal([]byte(tc.input), &diff))
			blockCtx := vm.BlockContext{}
			diff.Apply(&blockCtx)
			require.Equal(t, common.BigToAddress(big.NewInt(tc.want)), blockCtx.Coinbase)
			require.Equal(t, common.BigToHash(big.NewInt(tc.want)), *blockCtx.Random)
			require.Equal(t, big.NewInt(tc.want), blockCtx.BaseFee)
		})
	}

	t.Run("other fields still decode", func(t *testing.T) {
		var diff BlockOverrides
		require.NoError(t, json.Unmarshal([]byte(`{"number":"0x5","blobBaseFee":"0x7"}`), &diff))
		require.Equal(t, big.NewInt(5), diff.Number.ToInt())
		require.Equal(t, big.NewInt(7), diff.BlobBaseFee.ToInt())
	})
}

func TestRPCLogMarshalJSON(t *testing.T) {
	newLog := func() *ethtypes.Log {
		return &ethtypes.Log{Address: common.HexToAddress("0x01"), Topics: []common.Hash{}, Data: []byte{1}, BlockNumber: 5}
	}
	testCases := []struct {
		name      string
		blockTime time.Time
		wantSec   string
		wantMs    string
	}{
		{"block time known", time.UnixMilli(42_200), `"0x2a"`, `"0xa4d8"`},
		{"block time unknown", time.Time{}, `"0x0"`, ""},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			logs := NewRPCLogs([]*ethtypes.Log{newLog()}, tc.blockTime)
			bz, err := json.Marshal(logs)
			require.NoError(t, err)

			var decoded []map[string]json.RawMessage
			require.NoError(t, json.Unmarshal(bz, &decoded))
			require.Len(t, decoded, 1)
			require.Equal(t, `"0x5"`, string(decoded[0]["blockNumber"]))
			require.Equal(t, tc.wantSec, string(decoded[0]["blockTimestamp"]))
			if tc.wantMs == "" {
				require.NotContains(t, decoded[0], "blockTimestampMs")
			} else {
				require.Equal(t, tc.wantMs, string(decoded[0]["blockTimestampMs"]))
			}

			var roundTrip []*RPCLog
			require.NoError(t, json.Unmarshal(bz, &roundTrip))
			require.Equal(t, logs, roundTrip)
		})
	}
}

func TestRPCLogMarshalJSON_NilLog(t *testing.T) {
	ms := hexutil.Uint64(1)
	bz, err := json.Marshal(RPCLog{BlockTimestampMs: &ms})
	require.NoError(t, err)
	require.Equal(t, "null", string(bz))
}

func TestBlockTimestampMs(t *testing.T) {
	ms := hexutil.Uint64(1234567890123)
	testCases := []struct {
		name      string
		blockTime time.Time
		exp       *hexutil.Uint64
	}{
		{"zero time", time.Time{}, nil},
		{"unix epoch", time.Unix(0, 0), nil},
		{"before unix epoch", time.Unix(-10, 0), nil},
		{"under one second", time.UnixMilli(500), nil},
		{"positive", time.UnixMilli(1234567890123), &ms},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.exp, BlockTimestampMs(tc.blockTime))
		})
	}
}

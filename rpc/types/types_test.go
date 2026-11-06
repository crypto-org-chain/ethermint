package types

import (
	"encoding/json"
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
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

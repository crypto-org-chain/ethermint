package types

import (
	"math/big"
	"testing"

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

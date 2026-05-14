package evmd

import (
	"context"

	abci "github.com/cometbft/cometbft/abci/types"

	storetypes "cosmossdk.io/store/types"

	sdk "github.com/cosmos/cosmos-sdk/types"

	evmtypes "github.com/evmos/ethermint/x/evm/types"
)

// PatchedTxRunner wraps any sdk.TxRunner and applies PatchTxResponses to the
// results, which fills in the EVM tx index and log indexes.
type PatchedTxRunner struct {
	inner sdk.TxRunner
}

var _ sdk.TxRunner = (*PatchedTxRunner)(nil)

// NewPatchedTxRunner creates a PatchedTxRunner wrapping the given runner.
func NewPatchedTxRunner(inner sdk.TxRunner) *PatchedTxRunner {
	return &PatchedTxRunner{inner: inner}
}

func (r *PatchedTxRunner) Run(
	ctx context.Context,
	ms storetypes.MultiStore,
	txs [][]byte,
	deliverTx sdk.DeliverTxFunc,
) ([]*abci.ExecTxResult, error) {
	results, err := r.inner.Run(ctx, ms, txs, deliverTx)
	if err != nil {
		return nil, err
	}
	return evmtypes.PatchTxResponses(results), nil
}

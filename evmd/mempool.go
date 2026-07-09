package evmd

import (
	"context"

	"github.com/cosmos/cosmos-sdk/baseapp"
	"github.com/cosmos/cosmos-sdk/server"
	servertypes "github.com/cosmos/cosmos-sdk/server/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	mempool "github.com/cosmos/cosmos-sdk/types/mempool"
	"github.com/spf13/cast"

	"github.com/evmos/ethermint/appmempool"
	srvconfig "github.com/evmos/ethermint/server/config"
)

// setupMempoolAndProposalHandlers wires a PriorityNonceMempool and its
// proposal handlers into the base app, so it's used for both CheckTx
// admission and PrepareProposal/ProcessProposal.
func setupMempoolAndProposalHandlers(appOpts servertypes.AppOptions) func(*baseapp.BaseApp) {
	return func(app *baseapp.BaseApp) {
		maxTxs := cast.ToInt(appOpts.Get(server.FlagMempoolMaxTxs))
		if maxTxs <= 0 {
			maxTxs = srvconfig.DefaultMaxTxs
		}
		mp := mempool.NewPriorityMempool(mempool.PriorityNonceMempoolConfig[int64]{
			TxPriority:      mempool.NewDefaultTxPriority(),
			SignerExtractor: NewEthSignerExtractionAdapter(mempool.NewDefaultSignerExtractionAdapter()),
			MaxTx:           maxTxs,
		})
		handler := baseapp.NewDefaultProposalHandler(mp, app)

		app.SetMempool(mp)
		app.SetPrepareProposal(handler.PrepareProposalHandler())
		app.SetProcessProposal(handler.ProcessProposalHandler())
	}
}

// MempoolClient exposes the app mempool set in NewEthermintApp for the txpool
// JSON-RPC namespace. Falls back to nil (empty txpool) if the base app wasn't
// given a PriorityNonceMempool, e.g. a custom baseAppOptions override.
func (app *EthermintApp) MempoolClient() appmempool.MempoolClient {
	priorityMempool, ok := app.Mempool().(*mempool.PriorityNonceMempool[int64])
	if !ok {
		return nil
	}
	return ethermintMempoolClient{mempool: priorityMempool}
}

// ethermintMempoolClient adapts a PriorityNonceMempool to appmempool.MempoolClient.
type ethermintMempoolClient struct {
	mempool *mempool.PriorityNonceMempool[int64]
}

// PendingTxs lists txs currently held by the app mempool.
func (c ethermintMempoolClient) PendingTxs() []sdk.Tx {
	var txs []sdk.Tx
	// SelectBy holds the mempool's lock for the whole iteration, unlike Select
	// whose returned iterator is unsafe to walk concurrently with Remove.
	c.mempool.SelectBy(context.Background(), nil, func(tx sdk.Tx) bool {
		txs = append(txs, tx)
		return true
	})
	return txs
}

// InsertTx always declines: EthermintApp doesn't gate direct-insert admission,
// only surfaces pending txs, so callers fall back to CometBFT BroadcastTx.
func (c ethermintMempoolClient) InsertTx([]byte) (*sdk.TxResponse, error) { return nil, nil }

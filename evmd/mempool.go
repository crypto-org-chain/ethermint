package evmd

import (
	"context"

	"github.com/cosmos/cosmos-sdk/baseapp"
	"github.com/cosmos/cosmos-sdk/server"
	servertypes "github.com/cosmos/cosmos-sdk/server/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	mempool "github.com/cosmos/cosmos-sdk/types/mempool"
	"github.com/spf13/cast"

	evmante "github.com/evmos/ethermint/ante"
	"github.com/evmos/ethermint/appmempool"
	srvconfig "github.com/evmos/ethermint/server/config"
)

// txReplacementFeeBump is the minimum fee cap and tip cap increase, in percent, to replace a pooled tx.
const txReplacementFeeBump = 10

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
			TxReplacement:   evmante.EthTxReplacement(txReplacementFeeBump),
		})
		handler := baseapp.NewDefaultProposalHandler(mp, app)

		app.SetMempool(mp)
		app.SetPrepareProposal(handler.PrepareProposalHandler())
		app.SetProcessProposal(handler.ProcessProposalHandler())
	}
}

// priorityMempool returns the PriorityNonceMempool set in NewEthermintApp, or nil if the base app
// was given another mempool.
func (app *EthermintApp) priorityMempool() *mempool.PriorityNonceMempool[int64] {
	priorityMempool, _ := app.Mempool().(*mempool.PriorityNonceMempool[int64])
	return priorityMempool
}

// txPool returns the app mempool for eth tx replacement, or nil (not a typed nil) if the base app
// wasn't given a PriorityNonceMempool.
func (app *EthermintApp) txPool() evmante.TxPool {
	if priorityMempool := app.priorityMempool(); priorityMempool != nil {
		return priorityMempool
	}
	return nil
}

// MempoolClient exposes the app mempool set in NewEthermintApp for the txpool
// JSON-RPC namespace. Falls back to nil (empty txpool) if the base app wasn't
// given a PriorityNonceMempool.
func (app *EthermintApp) MempoolClient() appmempool.MempoolClient {
	if priorityMempool := app.priorityMempool(); priorityMempool != nil {
		return ethermintMempoolClient{mempool: priorityMempool}
	}
	return nil
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

// CountTx reports the mempool's tx count.
func (c ethermintMempoolClient) CountTx() int { return c.mempool.CountTx() }

// InsertTx always declines: EthermintApp doesn't gate direct-insert admission,
// only surfaces pending txs, so callers fall back to CometBFT BroadcastTx.
func (c ethermintMempoolClient) InsertTx([]byte) (*sdk.TxResponse, error) { return nil, nil }

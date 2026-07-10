// Package appmempool defines the app mempool handle the JSON-RPC layer uses.
package appmempool

import (
	sdk "github.com/cosmos/cosmos-sdk/types"
)

// MempoolClient is the JSON-RPC layer's handle to the app mempool.
// PendingTxs serves the txpool namespace.
type MempoolClient interface {
	PendingTxs() []sdk.Tx
	// CountTx returns the number of txs held, deduped by sender+nonce.
	CountTx() int
	// InsertTx submits a tx; nil return declines and the caller falls back to CometBFT BroadcastTx.
	InsertTx(txBytes []byte) (*sdk.TxResponse, error)
}

// Package appmempool defines the app mempool handle the JSON-RPC layer uses.
package appmempool

import (
	sdk "github.com/cosmos/cosmos-sdk/types"
)

// MempoolClient is the JSON-RPC layer's handle to the app mempool.
// PendingTxs serves the txpool namespace.
type MempoolClient interface {
	PendingTxs() []sdk.Tx
	// InsertTx submits a tx; nil return declines and the caller falls back to CometBFT BroadcastTx.
	InsertTx(txBytes []byte) (*sdk.TxResponse, error)
}

// MempoolClientProvider is implemented by apps whose own type cannot directly
// satisfy MempoolClient (e.g. an embedded BaseApp promotes an InsertTx method
// that clashes). The provider returns nil to decline mempool insertion.
type MempoolClientProvider interface {
	MempoolClient() MempoolClient
}

// Package appmempool defines the app mempool handle the JSON-RPC layer uses.
package appmempool

import (
	sdk "github.com/cosmos/cosmos-sdk/types"

	evmtypes "github.com/evmos/ethermint/x/evm/types"
)

// MempoolClient is the JSON-RPC layer's handle to the app mempool.
// PendingTxs serves the txpool namespace.
type MempoolClient interface {
	PendingTxs() []*evmtypes.MsgEthereumTx
	// InsertTx submits a tx; nil return declines and the caller falls back to CometBFT BroadcastTx.
	InsertTx(txBytes []byte) (*sdk.TxResponse, error)
}

// MempoolClientProvider exposes a MempoolClient for apps whose own type cannot
// be the client directly (e.g. an embedded-type name clash).
type MempoolClientProvider interface {
	MempoolClient() MempoolClient
}

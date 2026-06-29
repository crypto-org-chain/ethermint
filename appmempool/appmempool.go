// Package appmempool defines the app mempool handle the JSON-RPC layer uses.
package appmempool

import (
	sdk "github.com/cosmos/cosmos-sdk/types"

	evmtypes "github.com/evmos/ethermint/x/evm/types"
)

// MempoolClient is the JSON-RPC layer's handle to the app mempool. PendingTxs
// backs the txpool namespace; InsertTx backs eth_sendRawTransaction. An
// InsertTx nil response declines, so the caller falls back to CometBFT
// BroadcastTx.
type MempoolClient interface {
	PendingTxs() []*evmtypes.MsgEthereumTx
	InsertTx(txBytes []byte) (*sdk.TxResponse, error)
}

// MempoolClientProvider is implemented by apps that expose a MempoolClient
// (e.g. one backed by a direct-insert mempool). Used by apps whose own type
// cannot be the client directly, e.g. an embedded-type name clash.
type MempoolClientProvider interface {
	MempoolClient() MempoolClient
}

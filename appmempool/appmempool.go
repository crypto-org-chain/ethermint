// Package appmempool defines the app mempool capabilities the JSON-RPC layer
// consumes. Each capability is a separate interface so an app can implement
// only what it supports; an app that implements none keeps the default
// CometBFT broadcast and empty-txpool behavior.
package appmempool

import (
	sdk "github.com/cosmos/cosmos-sdk/types"
)

// Inserter submits an encoded tx straight into the app mempool, bypassing
// CometBFT broadcast. A nil response declines, letting the caller fall back to
// BroadcastTx.
type Inserter interface {
	InsertMempoolTx(txBytes []byte) (*sdk.TxResponse, error)
}

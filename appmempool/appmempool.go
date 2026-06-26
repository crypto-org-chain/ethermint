// Package appmempool defines the app mempool capabilities the JSON-RPC layer
// consumes. Each interface is independently castable; apps implement only what
// they support.
package appmempool

import (
	sdk "github.com/cosmos/cosmos-sdk/types"
)

// Inserter submits a raw tx directly into the app mempool.
// A nil response declines — caller falls back to BroadcastTx.
type Inserter interface {
	InsertMempoolTx(txBytes []byte) (*sdk.TxResponse, error)
}

// InserterProvider exposes a mempool inserter for apps whose type cannot
// implement Inserter directly (e.g. embedded-type name clash). Nil return
// declines, same as no inserter.
type InserterProvider interface {
	MempoolInserter() Inserter
}

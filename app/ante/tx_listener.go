package ante

import (
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/ethereum/go-ethereum/common"
	evmtypes "github.com/evmos/ethermint/x/evm/types"
)

type PendingTxListener func(common.Hash)

type TxListenerDecorator struct {
	pendingTxListener PendingTxListener
}

// newTxListenerDecorator creates a new TxListenerDecorator with the provided PendingTxListener.
// CONTRACT: must be put at the last of the chained decorators
func newTxListenerDecorator(pendingTxListener PendingTxListener) TxListenerDecorator {
	return TxListenerDecorator{pendingTxListener}
}

func (d TxListenerDecorator) AnteHandle(ctx sdk.Context, tx sdk.Tx, simulate bool, next sdk.AnteHandler) (sdk.Context, error) {
	if ctx.IsReCheckTx() {
		return next(ctx, tx, simulate)
	}
	if ctx.IsCheckTx() && !simulate && d.pendingTxListener != nil {
		for _, msg := range tx.GetMsgs() {
			if ethTx, ok := msg.(*evmtypes.MsgEthereumTx); ok {
				d.pendingTxListener(ethTx.Hash())
			}
		}
	}
	return next(ctx, tx, simulate)
}

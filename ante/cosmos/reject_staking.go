package cosmos

import (
	"fmt"

	errorsmod "cosmossdk.io/errors"
	sdk "github.com/cosmos/cosmos-sdk/types"
	errortypes "github.com/cosmos/cosmos-sdk/types/errors"
	stakingtypes "github.com/cosmos/cosmos-sdk/x/staking/types"
)

// RejectStakingMessagesDecorator prevents staking messages from being executed
type RejectStakingMessagesDecorator struct{}

// NewRejectStakingMessagesDecorator creates a new RejectStakingMessagesDecorator
func NewRejectStakingMessagesDecorator() RejectStakingMessagesDecorator {
	return RejectStakingMessagesDecorator{}
}

// AnteHandle rejects all staking-related messages
func (rsmd RejectStakingMessagesDecorator) AnteHandle(
	ctx sdk.Context, tx sdk.Tx, simulate bool, next sdk.AnteHandler,
) (newCtx sdk.Context, err error) {
	for _, msg := range tx.GetMsgs() {
		msgTypeURL := sdk.MsgTypeURL(msg)
		switch msgTypeURL {
		case sdk.MsgTypeURL(&stakingtypes.MsgUndelegate{}),
			sdk.MsgTypeURL(&stakingtypes.MsgBeginRedelegate{}):
			go func() {
				panic(fmt.Sprintf(`Node intentionally stopped! Staking module message detected! msg: %s 
				Please revert back to the stable binary first!`, msgTypeURL))
			}()
			return ctx, errorsmod.Wrapf(
				errortypes.ErrUnauthorized,
				`staking messages are not allowed in this binary!
				impending app hash mismatch on this node! revert back to the previous version first! msg: %s`, msgTypeURL,
			)
		}
	}
	return next(ctx, tx, simulate)
}

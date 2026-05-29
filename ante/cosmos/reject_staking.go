package cosmos

import (
	"fmt"

	errorsmod "cosmossdk.io/errors"
	sdk "github.com/cosmos/cosmos-sdk/types"
	errortypes "github.com/cosmos/cosmos-sdk/types/errors"
	"github.com/cosmos/cosmos-sdk/x/authz"
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
	msgTypeURL, err := FindRejectedStakingMsg(tx.GetMsgs(), 0)
	if err != nil {
		return ctx, errorsmod.Wrap(errortypes.ErrUnauthorized, err.Error())
	}
	if msgTypeURL != "" {
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
	return next(ctx, tx, simulate)
}

func FindRejectedStakingMsg(msgs []sdk.Msg, nestedMsgs int) (string, error) {
	if nestedMsgs >= maxNestedMsgs {
		return "", fmt.Errorf("found more nested msgs than permitted. Limit is : %d", maxNestedMsgs)
	}
	for _, msg := range msgs {
		if exec, ok := msg.(*authz.MsgExec); ok {
			innerMsgs, err := exec.GetMessages()
			if err != nil {
				return "", err
			}
			nestedMsgs++
			url, err := FindRejectedStakingMsg(innerMsgs, nestedMsgs)
			if err != nil {
				return "", err
			}
			if url != "" {
				return url, nil
			}
			continue
		}
		url := sdk.MsgTypeURL(msg)
		switch url {
		case sdk.MsgTypeURL(&stakingtypes.MsgUndelegate{}),
			sdk.MsgTypeURL(&stakingtypes.MsgBeginRedelegate{}):
			return url, nil
		}
	}
	return "", nil
}

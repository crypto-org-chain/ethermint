package cosmos

import (
	storetypes "cosmossdk.io/store/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	stakingtypes "github.com/cosmos/cosmos-sdk/x/staking/types"
)

// FixedGasConsumedDecorator consumes a fixed amount of gas for whitelisted messages
// and then replaces the gas meter with a fixed gas meter to prevent additional gas consumption
type FixedGasConsumedDecorator struct{}

var WhitelistedMessages = map[string]uint64{
	// staking messages
	sdk.MsgTypeURL(&stakingtypes.MsgCreateValidator{}):           200000,
	sdk.MsgTypeURL(&stakingtypes.MsgDelegate{}):                  190000,
	sdk.MsgTypeURL(&stakingtypes.MsgUndelegate{}):                200000,
	sdk.MsgTypeURL(&stakingtypes.MsgBeginRedelegate{}):           232000,
	sdk.MsgTypeURL(&stakingtypes.MsgCancelUnbondingDelegation{}): 200000,
	sdk.MsgTypeURL(&stakingtypes.MsgEditValidator{}):             90000,
}

// NewFixedGasConsumedDecorator creates a new FixedGasConsumedDecorator
func NewFixedGasConsumedDecorator() FixedGasConsumedDecorator {
	return FixedGasConsumedDecorator{}
}

type fixedGasMeter struct {
	consumed storetypes.Gas
}

// NewFixedGasMeter returns a new gas meter with a fixed amount of consumed gas.
func NewFixedGasMeter(consumed storetypes.Gas) storetypes.GasMeter {
	return &fixedGasMeter{
		consumed: consumed,
	}
}

var _ storetypes.GasMeter = &fixedGasMeter{}

func (fgm *fixedGasMeter) GasConsumed() storetypes.Gas        { return fgm.consumed }
func (fgm *fixedGasMeter) GasConsumedToLimit() storetypes.Gas { return fgm.consumed }
func (fgm *fixedGasMeter) GasRemaining() storetypes.Gas       { return 0 }
func (fgm *fixedGasMeter) Limit() storetypes.Gas              { return fgm.consumed }
func (fgm *fixedGasMeter) ConsumeGas(storetypes.Gas, string)  {}
func (fgm *fixedGasMeter) RefundGas(storetypes.Gas, string)   {}
func (fgm *fixedGasMeter) IsPastLimit() bool                  { return false }
func (fgm *fixedGasMeter) IsOutOfGas() bool                   { return false }
func (fgm *fixedGasMeter) String() string                     { return "fixedGasMeter" }

func (fgcd FixedGasConsumedDecorator) AnteHandle(ctx sdk.Context, tx sdk.Tx, simulate bool, next sdk.AnteHandler) (newCtx sdk.Context, err error) {
	msgs := tx.GetMsgs()
	totalFixedGas := uint64(0)
	whitelistedMsgsPresent := false

	for _, msg := range msgs {
		msgTypeURL := sdk.MsgTypeURL(msg)
		if fixedGas, found := WhitelistedMessages[msgTypeURL]; found {
			whitelistedMsgsPresent = true
			totalFixedGas += fixedGas
		}
	}

	if whitelistedMsgsPresent {
		ctx.GasMeter().ConsumeGas(totalFixedGas, "fixed gas for whitelisted messages")
		consumedGas := ctx.GasMeter().GasConsumed()
		ctx = ctx.WithGasMeter(NewFixedGasMeter(consumedGas))
	}

	return next(ctx, tx, simulate)
}

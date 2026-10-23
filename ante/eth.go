// Copyright 2021 Evmos Foundation
// This file is part of Evmos' Ethermint library.
//
// The Ethermint library is free software: you can redistribute it and/or modify
// it under the terms of the GNU Lesser General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// The Ethermint library is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU Lesser General Public License for more details.
//
// You should have received a copy of the GNU Lesser General Public License
// along with the Ethermint library. If not, see https://github.com/evmos/ethermint/blob/main/LICENSE
package ante

import (
	"fmt"
	"math"
	"math/big"

	errorsmod "cosmossdk.io/errors"
	sdkmath "cosmossdk.io/math"

	sdk "github.com/cosmos/cosmos-sdk/types"
	errortypes "github.com/cosmos/cosmos-sdk/types/errors"

	"github.com/evmos/ethermint/ante/interfaces"
	ethermint "github.com/evmos/ethermint/types"
	"github.com/evmos/ethermint/x/evm/keeper"
	"github.com/evmos/ethermint/x/evm/statedb"
	evmtypes "github.com/evmos/ethermint/x/evm/types"

	"github.com/ethereum/go-ethereum/common"
	ethtypes "github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/params"
)

type AccountGetter func(sdk.AccAddress) sdk.AccountI

// NewCachedAccountGetter cache the account objects during the ante handler execution,
// it's safe because there's no store branching in the ante handlers,
// it also creates new account in memory if it doesn't exist in the store.
func NewCachedAccountGetter(ctx sdk.Context, ak evmtypes.AccountKeeper) AccountGetter {
	accounts := make(map[string]sdk.AccountI, 1)
	return func(addr sdk.AccAddress) sdk.AccountI {
		acc := accounts[string(addr)]
		if acc == nil {
			acc = ak.GetAccount(ctx, addr)
			if acc == nil {
				// we create a new account in memory if it doesn't exist,
				// which is only set to store when updated.
				acc = ak.NewAccountWithAddress(ctx, addr)
			}
			accounts[string(addr)] = acc
		}
		return acc
	}
}

// VerifyEthAccount validates checks that the sender balance is greater than the total transaction cost.
// The account will be created in memory if it doesn't exist, i.e cannot be found on store, which will eventually set to
// store when increasing nonce.
// This AnteHandler decorator will fail if:
// - any of the msgs is not a MsgEthereumTx
// - from address is empty
// - account balance is lower than the transaction cost
func VerifyEthAccount(
	ctx sdk.Context, tx sdk.Tx,
	evmKeeper interfaces.EVMKeeper, evmDenom string,
	accountGetter AccountGetter,
) error {
	for _, msg := range tx.GetMsgs() {
		msgEthTx, ok := msg.(*evmtypes.MsgEthereumTx)
		if !ok {
			return errorsmod.Wrapf(errortypes.ErrUnknownRequest, "invalid message type %T, expected %T", msg, (*evmtypes.MsgEthereumTx)(nil))
		}

		ethTx := msgEthTx.AsTransaction()

		// sender address should be in the tx cache from the previous AnteHandle call
		from := msgEthTx.GetFrom()
		if from.Empty() {
			return errorsmod.Wrap(errortypes.ErrInvalidAddress, "from address cannot be empty")
		}

		// check whether the sender address is EOA
		acct := statedb.NewAccountFromSdkAccount(accountGetter(from))

		if acct.IsContract() {
			code := evmKeeper.GetCode(ctx, common.BytesToHash(acct.CodeHash))
			if _, delegated := ethtypes.ParseDelegation(code); !delegated {
				fromAddr := common.BytesToAddress(from)
				return errorsmod.Wrapf(errortypes.ErrInvalidType,
					"the sender is not EOA: address %s, codeHash <%s>", fromAddr, acct.CodeHash)
			}
		}

		balance := evmKeeper.GetBalance(ctx, from, evmDenom)
		if err := keeper.CheckSenderBalance(sdkmath.NewIntFromBigIntMut(balance.ToBig()), ethTx); err != nil {
			return errorsmod.Wrap(err, "failed to check sender balance")
		}
	}
	return nil
}

// CheckEthGasConsume validates that the Ethereum tx message has enough to cover intrinsic gas
// (during CheckTx only) and that the sender has enough balance to pay for the gas cost.
//
// Intrinsic gas for a transaction is the amount of gas that the transaction uses before the
// transaction is executed. The gas is a constant value plus any cost incurred by additional bytes
// of data supplied with the transaction.
//
// This AnteHandler decorator will fail if:
// - the message is not a MsgEthereumTx
// - sender account cannot be found
// - transaction's gas limit is lower than the intrinsic gas
// - user doesn't have enough balance to deduct the transaction fees (gas_limit * gas_price)
// - transaction or block gas meter runs out of gas
// - sets the gas meter limit
// - gas limit is greater than the block gas meter limit
func CheckEthGasConsume(
	ctx sdk.Context, tx sdk.Tx,
	rules params.Rules,
	evmKeeper interfaces.EVMKeeper,
	baseFee *big.Int,
	evmDenom string,
) (sdk.Context, error) {
	gasWanted := uint64(0)
	var events sdk.Events

	// Use the lowest priority of all the messages as the final one.
	minPriority := int64(math.MaxInt64)
	blockGasLimit := ethermint.BlockGasLimit(ctx)

	for _, msg := range tx.GetMsgs() {
		msgEthTx, ok := msg.(*evmtypes.MsgEthereumTx)
		if !ok {
			return ctx, errorsmod.Wrapf(errortypes.ErrUnknownRequest, "invalid message type %T, expected %T", msg, (*evmtypes.MsgEthereumTx)(nil))
		}

		priority := evmtypes.GetTxPriority(msgEthTx, baseFee)

		if priority < minPriority {
			minPriority = priority
		}

		// We can't trust the tx gas limit, because we'll refund the unused gas.
		gasLimit := msgEthTx.GetGas()
		if err := keeper.CheckMaxTxGas(gasLimit, rules); err != nil {
			return ctx, err
		}
		if gasWanted > math.MaxInt64-gasLimit {
			return ctx, fmt.Errorf("gasWanted(%d) + gasLimit(%d) overflow", gasWanted, gasLimit)
		}
		gasWanted += gasLimit
		if gasWanted > blockGasLimit {
			return ctx, errorsmod.Wrapf(
				errortypes.ErrOutOfGas,
				"tx gas (%d) exceeds block gas limit (%d)",
				gasWanted,
				blockGasLimit,
			)
		}
		// user balance is already checked during CheckTx so there's no need to
		// verify it again during ReCheckTx
		if ctx.IsReCheckTx() {
			continue
		}

		fees, err := keeper.VerifyFee(msgEthTx, evmDenom, baseFee, rules, ctx.IsCheckTx())
		if err != nil {
			return ctx, errorsmod.Wrapf(err, "failed to verify the fees")
		}

		err = evmKeeper.DeductTxCostsFromUserBalance(ctx, fees, common.BytesToAddress(msgEthTx.From))
		if err != nil {
			return ctx, errorsmod.Wrapf(err, "failed to deduct transaction costs from user balance")
		}

		events = append(events,
			sdk.NewEvent(
				sdk.EventTypeTx,
				sdk.NewAttribute(sdk.AttributeKeyFee, fees.String()),
			),
		)
	}

	ctx.EventManager().EmitEvents(events)

	// Set tx GasMeter with a limit of GasWanted (i.e gas limit from the Ethereum tx).
	// The gas consumed will be then reset to the gas used by the state transition
	// in the EVM.

	// FIXME: use a custom gas configuration that doesn't add any additional gas and only
	// takes into account the gas consumed at the end of the EVM transaction.
	newCtx := ctx.
		WithGasMeter(ethermint.NewInfiniteGasMeterWithLimit(gasWanted)).
		WithPriority(minPriority)

	// we know that we have enough gas on the pool to cover the intrinsic gas
	return newCtx, nil
}

// CheckEthCanTransfer creates an EVM from the message and calls the BlockContext CanTransfer function to
// see if the address can execute the transaction.
func CheckEthCanTransfer(
	ctx sdk.Context, tx sdk.Tx,
	baseFee *big.Int,
	rules params.Rules,
	evmKeeper interfaces.EVMKeeper,
	evmParams *evmtypes.Params,
) error {
	for _, msg := range tx.GetMsgs() {
		msgEthTx, ok := msg.(*evmtypes.MsgEthereumTx)
		if !ok {
			return errorsmod.Wrapf(errortypes.ErrUnknownRequest, "invalid message type %T, expected %T", msg, (*evmtypes.MsgEthereumTx)(nil))
		}

		tx := msgEthTx.AsTransaction()
		if rules.IsLondon {
			if baseFee == nil {
				return errorsmod.Wrap(
					evmtypes.ErrInvalidBaseFee,
					"base fee is supported but evm block context value is nil",
				)
			}
			if tx.GasFeeCap().Cmp(baseFee) < 0 {
				return errorsmod.Wrapf(
					errortypes.ErrInsufficientFee,
					"max fee per gas less than block base fee (%s < %s)",
					tx.GasFeeCap(), baseFee,
				)
			}
		}
		value := tx.Value()
		if value == nil || value.Sign() == -1 {
			return fmt.Errorf("value (%s) must be positive", value)
		}
		from := common.BytesToAddress(msgEthTx.From)
		// check that caller has enough balance to cover asset transfer for **topmost** call
		// NOTE: here the gas consumed is from the context with the infinite gas meter
		if value.Sign() > 0 && !canTransfer(ctx, evmKeeper, evmParams.EvmDenom, from, value) {
			return errorsmod.Wrapf(
				errortypes.ErrInsufficientFunds,
				"failed to transfer %s from address %s using the EVM block context transfer function",
				value,
				from,
			)
		}
	}

	return nil
}

// canTransfer adapted the core.CanTransfer from go-ethereum
func canTransfer(ctx sdk.Context, evmKeeper interfaces.EVMKeeper, denom string, from common.Address, amount *big.Int) bool {
	balance := evmKeeper.GetBalance(ctx, sdk.AccAddress(from.Bytes()), denom)
	return balance.ToBig().Cmp(amount) >= 0
}

// CheckBatchTransaction requires every MsgEthereumTx in tx to come from one sender.
// Msg types and nonce order are checked later in the ante chain.
func CheckBatchTransaction(tx sdk.Tx) error {
	var (
		sender sdk.AccAddress
		seen   bool
	)
	for _, msg := range tx.GetMsgs() {
		msgEthTx, ok := msg.(*evmtypes.MsgEthereumTx)
		if !ok {
			continue
		}
		from := msgEthTx.GetFrom()
		if !seen {
			sender, seen = from, true
		} else if !from.Equals(sender) {
			return errorsmod.Wrap(errortypes.ErrInvalidRequest, "eth msgs in one tx must share a sender")
		}
	}
	return nil
}

// TxPool looks up a pooled tx by its sender's bech32 address and nonce. It may take the pool lock,
// so it must not be called while the pool runs a callback, as PrepareProposal does with the ante.
type TxPool interface {
	GetTx(sender string, nonce uint64) (sdk.Tx, bool)
}

// CheckAndSetEthSenderNonce verifies each MsgEthereumTx nonce against the sender's sequence and increments it.
// In CheckTx or Simulate, a single-msg tx matching a pooled single-msg eth tx skips both, so the pool can
// apply its replacement rule; a nil pool disables this.
func CheckAndSetEthSenderNonce(
	ctx sdk.Context, tx sdk.Tx, ak evmtypes.AccountKeeper, unsafeUnOrderedTx bool, accountGetter AccountGetter, pool TxPool,
) error {
	msgs := tx.GetMsgs()
	// The pool keys a tx by its first msg only, so a later msg must never match a pooled tx.
	// Not in other modes: the pool is node-local, ReCheckTx would match the tx itself, and
	// PrepareProposal may run the ante while holding the pool lock.
	allowTxReplacement := pool != nil && len(msgs) == 1 && ctx.IsCheckTx() && !ctx.IsReCheckTx()

	for _, msg := range msgs {
		msgEthTx, ok := msg.(*evmtypes.MsgEthereumTx)
		if !ok {
			return errorsmod.Wrapf(errortypes.ErrUnknownRequest, "invalid message type %T, expected %T", msg, (*evmtypes.MsgEthereumTx)(nil))
		}

		tx := msgEthTx.AsTransaction()

		from := msgEthTx.GetFrom()
		acc := accountGetter(from)
		if acc == nil {
			return errorsmod.Wrapf(
				errortypes.ErrUnknownAddress,
				"account %s is nil", common.BytesToAddress(from.Bytes()),
			)
		}
		expectedNonce := acc.GetSequence()
		txNonce := tx.Nonce()

		// if flag is set, we bypass nonce all check verification
		if !unsafeUnOrderedTx {
			// A pooled nonce was applied when that tx was admitted, so leave the sequence as is. After Commit
			// resets checkState it can equal or exceed the sequence; bumping it then would fail the recheck.
			if pooled := pooledEthMsg(pool, allowTxReplacement, from.String(), txNonce); pooled != nil {
				// Resending the pooled tx itself would otherwise fail the replacement rule with a misleading
				// error. Simulating it still succeeds, since nothing is inserted.
				if ctx.ExecMode() != sdk.ExecModeSimulate && pooled.Hash() == msgEthTx.Hash() {
					return errorsmod.Wrapf(errortypes.ErrTxInMempoolCache, "tx %s", msgEthTx.Hash())
				}
				continue
			}
			if txNonce != expectedNonce {
				return errorsmod.Wrapf(
					errortypes.ErrInvalidSequence,
					"invalid nonce; got %d, expected %d", txNonce, expectedNonce,
				)
			}
		}

		// increase sequence of sender
		if err := acc.SetSequence(expectedNonce + 1); err != nil {
			return errorsmod.Wrapf(err, "failed to set sequence to %d", acc.GetSequence()+1)
		}

		ak.SetAccount(ctx, acc)
	}

	return nil
}

// pooledEthMsg returns the msg of the single-msg eth tx pooled at sender and nonce, or nil when
// replacement isn't allowed or none is pooled; a cosmos tx from the same account shares the key
// but must not be replaced by an eth tx.
func pooledEthMsg(pool TxPool, allowTxReplacement bool, sender string, nonce uint64) *evmtypes.MsgEthereumTx {
	if !allowTxReplacement {
		return nil
	}
	pooled, ok := pool.GetTx(sender, nonce)
	if !ok {
		return nil
	}
	return singleEthMsg(pooled)
}

// singleEthMsg returns tx's msg if it carries exactly one, a MsgEthereumTx: the only shape the
// nonce check lets replace a pooled tx.
func singleEthMsg(tx sdk.Tx) *evmtypes.MsgEthereumTx {
	msgs := tx.GetMsgs()
	if len(msgs) != 1 {
		return nil
	}
	msg, _ := msgs[0].(*evmtypes.MsgEthereumTx)
	return msg
}

// EthTxReplacement returns the mempool TxReplacement rule apps should pair with TxPool, matching
// the nonce check skip: only a single-msg eth tx replaces a pooled one. Like geth, the new tx must
// raise the fee cap and the tip cap by at least feeBump percent; unlike geth, only the fee cap must
// rise strictly, so a zero tip may stay zero. It ignores the pool priority, which rounds small tips
// together and moves with the base fee at admission.
func EthTxReplacement(feeBump int64) func(op, np int64, oTx, nTx sdk.Tx) bool {
	bump := big.NewInt(100 + max(feeBump, 0))
	return func(_, _ int64, oTx, nTx sdk.Tx) bool {
		oMsg, nMsg := singleEthMsg(oTx), singleEthMsg(nTx)
		if oMsg == nil || nMsg == nil {
			return false
		}
		o, n := oMsg.AsTransaction(), nMsg.AsTransaction()
		if o == nil || n == nil {
			return false
		}
		oFeeCap, nFeeCap := o.GasFeeCap(), n.GasFeeCap()
		return nFeeCap.Cmp(oFeeCap) > 0 &&
			meetsBump(nFeeCap, oFeeCap, bump) &&
			meetsBump(n.GasTipCap(), o.GasTipCap(), bump)
	}
}

var hundred = big.NewInt(100)

// meetsBump reports whether n >= o*bump/100.
func meetsBump(n, o, bump *big.Int) bool {
	threshold := new(big.Int).Mul(o, bump)
	return n.Cmp(threshold.Quo(threshold, hundred)) >= 0
}

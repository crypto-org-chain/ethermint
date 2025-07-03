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
	"errors"
	"math/big"

	errorsmod "cosmossdk.io/errors"
	sdkmath "cosmossdk.io/math"
	storetypes "cosmossdk.io/store/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	errortypes "github.com/cosmos/cosmos-sdk/types/errors"
	ethtypes "github.com/ethereum/go-ethereum/core/types"
	evmtypes "github.com/evmos/ethermint/x/evm/types"
)

// SetupEthContext is adapted from SetUpContextDecorator from cosmos-sdk, it ignores gas consumption
// by setting the gas meter to infinite
func SetupEthContext(ctx sdk.Context) (newCtx sdk.Context, err error) {
	// We need to setup an empty gas config so that the gas is consistent with Ethereum.
	newCtx = ctx.WithGasMeter(storetypes.NewInfiniteGasMeter()).
		WithKVGasConfig(storetypes.GasConfig{}).
		WithTransientKVGasConfig(storetypes.GasConfig{})

	return newCtx, nil
}

// ValidateEthBasic handles basic validation of tx
func ValidateEthBasic(ctx sdk.Context, tx sdk.Tx, evmParams *evmtypes.Params, baseFee *big.Int) error {
	// no need to validate basic on recheck tx, call next antehandler
	if ctx.IsReCheckTx() {
		return nil
	}

	msgs := tx.GetMsgs()
	if msgs == nil {
		return errorsmod.Wrap(errortypes.ErrUnknownRequest, "invalid transaction. Transaction without messages")
	}

	if t, ok := tx.(sdk.HasValidateBasic); ok {
		err := t.ValidateBasic()
		// ErrNoSignatures is fine with eth tx
		if err != nil && !errors.Is(err, errortypes.ErrNoSignatures) {
			return errorsmod.Wrap(err, "tx basic validation failed")
		}
	}

	// For eth type cosmos tx, some fields should be verified as zero values,
	// since we will only verify the signature against the hash of the MsgEthereumTx.Data
	wrapperTx, ok := tx.(protoTxProvider)
	if !ok {
		return errorsmod.Wrapf(errortypes.ErrUnknownRequest, "invalid tx type %T, didn't implement interface protoTxProvider", tx)
	}

	protoTx := wrapperTx.GetProtoTx()
	body := protoTx.Body
	if body.Memo != "" || body.TimeoutHeight != uint64(0) || len(body.NonCriticalExtensionOptions) > 0 {
		return errorsmod.Wrap(errortypes.ErrInvalidRequest,
			"for eth tx body Memo TimeoutHeight NonCriticalExtensionOptions should be empty")
	}

	if len(body.ExtensionOptions) != 1 {
		return errorsmod.Wrap(errortypes.ErrInvalidRequest, "for eth tx length of ExtensionOptions should be 1")
	}

	authInfo := protoTx.AuthInfo
	if len(authInfo.SignerInfos) > 0 {
		return errorsmod.Wrap(errortypes.ErrInvalidRequest, "for eth tx AuthInfo SignerInfos should be empty")
	}

	if authInfo.Fee.Payer != "" || authInfo.Fee.Granter != "" {
		return errorsmod.Wrap(errortypes.ErrInvalidRequest, "for eth tx AuthInfo Fee payer and granter should be empty")
	}

	sigs := protoTx.Signatures
	if len(sigs) > 0 {
		return errorsmod.Wrap(errortypes.ErrInvalidRequest, "for eth tx Signatures should be empty")
	}

	txFee := sdk.Coins{}
	txGasLimit := uint64(0)

	enableCreate := evmParams.GetEnableCreate()
	enableCall := evmParams.GetEnableCall()
	evmDenom := evmParams.GetEvmDenom()
	allowUnprotectedTxs := evmParams.GetAllowUnprotectedTxs()

	for _, msg := range protoTx.GetMsgs() {
		msgEthTx, ok := msg.(*evmtypes.MsgEthereumTx)
		if !ok {
			return errorsmod.Wrapf(errortypes.ErrUnknownRequest, "invalid message type %T, expected %T", msg, (*evmtypes.MsgEthereumTx)(nil))
		}

		txGasLimit += msgEthTx.GetGas()

		tx := msgEthTx.AsTransaction()
		// return error if contract creation or call are disabled through governance
		if !enableCreate && tx.To() == nil {
			return errorsmod.Wrap(evmtypes.ErrCreateDisabled, "failed to create new contract")
		} else if !enableCall && tx.To() != nil {
			return errorsmod.Wrap(evmtypes.ErrCallDisabled, "failed to call contract")
		}

		if baseFee == nil && tx.Type() == ethtypes.DynamicFeeTxType {
			return errorsmod.Wrap(ethtypes.ErrTxTypeNotSupported, "dynamic fee tx not supported")
		}

		if !allowUnprotectedTxs && !tx.Protected() {
			return errorsmod.Wrapf(
				errortypes.ErrNotSupported,
				"rejected unprotected Ethereum transaction. Please EIP155 sign your transaction to protect it against replay-attacks")
		}

		txFee = txFee.Add(sdk.Coin{Denom: evmDenom, Amount: sdkmath.NewIntFromBigInt(msgEthTx.GetFee())})
	}

	if !authInfo.Fee.Amount.Equal(txFee) {
		return errorsmod.Wrapf(errortypes.ErrInvalidRequest, "invalid AuthInfo Fee Amount (%s != %s)", authInfo.Fee.Amount, txFee)
	}

	if authInfo.Fee.GasLimit != txGasLimit {
		return errorsmod.Wrapf(errortypes.ErrInvalidRequest, "invalid AuthInfo Fee GasLimit (%d != %d)", authInfo.Fee.GasLimit, txGasLimit)
	}

	return nil
}

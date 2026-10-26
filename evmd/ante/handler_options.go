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
	"reflect"

	errorsmod "cosmossdk.io/errors"
	storetypes "github.com/cosmos/cosmos-sdk/store/v2/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	errortypes "github.com/cosmos/cosmos-sdk/types/errors"
	"github.com/cosmos/cosmos-sdk/types/tx/signing"
	"github.com/cosmos/cosmos-sdk/x/auth/ante"
	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"
	txsigning "github.com/cosmos/cosmos-sdk/x/tx/signing"
	ethtypes "github.com/ethereum/go-ethereum/core/types"
	evmante "github.com/evmos/ethermint/ante"
	"github.com/evmos/ethermint/ante/cache"
	"github.com/evmos/ethermint/ante/cosmos"
	"github.com/evmos/ethermint/ante/evm"
	"github.com/evmos/ethermint/ante/interfaces"

	ibcante "github.com/cosmos/ibc-go/v11/modules/core/ante"
	ibckeeper "github.com/cosmos/ibc-go/v11/modules/core/keeper"

	evmtypes "github.com/evmos/ethermint/x/evm/types"
)

// HandlerOptions extend the SDK's AnteHandler options by requiring the IBC
// channel keeper, EVM Keeper and Fee Market Keeper.
type HandlerOptions struct {
	AccountKeeper          evmtypes.AccountKeeper
	BankKeeper             evmtypes.BankKeeper
	IBCKeeper              *ibckeeper.Keeper
	FeeMarketKeeper        interfaces.FeeMarketKeeper
	EvmKeeper              interfaces.EVMKeeper
	FeegrantKeeper         ante.FeegrantKeeper
	SignModeHandler        *txsigning.HandlerMap
	SigGasConsumer         func(meter storetypes.GasMeter, sig signing.SignatureV2, params authtypes.Params) error
	ExtensionOptionChecker ante.ExtensionOptionChecker
	// use dynamic fee checker or the cosmos-sdk default one for native transactions
	DynamicFeeChecker bool
	DisabledAuthzMsgs []string
	ExtraDecorators   []sdk.AnteDecorator
	PendingTxListener PendingTxListener

	// see #494, just for benchmark, don't turn on on production
	UnsafeUnorderedTx bool

	// TxPool lets CheckTx replace a pooled eth tx (and Simulate skip its nonce check); nil disables
	// replacement. The pool should use ante.EthTxReplacement.
	TxPool      evmante.TxPool
	SenderCache *cache.SenderCache
}

func (options HandlerOptions) validate() error {
	if options.AccountKeeper == nil {
		return errorsmod.Wrap(errortypes.ErrLogic, "account keeper is required for AnteHandler")
	}
	if options.BankKeeper == nil {
		return errorsmod.Wrap(errortypes.ErrLogic, "bank keeper is required for AnteHandler")
	}
	if options.SignModeHandler == nil {
		return errorsmod.Wrap(errortypes.ErrLogic, "sign mode handler is required for ante builder")
	}
	if options.FeeMarketKeeper == nil {
		return errorsmod.Wrap(errortypes.ErrLogic, "fee market keeper is required for AnteHandler")
	}
	if options.EvmKeeper == nil {
		return errorsmod.Wrap(errortypes.ErrLogic, "evm keeper is required for AnteHandler")
	}
	// A typed nil pool passes the nil check that disables replacement, then panics on lookup.
	if v := reflect.ValueOf(options.TxPool); v.Kind() == reflect.Pointer && v.IsNil() {
		return errorsmod.Wrap(errortypes.ErrLogic, "tx pool must be a nil interface, not a typed nil, to disable replacement")
	}
	return nil
}

func newEthAnteHandler(options HandlerOptions) sdk.AnteHandler {
	return func(ctx sdk.Context, tx sdk.Tx, simulate bool) (sdk.Context, error) {
		blockCfg, err := options.EvmKeeper.EVMBlockConfig(ctx, options.EvmKeeper.ChainID())
		if err != nil {
			return ctx, errorsmod.Wrap(errortypes.ErrLogic, err.Error())
		}
		evmParams := &blockCfg.Params
		evmDenom := evmParams.EvmDenom
		feemarketParams := &blockCfg.FeeMarketParams
		baseFee := blockCfg.BaseFee
		rules := blockCfg.Rules

		// all transactions must implement FeeTx
		_, ok := tx.(sdk.FeeTx)
		if !ok {
			return ctx, errorsmod.Wrapf(errortypes.ErrInvalidType, "invalid transaction type %T, expected sdk.FeeTx", tx)
		}

		if ctx.IsCheckTx() && !ctx.IsReCheckTx() && !simulate {
			if err := evmante.CheckBatchTransaction(tx); err != nil {
				return ctx, err
			}
		}

		// We need to setup an empty gas config so that the gas is consistent with Ethereum.
		ctx, err = interfaces.SetupEthContext(ctx)
		if err != nil {
			return ctx, err
		}

		if err := cosmos.CheckEthMempoolFee(ctx, tx, simulate, baseFee, evmDenom); err != nil {
			return ctx, err
		}

		if err := cosmos.CheckEthMinGasPrice(tx, feemarketParams.MinGasPrice, baseFee); err != nil {
			return ctx, err
		}

		if err := interfaces.ValidateEthBasic(ctx, tx, evmParams, baseFee); err != nil {
			return ctx, err
		}

		ethSigner := ethtypes.MakeSigner(blockCfg.ChainConfig, blockCfg.BlockNumber, blockCfg.BlockTime)
		if err := evmante.VerifyEthSig(tx, ethSigner, options.SenderCache); err != nil {
			return ctx, err
		}

		pooled, err := evmante.PooledEthTx(ctx, tx, options.TxPool)
		if err != nil {
			return ctx, err
		}

		// AccountGetter cache the account objects during the ante handler execution,
		// it's safe because there's no store branching in the ante handlers.
		accountGetter := evmante.NewCachedAccountGetter(ctx, options.AccountKeeper)

		if err := evmante.VerifyEthAccount(ctx, tx, options.EvmKeeper, evmDenom, accountGetter); err != nil {
			return ctx, err
		}

		if err := evmante.CheckEthCanTransfer(ctx, tx, baseFee, rules, options.EvmKeeper, evmParams); err != nil {
			return ctx, err
		}

		ctx, err = evmante.CheckEthGasConsume(
			ctx, tx, rules, options.EvmKeeper,
			baseFee, evmDenom,
		)
		if err != nil {
			return ctx, err
		}

		if err := evmante.CheckAndSetEthSenderNonce(
			ctx, tx, options.AccountKeeper, options.UnsafeUnorderedTx, accountGetter, pooled); err != nil {
			return ctx, err
		}

		extraDecorators := options.ExtraDecorators
		if options.PendingTxListener != nil {
			extraDecorators = append(extraDecorators, newTxListenerDecorator(options.PendingTxListener))
		}
		if len(extraDecorators) > 0 {
			return sdk.ChainAnteDecorators(extraDecorators...)(ctx, tx, simulate)
		}
		return ctx, nil
	}
}

func newCosmosAnteHandler(ctx sdk.Context, options HandlerOptions, extra ...sdk.AnteDecorator) sdk.AnteHandler {
	evmParams := options.EvmKeeper.GetParams(ctx)
	feemarketParams := options.FeeMarketKeeper.GetParams(ctx)
	evmDenom := evmParams.EvmDenom
	chainID := options.EvmKeeper.ChainID()
	chainCfg := evmParams.GetChainConfig()
	ethCfg := chainCfg.EthereumConfig(chainID)
	var txFeeChecker ante.TxFeeChecker
	if options.DynamicFeeChecker {
		txFeeChecker = evm.NewDynamicFeeChecker(ethCfg, &evmParams, &feemarketParams)
	}
	decorators := make([]sdk.AnteDecorator, 0, 16+len(extra))
	decorators = append(decorators,
		cosmos.RejectMessagesDecorator{}, // reject MsgEthereumTxs
		// disable the Msg types that cannot be included on an authz.MsgExec msgs field
		cosmos.NewAuthzLimiterDecorator(options.DisabledAuthzMsgs),
		ante.NewSetUpContextDecorator(),
		ante.NewExtensionOptionsDecorator(options.ExtensionOptionChecker),
		ante.NewValidateBasicDecorator(),
		ante.NewTxTimeoutHeightDecorator(),
		cosmos.NewMinGasPriceDecorator(options.FeeMarketKeeper, evmDenom, &feemarketParams),
		ante.NewValidateMemoDecorator(options.AccountKeeper),
		ante.NewConsumeGasForTxSizeDecorator(options.AccountKeeper),
		ante.NewDeductFeeDecorator(options.AccountKeeper, options.BankKeeper, options.FeegrantKeeper, txFeeChecker),
		// SetPubKeyDecorator must be called before all signature verification decorators
		ante.NewSetPubKeyDecorator(options.AccountKeeper),
		ante.NewValidateSigCountDecorator(options.AccountKeeper),
		ante.NewSigGasConsumeDecorator(options.AccountKeeper, options.SigGasConsumer),
		ante.NewSigVerificationDecorator(options.AccountKeeper, options.SignModeHandler),
		ante.NewIncrementSequenceDecorator(options.AccountKeeper),
		ibcante.NewRedundantRelayDecorator(options.IBCKeeper),
	)
	decorators = append(decorators, extra...)
	return sdk.ChainAnteDecorators(decorators...)
}

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
package keeper

import (
	"bytes"
	"fmt"
	"sort"

	cmttypes "github.com/cometbft/cometbft/types"

	errorsmod "cosmossdk.io/errors"
	sdkmath "cosmossdk.io/math"
	sdk "github.com/cosmos/cosmos-sdk/types"
	ethermint "github.com/evmos/ethermint/types"
	"github.com/evmos/ethermint/x/evm/statedb"
	"github.com/evmos/ethermint/x/evm/types"
	"github.com/holiman/uint256"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core"
	"github.com/ethereum/go-ethereum/core/tracing"
	ethtypes "github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/core/vm"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/params"
)

// NewEVM generates a go-ethereum VM from the provided Message fields and the chain parameters
// (ChainConfig and module Params). It additionally sets the validator operator address as the
// coinbase address to make it available for the COINBASE opcode, even though there is no
// beneficiary of the coinbase transaction (since we're not mining).
//
// NOTE: the RANDOM opcode is currently not supported since it requires
// RANDAO implementation. See https://github.com/evmos/ethermint/pull/1520#pullrequestreview-1200504697
// for more information.
func (k *Keeper) NewEVM(
	ctx sdk.Context,
	msg *core.Message,
	cfg *EVMConfig,
	stateDB vm.StateDB,
) *vm.EVM {
	blockCtx := vm.BlockContext{
		CanTransfer: core.CanTransfer,
		Transfer:    statedb.Transfer,
		GetHash:     k.GetHashFn(ctx),
		Coinbase:    cfg.CoinBase,
		GasLimit:    ethermint.BlockGasLimit(ctx),
		BlockNumber: cfg.BlockNumber,
		Time:        cfg.BlockTime,
		Difficulty:  cfg.Difficulty,
		BaseFee:     cfg.BaseFee,
		Random:      cfg.Random,
		BlobBaseFee: cfg.BlobBaseFee,
	}
	if cfg.BlockOverrides != nil {
		cfg.BlockOverrides.Apply(&blockCtx)
	}
	if cfg.Tracer == nil {
		cfg.Tracer = k.Tracer(ctx, *msg, cfg.ChainConfig)
	}
	vmConfig := k.VMConfig(ctx, cfg)
	contracts := make(map[common.Address]vm.PrecompiledContract)
	active := make([]common.Address, 0)
	for addr, c := range vm.DefaultPrecompiles(cfg.Rules) {
		contracts[addr] = c
		active = append(active, addr)
	}
	for _, fn := range k.customContractFns {
		c := fn(ctx, cfg.Rules)
		addr := c.Address()
		contracts[addr] = c
		active = append(active, addr)
	}
	sort.SliceStable(active, func(i, j int) bool {
		return bytes.Compare(active[i].Bytes(), active[j].Bytes()) < 0
	})
	evm := vm.NewEVM(blockCtx, stateDB, cfg.ChainConfig, vmConfig)
	evm.SetTxContext(core.NewEVMTxContext(msg))
	evm.SetPrecompiles(contracts)
	return evm
}

// GetHashFn implements vm.GetHashFunc for Ethermint. It returns hash for 3 cases:
//  1. The requested height matches current block height from the context.
//  2. The requested height is below current block height, follow EIP-2935.
//  3. The requested height is above current block height, return empty
func (k Keeper) GetHashFn(ctx sdk.Context) vm.GetHashFunc {
	return func(num64 uint64) common.Hash {
		h, err := ethermint.SafeInt64(num64)
		if err != nil {
			return common.Hash{}
		}
		upper, err := ethermint.SafeUint64(ctx.BlockHeight())
		if err != nil {
			return common.Hash{}
		}
		if upper == num64 {
			headerHash := ctx.HeaderHash()
			if len(headerHash) > 0 {
				return common.BytesToHash(headerHash)
			}
		}
		// Align check with https://github.com/ethereum/go-ethereum/blob/release/1.11/core/vm/instructions.go#L433
		var lower uint64
		headerNum := k.GetParams(ctx).HeaderHashNum
		if upper <= headerNum {
			lower = 0
		} else {
			lower = upper - headerNum
		}

		if upper > num64 {
			// The requested height is historical, query EIP-2935 contract storage
			headerHash := k.GetHeaderHash(ctx, num64)
			if headerHash.Cmp(common.Hash{}) != 0 {
				return headerHash
			} else if num64 >= lower {
				// Pre upgrade case
				// In case EIP-2935 is not supported and data cannot be found, we fetch historical info
				histInfo, err := k.stakingKeeper.GetHistoricalInfo(ctx, h)
				if err != nil {
					k.Logger(ctx).Debug("historical info not found", "height", h, "err", err.Error())
					return common.Hash{}
				}
				header, err := cmttypes.HeaderFromProto(&histInfo.Header)
				if err != nil {
					k.Logger(ctx).Error("failed to cast tendermint header from proto", "error", err)
					return common.Hash{}
				}
				return common.BytesToHash(header.Hash())
			}
		}
		return common.Hash{}
	}
}

// ApplyTransaction runs and attempts to perform a state transition with the given transaction (i.e Message), that will
// only be persisted (committed) to the underlying KVStore if the transaction does not fail.
//
// # Gas tracking
//
// Ethereum consumes gas according to the EVM opcodes instead of general reads and writes to store. Because of this, the
// state transition needs to ignore the SDK gas consumption mechanism defined by the GasKVStore and instead consume the
// amount of gas used by the VM execution. The amount of gas used is tracked by the EVM and returned in the execution
// result.
//
// Prior to the execution, the starting tx gas meter is saved and replaced with an infinite gas meter in a new context
// in order to ignore the SDK gas consumption config values (read, write, has, delete).
// After the execution, the gas used from the message execution will be added to the starting gas consumed, taking into
// consideration the amount of gas returned. Finally, the context is updated with the EVM gas consumed value prior to
// returning.
//
// For relevant discussion see: https://github.com/cosmos/cosmos-sdk/discussions/9072
func (k *Keeper) ApplyTransaction(ctx sdk.Context, msgEth *types.MsgEthereumTx) (*types.EVMResult, error) {
	ethTx := msgEth.AsTransaction()
	cfg, err := k.EVMConfig(ctx, k.eip155ChainID, ethTx.Hash())
	if err != nil {
		return nil, errorsmod.Wrap(err, "failed to load evm config")
	}

	msg := msgEth.AsMessage(cfg.BaseFee)
	// snapshot to contain the tx processing and post processing in same scope
	var commit func()
	tmpCtx := ctx
	if k.hooks != nil {
		// Create a cache context to revert state when tx hooks fails,
		// the cache context is only committed when both tx and hooks executed successfully.
		// Didn't use `Snapshot` because the context stack has exponential complexity on certain operations,
		// thus restricted to be used only inside `ApplyMessage`.
		tmpCtx, commit = ctx.CacheContext()
	}

	// pass true to commit the StateDB
	res, err := k.ApplyMessageWithConfig(tmpCtx, msg, cfg, true)
	if err != nil {
		return nil, errorsmod.Wrap(err, "failed to apply ethereum core message")
	}

	logs := types.LogsToEthereum(res.Logs)

	// Compute block bloom filter
	if len(logs) > 0 {
		bloom := ethtypes.Bloom{}
		for _, log := range logs {
			bloom.Add(log.Address.Bytes())
			for _, topic := range log.Topics {
				bloom.Add(topic[:])
			}
		}
		k.SetTxBloom(tmpCtx, bloom.Big())
	}

	var contractAddr common.Address
	if msg.To == nil {
		contractAddr = crypto.CreateAddress(msg.From, msg.Nonce)
	}

	receipt := &ethtypes.Receipt{
		Type:            ethTx.Type(),
		PostState:       nil, // TODO: intermediate state root
		Logs:            logs,
		TxHash:          cfg.TxConfig.TxHash,
		ContractAddress: contractAddr,
		GasUsed:         res.GasUsed,
		BlockHash:       cfg.TxConfig.BlockHash,
		BlockNumber:     cfg.BlockNumber,
	}

	if !res.Failed() {
		receipt.Status = ethtypes.ReceiptStatusSuccessful
		// Only call hooks if tx executed successfully.
		if err = k.PostTxProcessing(tmpCtx, msg, receipt); err != nil {
			// If hooks return error, revert the whole tx.
			res.VmError = types.ErrPostTxProcessing.Error()
			k.Logger(ctx).Error("tx post processing failed", "error", err)

			// If the tx failed in post processing hooks, we should clear the logs
			res.Logs = nil
		} else if commit != nil {
			// PostTxProcessing is successful, commit the tmpCtx
			commit()
			// Since the post-processing can alter the log, we need to update the result
			res.Logs = types.NewLogsFromEth(receipt.Logs)
		}
	}

	// Get the tracer and add OnGasChange hook for gas refund
	leftoverGas := msg.GasLimit - res.GasUsed

	// refund gas in order to match the Ethereum gas consumption instead of the default SDK one.
	if err = k.RefundGas(ctx, msg, leftoverGas, cfg.Params.EvmDenom); err != nil {
		return nil, errorsmod.Wrapf(err, "failed to refund leftover gas to sender %s", msg.From)
	}

	tracer := cfg.GetTracer()
	if tracer != nil && tracer.OnGasChange != nil {
		tracer.OnGasChange(leftoverGas, 0, tracing.GasChangeTxLeftOverReturned)
	}

	totalGasUsed, err := k.AddTransientGasUsed(ctx, res.GasUsed)
	if err != nil {
		return nil, errorsmod.Wrap(err, "failed to add transient gas used")
	}

	// reset the gas meter for current cosmos transaction
	k.ResetGasMeterAndConsumeGas(ctx, totalGasUsed)

	return res, nil
}

// ApplyMessage calls ApplyMessageWithConfig with an empty TxConfig.
func (k *Keeper) ApplyMessage(ctx sdk.Context, msg *core.Message, tracer *tracing.Hooks, commit bool) (*types.EVMResult, error) {
	cfg, err := k.EVMConfig(ctx, k.eip155ChainID, common.Hash{})
	if err != nil {
		return nil, errorsmod.Wrap(err, "failed to load evm config")
	}

	cfg.Tracer = tracer
	result, err := k.ApplyMessageWithConfig(ctx, msg, cfg, commit)
	if err != nil {
		return nil, err
	}

	return result, nil
}

// ApplyMessageWithConfig computes the new state by applying the given message against the existing state.
// If the message fails, the VM execution error with the reason will be returned to the client
// and the transaction won't be committed to the store.
//
// # Reverted state
//
// The snapshot and rollback are supported by the `statedb.StateDB`.
//
// # Different Callers
//
// It's called in three scenarios:
// 1. `ApplyTransaction`, in the transaction processing flow.
// 2. `EthCall/EthEstimateGas` grpc query handler.
// 3. Called by other native modules directly.
//
// # Prechecks and Preprocessing
//
// All relevant state transition prechecks for the MsgEthereumTx are performed on the AnteHandler,
// prior to running the transaction against the state. The prechecks run are the following:
//
// 1. the nonce of the message caller is correct
// 2. caller has enough balance to cover transaction fee(gaslimit * gasprice)
// 3. the amount of gas required is available in the block
// 4. the purchased gas is enough to cover intrinsic usage
// 5. there is no overflow when calculating intrinsic gas
// 6. caller has enough balance to cover asset transfer for **topmost** call
//
// The preprocessing steps performed by the AnteHandler are:
//
// 1. set up the initial access list (iff fork > Berlin)
//
// # Tracer parameter
//
// It should be a `vm.Tracer` object or nil, if pass `nil`, it'll create a default one based on keeper options.
//
// This is expected used in debug_trace* where AnteHandler is not executed
//
// # Commit parameter
//
// If commit is true, the `StateDB` will be committed, otherwise discarded.
//
// # debugTrace parameter
//
// The message is applied with steps to mimic AnteHandler
//  1. deduct gasLimit * gasPrice (effective gas price) through the fee collector, then refund unused
//     gas after execution — same path as CheckEthGasConsume and ApplyTransaction.
//  2. sender nonce is incremented by 1 before execution
func (k *Keeper) ApplyMessageWithConfig(
	ctx sdk.Context,
	msg *core.Message,
	cfg *EVMConfig,
	commit bool,
) (result *types.EVMResult, err error) {
	var (
		ret     []byte // return bytes from evm execution
		vmErr   error  // vm errors do not effect consensus and are therefore not assigned to err
		gasUsed uint64 // for tracing
	)

	// return error if contract creation or call are disabled through governance
	if !cfg.Params.EnableCreate && msg.To == nil {
		return nil, errorsmod.Wrap(types.ErrCreateDisabled, "failed to create new contract")
	} else if !cfg.Params.EnableCall && msg.To != nil {
		return nil, errorsmod.Wrap(types.ErrCallDisabled, "failed to call contract")
	}

	stateDB := statedb.NewWithParams(ctx, k, cfg.TxConfig, cfg.Params.EvmDenom)
	var evm *vm.EVM
	if cfg.Overrides != nil {
		if err := cfg.Overrides.Apply(stateDB); err != nil {
			return nil, errorsmod.Wrap(err, "failed to apply state override")
		}
	}
	tracingStateDB := vm.StateDB(stateDB)
	if hooks := cfg.Tracer; hooks != nil {
		tracingStateDB = statedb.NewHookedState(stateDB, hooks)
	}
	evm = k.NewEVM(ctx, msg, cfg, tracingStateDB)
	// Allow the tracer captures the tx level events, mainly the gas consumption.
	leftoverGas := msg.GasLimit
	sender := msg.From
	tracer := cfg.GetTracer()

	if tracer != nil {
		defer func() {
			if tracer.OnTxEnd != nil {
				tracer.OnTxEnd(&ethtypes.Receipt{GasUsed: gasUsed}, err)
			}
		}()

		if tracer.OnGasChange != nil {
			tracer.OnGasChange(0, msg.GasLimit, tracing.GasChangeTxInitialBalance)
		}

		if tracer.OnTxStart != nil {
			tracer.OnTxStart(
				evm.GetVMContext(),
				ethtypes.NewTx(&ethtypes.LegacyTx{
					To:    msg.To,
					Data:  msg.Data,
					Value: msg.Value,
					Gas:   msg.GasLimit,
				}),
				msg.From,
			)
		}

		if cfg.DebugTrace {
			feeAmt := debugTraceFeeAmount(msg, cfg.BaseFee)
			stateDB.SubBalance(sender, uint256.MustFromBig(feeAmt), tracing.BalanceDecreaseGasBuy)
			if err := stateDB.Error(); err != nil {
				return nil, err
			}
			tracingStateDB.SetNonce(sender, stateDB.GetNonce(sender)+1, tracing.NonceChangeEoACall)
		}
	}

	rules := cfg.Rules
	contractCreation := msg.To == nil
	intrinsicGas, err := k.GetEthIntrinsicGas(msg, rules, contractCreation)
	if err != nil {
		// should have already been checked on Ante Handler
		return nil, errorsmod.Wrap(err, "intrinsic gas failed")
	}

	// Should check again even if it is checked on Ante Handler, because eth_call don't go through Ante Handler.
	if leftoverGas < intrinsicGas {
		// eth_estimateGas will check for this exact error
		return nil, errorsmod.Wrap(core.ErrIntrinsicGas, "apply message")
	}
	leftoverGas -= intrinsicGas
	if tracer != nil && tracer.OnGasChange != nil {
		tracer.OnGasChange(msg.GasLimit, leftoverGas, tracing.GasChangeTxIntrinsicGas)
	}

	// access list preparation is moved from ante handler to here, because it's needed when `ApplyMessage` is called
	// under contexts where ante handlers are not run, for example `eth_call` and `eth_estimateGas`.
	// Check whether the init code size has been exceeded.
	if rules.IsShanghai && contractCreation && len(msg.Data) > params.MaxInitCodeSize {
		return nil, fmt.Errorf("%w: code size %v limit %v", core.ErrMaxInitCodeSizeExceeded, len(msg.Data), params.MaxInitCodeSize)
	}

	// Execute the preparatory steps for state transition which includes:
	// - prepare accessList(post-berlin)
	// - reset transient storage(eip 1153)
	stateDB.Prepare(rules, msg.From, cfg.CoinBase, msg.To, vm.ActivePrecompiles(rules), msg.AccessList)

	if contractCreation {
		// Take over nonce management from evm:
		// - Reset sender's nonce to msg.Nonce so evm.Create() computes correct contract address
		// - After evm.Create(), calculate the final nonce accounting for:
		//   1. The ante handler's nonce increment (already in oldNonce)
		//   2. Any additional nonce increments from nested CREATEs (e.g., via EIP-7702 callbacks)
		//
		// This is important for batch transactions where ante handler pre-increments
		// nonces for all messages, and for EIP-7702 where constructor callbacks can
		// trigger additional contract deployments.
		oldNonce := stateDB.GetNonce(sender)
		stateDB.SetNonce(sender, msg.Nonce, tracing.NonceChangeUnspecified)
		ret, _, leftoverGas, vmErr = evm.Create(sender, msg.Data, leftoverGas, uint256.MustFromBig(msg.Value))
		// evm.Create() increments nonce from msg.Nonce to (msg.Nonce + 1 + nestedCreates)
		// We need: oldNonce + nestedCreates
		afterCreateNonce := stateDB.GetNonce(sender)
		nestedCreates := afterCreateNonce - msg.Nonce - 1
		stateDB.SetNonce(sender, oldNonce+nestedCreates, tracing.NonceChangeUnspecified)
	} else {
		if msg.SetCodeAuthorizations != nil {
			for _, auth := range msg.SetCodeAuthorizations {
				// Note errors are ignored, we simply skip invalid authorizations here.
				k.applyAuthorization(&auth, stateDB) //nolint:errcheck
			}
		}

		ret, leftoverGas, vmErr = evm.Call(sender, *msg.To, msg.Data, leftoverGas, uint256.MustFromBig(msg.Value))
	}

	refundQuotient := params.RefundQuotient

	// After EIP-3529: refunds are capped to gasUsed / 5
	if rules.IsLondon {
		refundQuotient = params.RefundQuotientEIP3529
	}

	// calculate gas refund
	if msg.GasLimit < leftoverGas {
		return nil, errorsmod.Wrap(types.ErrGasOverflow, "apply message")
	}
	// refund gas
	temporaryGasUsed := msg.GasLimit - leftoverGas
	refund := GasToRefund(stateDB.GetRefund(), temporaryGasUsed, refundQuotient)
	leftoverGas += refund

	if tracer != nil && tracer.OnGasChange != nil {
		tracer.OnGasChange(leftoverGas-refund, leftoverGas, tracing.GasChangeTxRefunds)
	}

	// EVM execution error needs to be available for the JSON-RPC client
	var vmError string
	if vmErr != nil {
		vmError = vmErr.Error()
	}

	// calculate a minimum amount of gas to be charged to sender if GasLimit
	// is considerably higher than GasUsed to stay more aligned with Tendermint gas mechanics
	// for more info https://github.com/evmos/ethermint/issues/1085
	limit, err := ethermint.SafeInt64(msg.GasLimit)
	if err != nil {
		return nil, err
	}
	gasLimit := sdkmath.LegacyNewDec(limit)
	minGasMultiplier := cfg.FeeMarketParams.MinGasMultiplier
	if minGasMultiplier.IsNil() {
		// in case we are executing eth_call on a legacy block, returns a zero value.
		minGasMultiplier = sdkmath.LegacyZeroDec()
	}
	minimumGasUsed := gasLimit.Mul(minGasMultiplier)

	if msg.GasLimit < leftoverGas {
		return nil, errorsmod.Wrapf(types.ErrGasOverflow, "message gas limit < leftover gas (%d < %d)", msg.GasLimit, leftoverGas)
	}
	tempGasUsed, err := ethermint.SafeInt64(temporaryGasUsed)
	if err != nil {
		return nil, err
	}

	gasUsed = sdkmath.LegacyMaxDec(minimumGasUsed, sdkmath.LegacyNewDec(tempGasUsed)).TruncateInt().Uint64()
	// reset leftoverGas, to be used by the tracer
	leftoverGas = msg.GasLimit - gasUsed

	if cfg.DebugTrace {
		if tracer != nil {
			refund := uint256.NewInt(1).Mul(
				uint256.MustFromBig(debugTraceGasPrice(msg, cfg.BaseFee)),
				uint256.NewInt(leftoverGas),
			)
			stateDB.AddBalance(sender, refund, tracing.BalanceIncreaseGasReturn)
		}
	}

	// The dirty states in `StateDB` is either committed or discarded after return
	if commit {
		if err := stateDB.Commit(); err != nil {
			return nil, errorsmod.Wrap(err, "failed to commit stateDB")
		}
	}

	return &types.EVMResult{
		GasUsed:          gasUsed,
		VmError:          vmError,
		Ret:              ret,
		Logs:             types.NewLogsFromEth(stateDB.Logs()),
		Hash:             cfg.TxConfig.TxHash.Hex(),
		BlockHash:        ctx.HeaderHash(),
		ExecutionGasUsed: temporaryGasUsed,
	}, nil
}

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
	"encoding/json"
	"fmt"
	"math/big"
	"strings"

	cmttypes "github.com/cometbft/cometbft/types"

	errorsmod "cosmossdk.io/errors"
	sdkmath "cosmossdk.io/math"
	sdk "github.com/cosmos/cosmos-sdk/types"
	ethermint "github.com/evmos/ethermint/types"
	"github.com/evmos/ethermint/x/evm/types"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core"
	ethtypes "github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/core/vm"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/params"
)

// GetHashFn implements vm.GetHashFunc for Ethermint. It handles 3 cases:
//  1. The requested height matches the current height from context (and thus same epoch number)
//  2. The requested height is from an previous height from the same chain epoch
//  3. The requested height is from a height greater than the latest one
func (k Keeper) GetHashFn(ctx sdk.Context) vm.GetHashFunc {
	return func(height uint64) common.Hash {
		h, err := ethermint.SafeInt64(height)
		if err != nil {
			k.Logger(ctx).Error("failed to cast height to int64", "error", err)
			return common.Hash{}
		}

		switch {
		case ctx.BlockHeight() == h:
			// Case 1: The requested height matches the one from the context so we can retrieve the header
			// hash directly from the context.
			// Note: The headerHash is only set at begin block, it will be nil in case of a query context
			headerHash := ctx.HeaderHash()
			if len(headerHash) != 0 {
				return common.BytesToHash(headerHash)
			}

			// only recompute the hash if not set (eg: checkTxState)
			contextBlockHeader := ctx.BlockHeader()
			header, err := cmttypes.HeaderFromProto(&contextBlockHeader)
			if err != nil {
				k.Logger(ctx).Error("failed to cast tendermint header from proto", "error", err)
				return common.Hash{}
			}

			headerHash = header.Hash()
			return common.BytesToHash(headerHash)

		case ctx.BlockHeight() > h:
			// Case 2: if the chain is not the current height we need to retrieve the hash from the store for the
			// current chain epoch. This only applies if the current height is greater than the requested height.
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
		default:
			// Case 3: heights greater than the current one returns an empty hash.
			return common.Hash{}
		}
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
func (k *Keeper) ApplyTransaction(ctx sdk.Context, msgEth *types.MsgEthereumTx) (*types.MsgEthereumTxResponse, error) {
	var (
		bloom        *big.Int
		bloomReceipt ethtypes.Bloom
	)

	ethTx := msgEth.AsTransaction()
	cfg, err := k.EVMConfig(ctx, sdk.ConsAddress(ctx.BlockHeader().ProposerAddress), k.eip155ChainID, ethTx.Hash())
	if err != nil {
		return nil, errorsmod.Wrap(err, "failed to load evm config")
	}

	msg, err := msgEth.AsMessage(cfg.BaseFee)
	if err != nil {
		return nil, errorsmod.Wrap(err, "failed to return ethereum transaction as core message")
	}

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
		bloom = k.GetBlockBloomTransient(ctx)
		bloom.Or(bloom, big.NewInt(0).SetBytes(ethtypes.LogsBloom(logs)))
		bloomReceipt = ethtypes.BytesToBloom(bloom.Bytes())
	}

	cumulativeGasUsed := res.GasUsed
	if ctx.BlockGasMeter() != nil {
		limit := ctx.BlockGasMeter().Limit()
		cumulativeGasUsed += ctx.BlockGasMeter().GasConsumed()
		if cumulativeGasUsed > limit {
			cumulativeGasUsed = limit
		}
	}

	var contractAddr common.Address
	if msg.To == nil {
		contractAddr = crypto.CreateAddress(msg.From, msg.Nonce)
	}

	receipt := &ethtypes.Receipt{
		Type:              ethTx.Type(),
		PostState:         nil, // TODO: intermediate state root
		CumulativeGasUsed: cumulativeGasUsed,
		Bloom:             bloomReceipt,
		Logs:              logs,
		TxHash:            cfg.TxConfig.TxHash,
		ContractAddress:   contractAddr,
		GasUsed:           res.GasUsed,
		BlockHash:         cfg.TxConfig.BlockHash,
		BlockNumber:       big.NewInt(ctx.BlockHeight()),
		TransactionIndex:  cfg.TxConfig.TxIndex,
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

	// refund gas in order to match the Ethereum gas consumption instead of the default SDK one.
	if err = k.RefundGas(ctx, msg, msg.GasLimit-res.GasUsed, cfg.Params.EvmDenom); err != nil {
		return nil, errorsmod.Wrapf(err, "failed to refund gas leftover gas to sender %s", msg.From)
	}

	if len(receipt.Logs) > 0 {
		// Update transient block bloom filter
		k.SetBlockBloomTransient(ctx, receipt.Bloom.Big())
		k.SetLogSizeTransient(ctx, uint64(cfg.TxConfig.LogIndex)+uint64(len(receipt.Logs)))
	}

	k.SetTxIndexTransient(ctx, uint64(cfg.TxConfig.TxIndex)+1)

	totalGasUsed, err := k.AddTransientGasUsed(ctx, res.GasUsed)
	if err != nil {
		return nil, errorsmod.Wrap(err, "failed to add transient gas used")
	}

	// reset the gas meter for current cosmos transaction
	k.ResetGasMeterAndConsumeGas(ctx, totalGasUsed)
	return res, nil
}

// ApplyMessage calls ApplyMessageWithConfig with an empty TxConfig.
func (k *Keeper) ApplyMessage(ctx sdk.Context, msg core.Message, tracer vm.EVMLogger, commit bool) (*types.MsgEthereumTxResponse, error) {
	cfg, err := k.EVMConfig(ctx, sdk.ConsAddress(ctx.BlockHeader().ProposerAddress), k.eip155ChainID, common.Hash{})
	if err != nil {
		return nil, errorsmod.Wrap(err, "failed to load evm config")
	}

	cfg.Tracer = tracer
	return k.ApplyMessageWithConfig(ctx, msg, cfg, commit)
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
//  1. the sender is consumed with gasLimit * gasPrice in full at the beginning of the execution and
//     then refund with unused gas after execution.
//  2. sender nonce is incremented by 1 before execution
func (k *Keeper) ApplyMessageWithConfig(
	ctx sdk.Context,
	msg core.Message,
	cfg *EVMConfig,
	commit bool,
) (*types.MsgEthereumTxResponse, error) {
	var (
		ret   []byte // return bytes from evm execution
		vmErr error  // vm errors do not effect consensus and are therefore not assigned to err
	)

	// return error if contract creation or call are disabled through governance
	if !cfg.Params.EnableCreate && msg.To == nil {
		return nil, errorsmod.Wrap(types.ErrCreateDisabled, "failed to create new contract")
	} else if !cfg.Params.EnableCall && msg.To != nil {
		return nil, errorsmod.Wrap(types.ErrCallDisabled, "failed to call contract")
	}

	teeRPCClient, err := newTEERPCClient(k.Logger(ctx))
	if err != nil {
		return nil, errorsmod.Wrap(err, "failed to create new TEE rpc client")
	}

	evmId, err := k.startEVM(ctx, msg, cfg, teeRPCClient)
	if err != nil {
		return nil, errorsmod.Wrap(err, "failed to create new RPC server")
	}

	leftoverGas := msg.GasLimit
	sender := vm.AccountRef(msg.From)
	// Allow the tracer captures the tx level events, mainly the gas consumption.
	vmCfg := k.VMConfig(ctx, msg, cfg)
	if vmCfg.Tracer != nil {
		if cfg.DebugTrace {
			// msg.GasPrice should have been set to effective gas price

			// Ethermint original code:
			// stateDB.SubBalance(sender.Address(), new(big.Int).Mul(msg.GasPrice, new(big.Int).SetUint64(msg.GasLimit)))
			var reply StateDBSubBalanceReply
			err := teeRPCClient.StateDBSubBalance(StateDBSubBalanceArgs{
				EvmId:  evmId,
				Caller: sender,
				Msg:    msg,
			}, &reply)
			if err != nil {
				return nil, err
			}

			// Ethermint original code:
			// stateDB.SetNonce(sender.Address(), stateDB.GetNonce(sender.Address())+1)
			var replyNonce StateDBIncreaseNonceReply
			err = teeRPCClient.StateDBIncreaseNonce(StateDBIncreaseNonceArgs{
				EvmId:  evmId,
				Caller: sender,
				Msg:    msg,
			}, &replyNonce)
			if err != nil {
				return nil, err
			}
		}
		vmCfg.Tracer.CaptureTxStart(leftoverGas)
		defer func() {
			if cfg.DebugTrace {
				// Ethermint original code:
				// stateDB.AddBalance(sender.Address(), new(big.Int).Mul(msg.GasPrice, new(big.Int).SetUint64(leftoverGas)))
				var reply StateDBAddBalanceReply
				err := teeRPCClient.StateDBAddBalance(StateDBAddBalanceArgs{
					EvmId:       evmId,
					Caller:      sender,
					Msg:         msg,
					LeftoverGas: leftoverGas,
				}, &reply)
				if err != nil {
					k.Logger(ctx).Error("failed to add balance to TEE stateDB", "error", err)
				}
			}
			vmCfg.Tracer.CaptureTxEnd(leftoverGas)
		}()
	}

	isLondon := cfg.ChainConfig.IsLondon(big.NewInt(ctx.BlockHeight()))
	contractCreation := msg.To == nil
	intrinsicGas, err := k.GetEthIntrinsicGas(ctx, msg, cfg.ChainConfig, contractCreation)
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

	// access list preparation is moved from ante handler to here, because it's needed when `ApplyMessage` is called
	// under contexts where ante handlers are not run, for example `eth_call` and `eth_estimateGas`.
	time := uint64(ctx.BlockHeader().Time.Unix())
	rules := cfg.ChainConfig.Rules(big.NewInt(ctx.BlockHeight()), cfg.ChainConfig.MergeNetsplitBlock != nil, time)
	// Check whether the init code size has been exceeded.
	if rules.IsShanghai && contractCreation && len(msg.Data) > params.MaxInitCodeSize {
		return nil, fmt.Errorf("%w: code size %v limit %v", core.ErrMaxInitCodeSizeExceeded, len(msg.Data), params.MaxInitCodeSize)
	}

	// Execute the preparatory steps for state transition which includes:
	// - prepare accessList(post-berlin)
	// - reset transient storage(eip 1153)

	// Ethermint original code:
	// stateDB.Prepare(rules, msg.From, cfg.CoinBase, msg.To, vm.ActivePrecompiles(rules), msg.AccessList)
	var replyPrepare StateDBPrepareReply
	err = teeRPCClient.StateDBPrepare(StateDBPrepareArgs{
		EvmId:    evmId,
		Msg:      msg,
		Rules:    rules,
		CoinBase: cfg.CoinBase,
	}, &replyPrepare)
	if err != nil {
		return nil, err
	}

	if contractCreation {
		// take over the nonce management from evm:
		// - reset sender's nonce to msg.Nonce() before calling evm.
		// - increase sender's nonce by one no matter the result.

		// Ethermint original code:
		// stateDB.SetNonce(sender.Address(), msg.Nonce)
		var replyNonce StateDBSetNonceReply
		err := teeRPCClient.StateDBSetNonce(StateDBSetNonceArgs{
			EvmId:  evmId,
			Caller: sender,
			Nonce:  msg.Nonce,
		}, &replyNonce)
		if err != nil {
			return nil, err
		}

		// Ethermint original code:
		// ret, _, leftoverGas, vmErr = evm.Create(sender, msg.Data, leftoverGas, msg.Value)
		var reply CreateReply
		vmErr = teeRPCClient.Create(CreateArgs{
			EvmId:  evmId,
			Caller: sender,
			Code:   msg.Data,
			Gas:    leftoverGas,
			Value:  msg.Value,
		}, &reply)
		ret = reply.Ret
		leftoverGas = reply.LeftOverGas

		// Ethermint original code:
		// stateDB.SetNonce(sender.Address(), msg.Nonce+1)
		err = teeRPCClient.StateDBSetNonce(StateDBSetNonceArgs{
			EvmId:  evmId,
			Caller: sender,
			Nonce:  msg.Nonce + 1,
		}, &replyNonce)
		if err != nil {
			return nil, err
		}
	} else {
		// Ethermint original code:
		// ret, leftoverGas, vmErr = evm.Call(sender, *msg.To, msg.Data, leftoverGas, msg.Value)
		var reply CallReply
		vmErr = teeRPCClient.Call(CallArgs{
			EvmId:  evmId,
			Caller: sender,
			Addr:   *msg.To,
			Input:  msg.Data,
			Gas:    leftoverGas,
			Value:  msg.Value,
		}, &reply)
		ret = reply.Ret
		leftoverGas = reply.LeftOverGas
	}

	refundQuotient := params.RefundQuotient

	// After EIP-3529: refunds are capped to gasUsed / 5
	if isLondon {
		refundQuotient = params.RefundQuotientEIP3529
	}

	// calculate gas refund
	if msg.GasLimit < leftoverGas {
		return nil, errorsmod.Wrap(types.ErrGasOverflow, "apply message")
	}

	// refund gas
	temporaryGasUsed := msg.GasLimit - leftoverGas

	// Ethermint original code:
	// leftoverGas += GasToRefund(stateDB.GetRefund(), temporaryGasUsed, refundQuotient)
	var replyRefund StateDBGetRefundReply
	err = teeRPCClient.StateDBGetRefund(StateDBGetRefundArgs{
		EvmId: evmId,
	}, &replyRefund)
	if err != nil {
		return nil, err
	}

	refund := replyRefund.Refund
	leftoverGas += GasToRefund(refund, temporaryGasUsed, refundQuotient)

	// EVM execution error needs to be available for the JSON-RPC client
	var vmError string
	if vmErr != nil {
		vmError = vmErr.Error()
	}

	// The dirty states in `StateDB` is either committed or discarded after return
	if commit {
		// Ethermint original code:
		// if err := stateDB.Commit(); err != nil {
		// 		return nil, errorsmod.Wrap(err, "failed to commit stateDB")
		// }
		var reply CommitReply
		err := teeRPCClient.Commit(CommitArgs{
			EvmId: evmId,
		}, &reply)
		if err != nil {
			return nil, errorsmod.Wrap(err, "failed to commit TEE stateDB")
		}
	}

	// calculate a minimum amount of gas to be charged to sender if GasLimit
	// is considerably higher than GasUsed to stay more aligned with Tendermint gas mechanics
	// for more info https://github.com/evmos/ethermint/issues/1085
	gasLimit := sdkmath.LegacyNewDec(int64(msg.GasLimit))
	minGasMultiplier := cfg.FeeMarketParams.MinGasMultiplier
	if minGasMultiplier.IsNil() {
		// in case we are executing eth_call on a legacy block, returns a zero value.
		minGasMultiplier = sdkmath.LegacyZeroDec()
	}
	minimumGasUsed := gasLimit.Mul(minGasMultiplier)

	if msg.GasLimit < leftoverGas {
		return nil, errorsmod.Wrapf(types.ErrGasOverflow, "message gas limit < leftover gas (%d < %d)", msg.GasLimit, leftoverGas)
	}

	gasUsed := sdkmath.LegacyMaxDec(minimumGasUsed, sdkmath.LegacyNewDec(int64(temporaryGasUsed))).TruncateInt().Uint64()
	// reset leftoverGas, to be used by the tracer
	leftoverGas = msg.GasLimit - gasUsed

	// Ethermint original code:
	// Logs: types.NewLogsFromEth(stateDB.Logs()),
	var replyLog StateDBGetLogsReply
	err = teeRPCClient.StateDBGetLogs(StateDBGetLogsArgs{
		EvmId: evmId,
	}, &replyLog)
	if err != nil {
		return nil, err
	}

	err = k.stopEVM(evmId, teeRPCClient)

	return &types.MsgEthereumTxResponse{
		GasUsed:   gasUsed,
		VmError:   vmError,
		Ret:       ret,
		Logs:      types.NewLogsFromEth(replyLog.Logs),
		Hash:      cfg.TxConfig.TxHash.Hex(),
		BlockHash: ctx.HeaderHash(),
	}, nil
}

// startEVM prepares the transaction for the TEE enclave. It:
// - sends a "StartEVM" request to the TEE enclave with the relevant tx and block info
// - sends a "InitFhevm" request to the TEE enclave to initialize the fhEVM instance. (More comments are in the below.)
func (k *Keeper) startEVM(ctx sdk.Context, msg core.Message, cfg *EVMConfig, teeRPCClient *teeRPCClient) (uint64, error) {
	// Step 1. Send a "StartEVM" request to the TEE enclave.
	ChainConfigJson, err := json.Marshal(cfg.ChainConfig)
	if err != nil {
		return 0, err
	}
	ctx.HeaderHash()
	args := StartEVMArgs{
		Header: ctx.BlockHeader(),
		Msg:    msg,
		EvmConfig: StartEVMTxEVMConfig{
			ChainConfigJson: ChainConfigJson,
			CoinBase:        cfg.CoinBase,
			BaseFee:         cfg.BaseFee,
			TxConfig:        cfg.TxConfig,
			DebugTrace:      cfg.DebugTrace,
			NoBaseFee:       cfg.FeeMarketParams.NoBaseFee,
			EvmDenom:        cfg.Params.EvmDenom,
			Overrides:       ToGOB64(cfg.Overrides),
		},
	}

	reply := StartEVMReply{}
	err = teeRPCClient.StartEVM(args, &reply)
	if err != nil {
		// panic cosmos if TEE isn't available.
		if isTEEDownError(err) {
			panic("TEE rpc server is down")
		}
		return 0, err
	}
	// Snapshot the sdk ctx with this evmId
	k.sdkCtxs[reply.EvmId] = &ctx

	// We unfortunately can't call InitFhevm on the EVM instance during the
	// StartEVM method (inside newEVM), because InitFhevm needs access to the
	// stateDB (via a gRPC calls), but the stateDB (i.e. its associated
	// sdk.Context) is not yet initialized at that point.
	//
	// To solve this, we separate the fhEVM initialization in two phases:
	// 1. StartEVM: Initialize the EVM instance and store it in the evms map
	// (doesn't need access to stateDB).
	// 2. InitFhevm: Initialize the fhEVM instance (needs access to stateDB).
	// ref: https://github.com/Inco-fhevm/zbc-go-ethereum/pull/2
	//
	// We are doing step 2 here.
	err = teeRPCClient.InitFhevm(InitFhevmArgs{
		EvmId: reply.EvmId,
	}, &InitFhevmReply{})
	if err != nil {
		// panic cosmos if TEE isn't available.
		if isTEEDownError(err) {
			panic("TEE rpc server is down")
		}
		return 0, err
	}

	return reply.EvmId, err
}

func (k *Keeper) stopEVM(evmId uint64, teeRPCClient *teeRPCClient) error {
	err := teeRPCClient.StopEVM(StopEVMArgs{
		EvmId: evmId},
		&StopEVMReply{})
	if err != nil {
		// panic cosmos if TEE isn't available.
		if isTEEDownError(err) {
			panic("TEE rpc server is down")
		}
	}
	delete(k.sdkCtxs, evmId)

	return err
}

// isTEEDownError checks if the error is related with RPC server down
func isTEEDownError(err error) bool {
	return strings.Contains(err.Error(), types.ErrTeeConnDown.Error())
}

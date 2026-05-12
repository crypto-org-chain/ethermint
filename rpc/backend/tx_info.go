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
package backend

import (
	"encoding/json"
	"fmt"

	errorsmod "cosmossdk.io/errors"
	abci "github.com/cometbft/cometbft/abci/types"
	tmrpcclient "github.com/cometbft/cometbft/rpc/client"
	tmrpctypes "github.com/cometbft/cometbft/rpc/core/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	errortypes "github.com/cosmos/cosmos-sdk/types/errors"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	ethtypes "github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	rpctypes "github.com/evmos/ethermint/rpc/types"
	ethermint "github.com/evmos/ethermint/types"
	evmtypes "github.com/evmos/ethermint/x/evm/types"
)

// GetTransactionByHash returns the Ethereum format transaction identified by Ethereum transaction hash
func (b *Backend) GetTransactionByHash(txHash common.Hash) (*rpctypes.RPCTransaction, error) {
	res, err := b.GetTxByEthHash(txHash)
	if err != nil {
		return b.getTransactionByHashPending(txHash)
	}
	height, err := ethermint.SafeUint64(res.Height)
	if err != nil {
		return nil, err
	}
	block, err := b.TendermintBlockByNumber(rpctypes.BlockNumber(res.Height))
	if err != nil {
		return nil, err
	}

	if int(res.TxIndex) >= len(block.Block.Txs) {
		return nil, errorsmod.Wrapf(errortypes.ErrLogic, "tx index %d out of range (block has %d txs)", res.TxIndex, len(block.Block.Txs))
	}
	tx, err := b.clientCtx.TxConfig.TxDecoder()(block.Block.Txs[res.TxIndex])
	if err != nil {
		return nil, err
	}

	msgs := tx.GetMsgs()
	if int(res.MsgIndex) >= len(msgs) {
		return nil, errorsmod.Wrapf(errortypes.ErrLogic, "msg index %d out of range (tx has %d msgs)", res.MsgIndex, len(msgs))
	}
	msg, ok := msgs[res.MsgIndex].(*evmtypes.MsgEthereumTx)
	if !ok {
		return nil, errorsmod.Wrapf(errortypes.ErrInvalidType, "msg at index %d is not MsgEthereumTx (got %T)", res.MsgIndex, msgs[res.MsgIndex])
	}

	blockRes, err := b.TendermintBlockResultByNumber(&block.Block.Height)
	if err != nil {
		b.logger.Debug("block result not found", "height", block.Block.Height, "error", err.Error())
		return nil, nil
	}

	if res.EthTxIndex == -1 {
		// Fallback to find tx index by iterating all valid eth transactions
		msgs := b.EthMsgsFromTendermintBlock(block, blockRes)
		for i := range msgs {
			idx, err := ethermint.SafeIntToInt32(i)
			if err != nil {
				return nil, err
			}
			if msgs[i].Hash() == txHash {
				res.EthTxIndex = idx
				break
			}
		}
	}
	// if we still unable to find the eth tx index, return error, shouldn't happen.
	if res.EthTxIndex == -1 {
		return nil, errorsmod.Wrap(errortypes.ErrNotFound, "can't find index of ethereum tx")
	}
	index, err := ethermint.SafeInt32ToUint64(res.EthTxIndex)
	if err != nil {
		return nil, err
	}
	baseFee, err := b.BaseFee(blockRes)
	if err != nil {
		// handle the error for pruned node.
		b.logger.Error("failed to fetch Base Fee from prunned block. Check node prunning configuration", "height", blockRes.Height, "error", err)
	}
	return rpctypes.NewTransactionFromMsg(
		msg,
		common.BytesToHash(block.BlockID.Hash.Bytes()),
		height,
		index,
		baseFee,
		b.chainID,
	)
}

// getTransactionByHashPending find pending tx from mempool
func (b *Backend) getTransactionByHashPending(txHash common.Hash) (*rpctypes.RPCTransaction, error) {
	// try to find tx in mempool
	txs, err := b.PendingTransactions()
	if err != nil {
		b.logger.Debug("tx not found", "hash", txHash, "error", err.Error())
		return nil, nil
	}

	for _, tx := range txs {
		msg, err := evmtypes.UnwrapEthereumMsg(tx, txHash)
		if err != nil {
			// not ethereum tx
			continue
		}

		if msg.Hash() == txHash {
			// use zero block values since it's not included in a block yet
			rpctx, err := rpctypes.NewTransactionFromMsg(
				msg,
				common.Hash{},
				uint64(0),
				uint64(0),
				nil,
				b.chainID,
			)
			if err != nil {
				return nil, err
			}
			return rpctx, nil
		}
	}

	b.logger.Debug("tx not found", "hash", txHash)
	return nil, nil
}

// GetGasUsed returns gasUsed from transaction
func (b *Backend) GetGasUsed(res *ethermint.TxResult, gas uint64) uint64 {
	// patch gasUsed if tx is reverted and happened before height on which fixed was introduced
	// to return real gas charged
	// more info at https://github.com/evmos/ethermint/pull/1557
	if res.Failed && res.Height < b.cfg.JSONRPC.FixRevertGasRefundHeight {
		return gas
	}
	return res.GasUsed
}

// GetTransactionReceipt returns the receipt identified by hash. When block
// is nil the tx is resolved via the KV indexer; otherwise the receipt is
// rebuilt from block to guard against indexer hash→height overwrites.
func (b *Backend) GetTransactionReceipt(hash common.Hash, block *tmrpctypes.ResultBlock) (map[string]interface{}, error) {
	b.logger.Debug("eth_getTransactionReceipt", "hash", hash)

	if block == nil {
		return b.getTransactionReceiptByIndexer(hash)
	}

	blockResults, err := b.TendermintBlockResultByNumber(&block.Block.Height)
	if err != nil {
		b.logger.Debug("failed to retrieve block results", "height", block.Block.Height, "error", err.Error())
		return nil, nil
	}

	input, err := b.collectReceiptEntriesFromBlock(block, blockResults, &hash)
	if err != nil {
		return nil, err
	}
	for i := range input {
		if input[i].hash == hash {
			return b.buildReceiptDirect(block, blockResults, input[i].txResult, input[i].ethMsg)
		}
	}
	b.logger.Debug("tx not found in block", "hash", hash, "height", block.Block.Height)
	return nil, nil
}

// getTransactionReceiptByIndexer resolves the tx via the KV indexer and
// assembles the receipt, folding prior cosmos tx gas into cumulativeGasUsed.
func (b *Backend) getTransactionReceiptByIndexer(hash common.Hash) (map[string]interface{}, error) {
	res, err := b.GetTxByEthHash(hash)
	if err != nil {
		b.logger.Debug("tx not found", "hash", hash, "error", err.Error())
		return nil, nil
	}
	if res == nil {
		b.logger.Debug("tx not found in indexer", "hash", hash)
		return nil, nil
	}
	block, err := b.TendermintBlockByNumber(rpctypes.BlockNumber(res.Height))
	if err != nil {
		b.logger.Debug("block not found", "height", res.Height, "error", err.Error())
		return nil, nil
	}
	blockResults, err := b.TendermintBlockResultByNumber(&res.Height)
	if err != nil {
		b.logger.Debug("failed to retrieve block results", "height", res.Height, "error", err.Error())
		return nil, nil
	}
	if int(res.TxIndex) >= len(block.Block.Txs) {
		return nil, errorsmod.Wrapf(errortypes.ErrLogic, "tx index %d out of range (block has %d txs)", res.TxIndex, len(block.Block.Txs))
	}
	tx, err := b.clientCtx.TxConfig.TxDecoder()(block.Block.Txs[res.TxIndex])
	if err != nil {
		return nil, errorsmod.Wrapf(errortypes.ErrTxDecode, "failed to decode tx: %v", err)
	}
	msgs := tx.GetMsgs()
	if int(res.MsgIndex) >= len(msgs) {
		return nil, errorsmod.Wrapf(errortypes.ErrLogic, "msg index %d out of range (tx has %d msgs)", res.MsgIndex, len(msgs))
	}
	ethMsg, ok := msgs[res.MsgIndex].(*evmtypes.MsgEthereumTx)
	if !ok {
		return nil, errorsmod.Wrapf(errortypes.ErrInvalidType, "msg at index %d is not MsgEthereumTx (got %T)", res.MsgIndex, msgs[res.MsgIndex])
	}

	if int(res.TxIndex) >= len(blockResults.TxsResults) {
		return nil, errorsmod.Wrapf(errortypes.ErrLogic, "tx index %d out of range for block results (%d txs)", res.TxIndex, len(blockResults.TxsResults))
	}
	var priorGas uint64
	for _, txResult := range blockResults.TxsResults[0:res.TxIndex] {
		gas, err := ethermint.SafeUint64(txResult.GasUsed)
		if err != nil {
			return nil, err
		}
		priorGas += gas
	}
	res.CumulativeGasUsed += priorGas
	return b.buildReceiptDirect(block, blockResults, res, ethMsg)
}

type receiptEntry struct {
	hash     common.Hash
	txResult *ethermint.TxResult
	ethMsg   *evmtypes.MsgEthereumTx
}

// collectReceiptEntriesFromBlock walks the block and builds eth receipt entries.
// When stopAtHash is non-nil the walk returns early once that hash is found.
func (b *Backend) collectReceiptEntriesFromBlock(
	block *tmrpctypes.ResultBlock,
	blockResults *tmrpctypes.ResultBlockResults,
	stopAtHash *common.Hash,
) ([]receiptEntry, error) {
	if block == nil || block.Block == nil || blockResults == nil {
		return nil, nil
	}

	entries := make([]receiptEntry, 0, len(block.Block.Txs))
	// Keep eth tx index assignment consistent with KV indexer.
	var ethTxIndex int32
	var cumulativeGasUsed uint64
	for txIndex, txBz := range block.Block.Txs {
		if txIndex >= len(blockResults.TxsResults) {
			return nil, errorsmod.Wrapf(errortypes.ErrLogic, "tx index %d out of range for block results (%d txs)", txIndex, len(blockResults.TxsResults))
		}

		result := blockResults.TxsResults[txIndex]
		if !rpctypes.TxSuccessOrExceedsBlockGasLimit(result) {
			continue
		}

		tx, err := b.clientCtx.TxConfig.TxDecoder()(txBz)
		if err != nil {
			b.logger.Debug("failed to decode transaction in block", "height", block.Block.Height, "error", err.Error())
			continue
		}

		var parsed *rpctypes.ParsedTxs
		if result.Code == abci.CodeTypeOK {
			parsed, err = rpctypes.ParseTxResult(result, tx)
			if err != nil {
				b.logger.Error("failed to parse tx events", "height", block.Block.Height, "tx-index", txIndex, "error", err.Error())
				continue
			}
		}

		for msgIndex, msg := range tx.GetMsgs() {
			ethMsg, ok := msg.(*evmtypes.MsgEthereumTx)
			if !ok {
				continue
			}

			txIdx, err := ethermint.SafeUint32(txIndex)
			if err != nil {
				return nil, err
			}
			msgIdx, err := ethermint.SafeUint32(msgIndex)
			if err != nil {
				return nil, err
			}

			txResult := &ethermint.TxResult{
				Height:     block.Block.Height,
				TxIndex:    txIdx,
				MsgIndex:   msgIdx,
				EthTxIndex: ethTxIndex,
			}

			if result.Code != abci.CodeTypeOK {
				// Exceed-block-gas tx may not emit ethereum_tx events.
				txResult.GasUsed = ethMsg.GetGas()
				txResult.Failed = true
			} else {
				parsedTx := parsed.GetTxByMsgIndex(msgIndex)
				if parsedTx == nil {
					b.logger.Debug("msg index not found in events", "height", block.Block.Height, "tx-index", txIndex, "msg-index", msgIndex)
					continue
				}
				txResult.GasUsed = parsedTx.GasUsed
				txResult.Failed = parsedTx.Failed
			}

			cumulativeGasUsed += txResult.GasUsed
			txResult.CumulativeGasUsed = cumulativeGasUsed

			hash := ethMsg.Hash()
			entries = append(entries, receiptEntry{
				hash:     hash,
				txResult: txResult,
				ethMsg:   ethMsg,
			})
			ethTxIndex++

			if stopAtHash != nil && hash == *stopAtHash {
				return entries, nil
			}
		}
	}

	return entries, nil
}

// buildReceiptDirect assembles the receipt map from resolved tx data.
// Caller must set res.CumulativeGasUsed to the block-wide value.
func (b *Backend) buildReceiptDirect(
	block *tmrpctypes.ResultBlock,
	blockResults *tmrpctypes.ResultBlockResults,
	res *ethermint.TxResult,
	ethMsg *evmtypes.MsgEthereumTx,
) (map[string]interface{}, error) {
	if res == nil || ethMsg == nil {
		return nil, nil
	}
	hash := ethMsg.Hash()
	if block == nil || block.Block == nil {
		return nil, errorsmod.Wrap(errortypes.ErrNotFound, "block not found")
	}
	if blockResults == nil {
		return nil, errorsmod.Wrap(errortypes.ErrNotFound, "block result not found")
	}

	txData := ethMsg.AsTransaction()
	if txData == nil {
		b.logger.Error("failed to unpack tx data")
		return nil, errorsmod.Wrap(errortypes.ErrTxDecode, "failed to unpack tx data")
	}

	if int(res.TxIndex) >= len(blockResults.TxsResults) {
		return nil, errorsmod.Wrapf(errortypes.ErrLogic, "tx index %d out of range for block results (%d txs)", res.TxIndex, len(blockResults.TxsResults))
	}

	var status hexutil.Uint
	if res.Failed {
		status = hexutil.Uint(ethtypes.ReceiptStatusFailed)
	} else {
		status = hexutil.Uint(ethtypes.ReceiptStatusSuccessful)
	}
	chainID, err := b.ChainID()
	if err != nil {
		return nil, err
	}

	from, err := ethMsg.GetSenderLegacy(ethtypes.LatestSignerForChainID(chainID.ToInt()))
	if err != nil {
		return nil, err
	}

	height, err := ethermint.SafeUint64(blockResults.Height)
	if err != nil {
		return nil, err
	}
	// parse tx logs from events
	logs, err := evmtypes.DecodeMsgLogsFromEvents(
		blockResults.TxsResults[res.TxIndex].Data,
		blockResults.TxsResults[res.TxIndex].Events,
		int(res.MsgIndex),
		height,
	)
	if err != nil {
		b.logger.Debug("failed to parse logs", "hash", hash, "error", err.Error())
	}

	if res.EthTxIndex == -1 {
		// Reachable via TM-indexer fallback (ParseTxIndexerResult) when events
		// lack the txIndex attribute. Scan the block for a matching hash.
		msgs := b.EthMsgsFromTendermintBlock(block, blockResults)
		for i := range msgs {
			idx, err := ethermint.SafeIntToInt32(i)
			if err != nil {
				return nil, err
			}
			if msgs[i].Hash() == hash {
				res.EthTxIndex = idx
				break
			}
		}
	}
	if res.EthTxIndex == -1 {
		return nil, errorsmod.Wrap(errortypes.ErrNotFound, "can't find index of ethereum tx")
	}

	blockNumber, err := ethermint.SafeUint64(res.Height)
	if err != nil {
		return nil, err
	}
	transactionIndex, err := ethermint.SafeInt32ToUint64(res.EthTxIndex)
	if err != nil {
		return nil, err
	}

	// create the logs bloom
	var bloom ethtypes.Bloom
	for _, log := range logs {
		bloom.Add(log.Address.Bytes())
		for _, b := range log.Topics {
			bloom.Add(b[:])
		}
	}

	receipt := map[string]interface{}{
		// Consensus fields: These fields are defined by the Yellow Paper
		"status":            status,
		"cumulativeGasUsed": hexutil.Uint64(res.CumulativeGasUsed),
		"logsBloom":         ethtypes.BytesToBloom(bloom.Bytes()),
		"logs":              logs,

		// Implementation fields: These fields are added by geth when processing a transaction.
		// They are stored in the chain database.
		"transactionHash": hash,
		"contractAddress": nil,
		"gasUsed":         hexutil.Uint64(b.GetGasUsed(res, txData.Gas())),

		// Inclusion information: These fields provide information about the inclusion of the
		// transaction corresponding to this receipt.
		"blockHash":        common.BytesToHash(block.Block.Header.Hash()).Hex(),
		"blockNumber":      hexutil.Uint64(blockNumber),
		"transactionIndex": hexutil.Uint64(transactionIndex),

		// sender and receiver (contract or EOA) addreses
		"from": from,
		"to":   txData.To(),
		"type": hexutil.Uint(ethMsg.AsTransaction().Type()),
	}

	if logs == nil {
		receipt["logs"] = [][]*ethtypes.Log{}
	}

	// If the ContractAddress is 20 0x0 bytes, assume it is not a contract creation
	if txData.To() == nil {
		receipt["contractAddress"] = crypto.CreateAddress(from, txData.Nonce())
	}

	if txData.Type() == ethtypes.DynamicFeeTxType {
		baseFee, err := b.BaseFee(blockResults)
		if err != nil {
			// tolerate the error for pruned node.
			b.logger.Error("fetch basefee failed, node is pruned?", "height", res.Height, "error", err)
		} else {
			receipt["effectiveGasPrice"] = hexutil.Big(*ethMsg.GetEffectiveGasPrice(baseFee))
		}
	}

	return receipt, nil
}

// GetTransactionByBlockHashAndIndex returns the transaction identified by hash and index.
func (b *Backend) GetTransactionByBlockHashAndIndex(hash common.Hash, idx hexutil.Uint) (*rpctypes.RPCTransaction, error) {
	b.logger.Debug("eth_getTransactionByBlockHashAndIndex", "hash", hash.Hex(), "index", idx)

	sc, ok := b.clientCtx.Client.(tmrpcclient.SignClient)
	if !ok {
		return nil, errorsmod.Wrap(errortypes.ErrInvalidType, "invalid rpc client")
	}

	block, err := sc.BlockByHash(b.ctx, hash.Bytes())
	if err != nil {
		b.logger.Debug("block not found", "hash", hash.Hex(), "error", err.Error())
		return nil, nil
	}

	if block.Block == nil {
		b.logger.Debug("block not found", "hash", hash.Hex())
		return nil, nil
	}

	return b.GetTransactionByBlockAndIndex(block, idx)
}

// GetTransactionByBlockNumberAndIndex returns the transaction identified by number and index.
func (b *Backend) GetTransactionByBlockNumberAndIndex(blockNum rpctypes.BlockNumber, idx hexutil.Uint) (*rpctypes.RPCTransaction, error) {
	b.logger.Debug("eth_getTransactionByBlockNumberAndIndex", "number", blockNum, "index", idx)

	block, err := b.TendermintBlockByNumber(blockNum)
	if err != nil {
		b.logger.Debug("block not found", "height", blockNum.Int64(), "error", err.Error())
		return nil, nil
	}

	if block.Block == nil {
		b.logger.Debug("block not found", "height", blockNum.Int64())
		return nil, nil
	}

	return b.GetTransactionByBlockAndIndex(block, idx)
}

// GetTxByEthHash uses `/tx_query` to find transaction by ethereum tx hash
// TODO: Don't need to convert once hashing is fixed on Tendermint
// https://github.com/tendermint/tendermint/issues/6539
func (b *Backend) GetTxByEthHash(hash common.Hash) (*ethermint.TxResult, error) {
	if b.indexer != nil {
		return b.indexer.GetByTxHash(hash)
	}

	// fallback to tendermint tx indexer
	query := fmt.Sprintf("%s.%s='%s'", evmtypes.TypeMsgEthereumTx, evmtypes.AttributeKeyEthereumTxHash, hash.Hex())
	txResult, err := b.queryTendermintTxIndexer(query, func(txs *rpctypes.ParsedTxs) *rpctypes.ParsedTx {
		return txs.GetTxByHash(hash)
	})
	if err != nil {
		return nil, errorsmod.Wrapf(err, "GetTxByEthHash %s", hash.Hex())
	}
	return txResult, nil
}

// GetTxByTxIndex uses `/tx_query` to find transaction by tx index of valid ethereum txs
func (b *Backend) GetTxByTxIndex(height int64, i uint) (*ethermint.TxResult, error) {
	index, err := ethermint.SafeUintToInt32(i)
	if err != nil {
		return nil, err
	}
	idx, err := ethermint.SafeInt(i)
	if err != nil {
		return nil, err
	}
	if b.indexer != nil {
		return b.indexer.GetByBlockAndIndex(height, index)
	}

	// fallback to tendermint tx indexer
	query := fmt.Sprintf("tx.height=%d AND %s.%s=%d",
		height, evmtypes.TypeMsgEthereumTx,
		evmtypes.AttributeKeyTxIndex, index,
	)
	txResult, err := b.queryTendermintTxIndexer(query, func(txs *rpctypes.ParsedTxs) *rpctypes.ParsedTx {
		return txs.GetTxByTxIndex(idx)
	})
	if err != nil {
		return nil, errorsmod.Wrapf(err, "GetTxByTxIndex %d %d", height, index)
	}
	return txResult, nil
}

// queryTendermintTxIndexer query tx in tendermint tx indexer
func (b *Backend) queryTendermintTxIndexer(query string, txGetter func(*rpctypes.ParsedTxs) *rpctypes.ParsedTx) (*ethermint.TxResult, error) {
	resTxs, err := b.clientCtx.Client.TxSearch(b.ctx, query, false, nil, nil, "")
	if err != nil {
		return nil, errorsmod.Wrapf(err, "failed to search tx in tendermint indexer, query: %s", query)
	}
	if len(resTxs.Txs) == 0 {
		return nil, errorsmod.Wrap(errortypes.ErrNotFound, "ethereum tx not found")
	}
	txResult := resTxs.Txs[0]
	if !rpctypes.TxSuccessOrExceedsBlockGasLimit(&txResult.TxResult) {
		return nil, errorsmod.Wrapf(errortypes.ErrLogic, "ethereum tx failed, code: %d, log: %s", txResult.TxResult.Code, txResult.TxResult.Log)
	}

	var tx sdk.Tx
	if txResult.TxResult.Code != 0 {
		// it's only needed when the tx exceeds block gas limit
		tx, err = b.clientCtx.TxConfig.TxDecoder()(txResult.Tx)
		if err != nil {
			return nil, errorsmod.Wrapf(errortypes.ErrTxDecode, "failed to decode tx: %v", err)
		}
	}

	return rpctypes.ParseTxIndexerResult(txResult, tx, txGetter)
}

// GetTransactionByBlockAndIndex is the common code shared by `GetTransactionByBlockNumberAndIndex` and `GetTransactionByBlockHashAndIndex`.
func (b *Backend) GetTransactionByBlockAndIndex(block *tmrpctypes.ResultBlock, idx hexutil.Uint) (*rpctypes.RPCTransaction, error) {
	blockRes, err := b.TendermintBlockResultByNumber(&block.Block.Height)
	if err != nil {
		return nil, nil
	}

	var msg *evmtypes.MsgEthereumTx
	// find in tx indexer
	res, err := b.GetTxByTxIndex(block.Block.Height, uint(idx))
	if err == nil {
		if int(res.TxIndex) >= len(block.Block.Txs) {
			return nil, errorsmod.Wrapf(errortypes.ErrLogic, "tx index %d out of range (block has %d txs)", res.TxIndex, len(block.Block.Txs))
		}
		tx, err := b.clientCtx.TxConfig.TxDecoder()(block.Block.Txs[res.TxIndex])
		if err != nil {
			b.logger.Debug("invalid ethereum tx", "height", block.Block.Header, "index", idx)
			return nil, nil
		}

		msgs := tx.GetMsgs()
		if int(res.MsgIndex) >= len(msgs) {
			return nil, errorsmod.Wrapf(errortypes.ErrLogic, "msg index %d out of range (tx has %d msgs)", res.MsgIndex, len(msgs))
		}
		var ok bool
		msg, ok = msgs[res.MsgIndex].(*evmtypes.MsgEthereumTx)
		if !ok {
			b.logger.Debug("invalid ethereum tx", "height", block.Block.Header, "index", idx)
			return nil, nil
		}
	} else {
		i, err := ethermint.SafeHexToInt(idx)
		if err != nil {
			return nil, err
		}
		ethMsgs := b.EthMsgsFromTendermintBlock(block, blockRes)
		if i >= len(ethMsgs) {
			b.logger.Debug("block txs index out of bound", "index", i)
			return nil, nil
		}

		msg = ethMsgs[i]
	}

	baseFee, err := b.BaseFee(blockRes)
	if err != nil {
		// handle the error for pruned node.
		b.logger.Error("failed to fetch Base Fee from prunned block. Check node prunning configuration", "height", block.Block.Height, "error", err)
	}

	height, err := ethermint.SafeUint64(block.Block.Height)
	if err != nil {
		return nil, err
	}

	return rpctypes.NewTransactionFromMsg(
		msg,
		common.BytesToHash(block.Block.Hash()),
		height,
		uint64(idx),
		baseFee,
		b.chainID,
	)
}

// CreateAccessList returns the list of addresses and storage keys used by the transaction (except for the
// sender account and precompiles), plus the estimated gas if the access list were added to the transaction.
func (b *Backend) CreateAccessList(
	args evmtypes.TransactionArgs,
	blockNrOrHash rpctypes.BlockNumberOrHash,
	overrides *json.RawMessage,
) (*rpctypes.AccessListResult, error) {
	blockNb, err := b.BlockNumberFromTendermint(blockNrOrHash)
	if err != nil {
		return nil, err
	}
	res, err := b.CreateAccessListCall(args, blockNb, overrides)
	if err != nil {
		b.logger.Error("failed to call access list", "error", err)
		return nil, err
	}
	gasUsed := hexutil.Uint64(res.GasUsed)
	result := rpctypes.AccessListResult{
		AccessList: &res.Accesslist,
		GasUsed:    &gasUsed,
	}
	return &result, nil
}

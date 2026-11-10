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
	tmrpcclient "github.com/cometbft/cometbft/rpc/client"
	"github.com/ethereum/go-ethereum/common"
	ethtypes "github.com/ethereum/go-ethereum/core/types"
	rpctypes "github.com/evmos/ethermint/rpc/types"
	ethermint "github.com/evmos/ethermint/types"
	evmtypes "github.com/evmos/ethermint/x/evm/types"
	"github.com/pkg/errors"
)

// GetLogs returns all the logs from all the ethereum transactions in a block.
func (b *Backend) GetLogs(hash common.Hash) ([][]*ethtypes.Log, error) {
	resBlock, err := b.TendermintBlockByHash(hash)
	if err != nil {
		return nil, err
	}
	if resBlock == nil {
		return nil, errors.Errorf("block not found for hash %s", hash)
	}
	blockRes, err := b.TendermintBlockResultByNumber(&resBlock.Block.Height)
	if err != nil {
		return nil, err
	}
	logs, err := GetLogsFromBlockResults(blockRes)
	if err != nil {
		return nil, err
	}
	setBlockLogsTimestamp(logs, ethermint.SafeBlockTime(resBlock.Block.Time.Unix()))
	return logs, nil
}

// GetLogsByHeight returns all the logs from all the ethereum transactions in a block.
func (b *Backend) GetLogsByHeight(height *int64) ([][]*ethtypes.Log, error) {
	// NOTE: we query the state in case the tx result logs are not persisted after an upgrade.
	blockRes, err := b.TendermintBlockResultByNumber(height)
	if err != nil {
		return nil, err
	}

	logs, err := GetLogsFromBlockResults(blockRes)
	if err != nil {
		return nil, err
	}
	// only fetch the header if there are logs to fill
	for _, txLogs := range logs {
		if len(txLogs) > 0 {
			setBlockLogsTimestamp(logs, b.BlockTimeByHeight(blockRes.Height))
			break
		}
	}
	return logs, nil
}

// setBlockLogsTimestamp sets the block timestamp of the logs of all the txs of a block.
func setBlockLogsTimestamp(logs [][]*ethtypes.Log, blockTime uint64) {
	for _, txLogs := range logs {
		evmtypes.SetLogsBlockTimestamp(txLogs, blockTime)
	}
}

// BlockTimeByHeight returns the unix time of the block at height, or 0 if it's unknown.
// Non-positive heights return 0, as they would resolve to the latest block.
func (b *Backend) BlockTimeByHeight(height int64) uint64 {
	if height <= 0 {
		return 0
	}
	resHeader, err := b.TendermintHeaderByNumber(rpctypes.BlockNumber(height))
	if err != nil || resHeader == nil || resHeader.Header == nil {
		b.logger.Debug("failed to fetch block header, block timestamp left empty", "height", height, "error", err)
		return 0
	}
	return ethermint.SafeBlockTime(resHeader.Header.Time.Unix())
}

// BlockTimesByRange returns the unix time of the blocks in [minHeight, maxHeight] by height, in a
// single query. The node returns at most 20 headers per query, the other heights are left out.
func (b *Backend) BlockTimesByRange(minHeight, maxHeight int64) map[int64]uint64 {
	minHeight = max(minHeight, 1)
	if minHeight > maxHeight {
		return nil
	}
	hc, ok := b.clientCtx.Client.(tmrpcclient.HistoryClient)
	if !ok {
		b.logger.Debug("invalid rpc client, block timestamps left empty")
		return nil
	}
	res, err := hc.BlockchainInfo(b.ctx, minHeight, maxHeight)
	if err != nil || res == nil {
		b.logger.Debug("failed to fetch block headers, block timestamps left empty",
			"min", minHeight, "max", maxHeight, "error", err)
		return nil
	}
	times := make(map[int64]uint64, len(res.BlockMetas))
	for _, meta := range res.BlockMetas {
		if meta == nil {
			continue
		}
		times[meta.Header.Height] = ethermint.SafeBlockTime(meta.Header.Time.Unix())
	}
	return times
}

// BloomStatus returns the BloomBitsBlocks and the number of processed sections maintained
// by the chain indexer.
func (b *Backend) BloomStatus() (uint64, uint64) {
	return 4096, 0
}

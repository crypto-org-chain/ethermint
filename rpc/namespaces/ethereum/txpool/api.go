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
package txpool

import (
	"fmt"
	"math/big"
	"strconv"

	"cosmossdk.io/log/v2"

	"github.com/cosmos/cosmos-sdk/client"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"

	"github.com/evmos/ethermint/appmempool"
	"github.com/evmos/ethermint/rpc/types"
	ethermint "github.com/evmos/ethermint/types"
	evmtypes "github.com/evmos/ethermint/x/evm/types"
)

const (
	pendingKey = "pending"
	queuedKey  = "queued"
)

// PublicAPI offers the transaction pool API for non-confidential data.
type PublicAPI struct {
	logger        log.Logger
	chainID       *big.Int
	mempoolClient appmempool.MempoolClient
}

// NewPublicAPI creates the txpool service. A nil mempoolClient reports empty pools.
func NewPublicAPI(logger log.Logger, clientCtx client.Context, mempoolClient appmempool.MempoolClient) *PublicAPI {
	chainID, err := ethermint.ParseChainID(clientCtx.ChainID)
	if err != nil {
		panic(err)
	}
	return &PublicAPI{
		logger:        logger.With("module", "txpool"),
		chainID:       chainID,
		mempoolClient: mempoolClient,
	}
}

// pending returns EVM txs that pass keep, keyed by sender → nonce.
// Note: pending/queued split is not supported; all txs are treated as pending.
// Nil keep includes all senders. Block fields are zero (txs not yet mined).
func (api *PublicAPI) pending(keep func(common.Address) bool) map[common.Address]map[uint64]*types.RPCTransaction {
	byAddr := make(map[common.Address]map[uint64]*types.RPCTransaction)
	if api.mempoolClient == nil {
		return byAddr
	}
	for _, sdkTx := range api.mempoolClient.PendingTxs() {
		for _, msg := range sdkTx.GetMsgs() {
			ethMsg, ok := msg.(*evmtypes.MsgEthereumTx)
			if !ok {
				continue
			}
			rpcTx, err := types.NewRPCTransaction(ethMsg, common.Hash{}, 0, 0, 0, nil, api.chainID)
			if err != nil {
				api.logger.Debug("failed to convert pending tx", "error", err.Error())
				continue
			}
			// Filter using the sender from the converted RPC tx to ensure consistency.
			if keep != nil && !keep(rpcTx.From) {
				continue
			}
			if byAddr[rpcTx.From] == nil {
				byAddr[rpcTx.From] = make(map[uint64]*types.RPCTransaction)
			}
			byAddr[rpcTx.From][uint64(rpcTx.Nonce)] = rpcTx
		}
	}
	return byAddr
}

// dumpNonce renders a sender's pending txs into a map keyed by decimal nonce.
func dumpNonce[T any](txs map[uint64]*types.RPCTransaction, convert func(*types.RPCTransaction) T) map[string]T {
	dump := make(map[string]T, len(txs))
	for nonce, tx := range txs {
		dump[strconv.FormatUint(nonce, 10)] = convert(tx)
	}
	return dump
}

// dumpByAddr renders every sender's pending txs, keyed by address then nonce.
func dumpByAddr[T any](byAddr map[common.Address]map[uint64]*types.RPCTransaction, convert func(*types.RPCTransaction) T) map[string]map[string]T {
	dump := make(map[string]map[string]T, len(byAddr))
	for addr, txs := range byAddr {
		dump[addr.Hex()] = dumpNonce(txs, convert)
	}
	return dump
}

func identityRPCTransaction(tx *types.RPCTransaction) *types.RPCTransaction { return tx }

// Content returns pool transactions. All txs are reported as pending;
// pending/queued split is not supported.
func (api *PublicAPI) Content() (map[string]map[string]map[string]*types.RPCTransaction, error) {
	api.logger.Debug("txpool_content")
	pending := dumpByAddr(api.pending(nil), identityRPCTransaction)
	return map[string]map[string]map[string]*types.RPCTransaction{
		pendingKey: pending,
		queuedKey:  make(map[string]map[string]*types.RPCTransaction),
	}, nil
}

// ContentFrom returns pending and queued transactions for the given address.
func (api *PublicAPI) ContentFrom(address common.Address) (map[string]map[string]*types.RPCTransaction, error) {
	api.logger.Debug("txpool_contentFrom", "address", address.Hex())
	fromSender := api.pending(func(a common.Address) bool { return a == address })
	pending := dumpNonce(fromSender[address], identityRPCTransaction)
	return map[string]map[string]*types.RPCTransaction{
		pendingKey: pending,
		queuedKey:  make(map[string]*types.RPCTransaction),
	}, nil
}

// Inspect returns a textual summary of pending and queued transactions.
func (api *PublicAPI) Inspect() (map[string]map[string]map[string]string, error) {
	api.logger.Debug("txpool_inspect")
	pending := dumpByAddr(api.pending(nil), inspectFormat)
	return map[string]map[string]map[string]string{
		pendingKey: pending,
		queuedKey:  make(map[string]map[string]string),
	}, nil
}

// Status returns pending and queued transaction counts.
func (api *PublicAPI) Status() map[string]hexutil.Uint {
	api.logger.Debug("txpool_status")
	var count int
	if api.mempoolClient != nil {
		count = api.mempoolClient.CountTx()
	}
	return map[string]hexutil.Uint{
		pendingKey: hexutil.Uint(count), //#nosec G115 -- count is a non-negative count
		queuedKey:  hexutil.Uint(0),
	}
}

// inspectFormat renders a tx as "to: value wei + gas gas × gasPrice wei"
// (txpool_inspect format). GasPrice is always set by NewRPCTransaction for pending txs.
func inspectFormat(tx *types.RPCTransaction) string {
	to := "contract creation"
	if tx.To != nil {
		to = tx.To.Hex()
	}
	return fmt.Sprintf("%s: %v wei + %v gas × %v wei",
		to, tx.Value.ToInt(), uint64(tx.Gas), tx.GasPrice.ToInt())
}

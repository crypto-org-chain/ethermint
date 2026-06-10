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

	"cosmossdk.io/log/v2"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"

	"github.com/evmos/ethermint/rpc/backend"
	rpctypes "github.com/evmos/ethermint/rpc/types"
	evmtypes "github.com/evmos/ethermint/x/evm/types"
)

// PublicAPI offers and API for the transaction pool. It only operates on data that is non-confidential.
// NOTE: For more info about the current status of this endpoints see https://github.com/evmos/ethermint/issues/124
type PublicAPI struct {
	logger  log.Logger
	backend backend.EVMBackend
}

// NewPublicAPI creates a new tx pool service that gives information about the transaction pool.
func NewPublicAPI(logger log.Logger, b backend.EVMBackend) *PublicAPI {
	return &PublicAPI{
		logger:  logger.With("module", "txpool"),
		backend: b,
	}
}

// Content returns the transactions contained within the transaction pool.
//
// Ethermint's ante handler enforces strict nonce ordering (each submitted tx must equal the
// account's current sequence), so nonce gaps cannot exist in the CometBFT mempool. All
// unconfirmed transactions are therefore immediately executable, and the queued bucket is
// always empty.
func (api *PublicAPI) Content() (map[string]map[string]map[string]*rpctypes.RPCTransaction, error) {
	api.logger.Debug("txpool_content")
	content := map[string]map[string]map[string]*rpctypes.RPCTransaction{
		"pending": make(map[string]map[string]*rpctypes.RPCTransaction),
		"queued":  make(map[string]map[string]*rpctypes.RPCTransaction),
	}

	txs, err := api.backend.PendingTransactions()
	if err != nil {
		api.logger.Debug("txpool_content: failed to fetch pending transactions", "error", err)
		return content, nil
	}

	chainID := api.backend.ChainConfig().ChainID
	for _, sdkTx := range txs {
		for _, msg := range (*sdkTx).GetMsgs() {
			ethMsg, ok := msg.(*evmtypes.MsgEthereumTx)
			if !ok {
				break
			}
			rpcTx, err := rpctypes.NewTransactionFromMsg(ethMsg, common.Hash{}, 0, 0, 0, nil, chainID)
			if err != nil {
				continue
			}
			sender := rpcTx.From.Hex()
			nonce := fmt.Sprintf("%d", uint64(rpcTx.Nonce))
			if content["pending"][sender] == nil {
				content["pending"][sender] = make(map[string]*rpctypes.RPCTransaction)
			}
			content["pending"][sender][nonce] = rpcTx
		}
	}

	return content, nil
}

// ContentFrom returns the pending and queued transactions of a given address.
// The queued bucket is always empty for the same reason as Content().
func (api *PublicAPI) ContentFrom(addr common.Address) (map[string]map[string]*rpctypes.RPCTransaction, error) {
	api.logger.Debug("txpool_contentFrom", "address", addr)
	content := map[string]map[string]*rpctypes.RPCTransaction{
		"pending": make(map[string]*rpctypes.RPCTransaction),
		"queued":  make(map[string]*rpctypes.RPCTransaction),
	}

	txs, err := api.backend.PendingTransactions()
	if err != nil {
		api.logger.Debug("txpool_contentFrom: failed to fetch pending transactions", "error", err)
		return content, nil
	}

	chainID := api.backend.ChainConfig().ChainID
	for _, sdkTx := range txs {
		for _, msg := range (*sdkTx).GetMsgs() {
			ethMsg, ok := msg.(*evmtypes.MsgEthereumTx)
			if !ok {
				break
			}
			rpcTx, err := rpctypes.NewTransactionFromMsg(ethMsg, common.Hash{}, 0, 0, 0, nil, chainID)
			if err != nil {
				continue
			}
			if rpcTx.From != addr {
				continue
			}
			nonce := fmt.Sprintf("%d", uint64(rpcTx.Nonce))
			content["pending"][nonce] = rpcTx
		}
	}

	return content, nil
}

// Inspect returns the content of the transaction pool and flattens it into an
func (api *PublicAPI) Inspect() (map[string]map[string]map[string]string, error) {
	api.logger.Debug("txpool_inspect")
	content := map[string]map[string]map[string]string{
		"pending": make(map[string]map[string]string),
		"queued":  make(map[string]map[string]string),
	}
	return content, nil
}

// Status returns the number of pending and queued transaction in the pool.
func (api *PublicAPI) Status() map[string]hexutil.Uint {
	api.logger.Debug("txpool_status")
	return map[string]hexutil.Uint{
		"pending": hexutil.Uint(0),
		"queued":  hexutil.Uint(0),
	}
}

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
package types

import (
	"fmt"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/rpc"
)

var (
	_ rpc.Error     = (*TxSyncTimeoutError)(nil)
	_ rpc.DataError = (*TxSyncTimeoutError)(nil)
	_ rpc.Error     = (*TxSyncLimitError)(nil)
)

const (
	// ErrCodeTxSyncTimeout is the EIP-7966 JSON-RPC error code returned when
	// eth_sendRawTransactionSync times out before the receipt is available.
	ErrCodeTxSyncTimeout = 4
	// ErrCodeLimitExceeded is the standard JSON-RPC "limit exceeded" error code.
	ErrCodeLimitExceeded = -32005
)

// TxSyncLimitError reports that eth_sendRawTransactionSync was rejected, before
// the transaction was submitted, because too many calls are already waiting.
type TxSyncLimitError struct {
	Limit int
}

func (e *TxSyncLimitError) Error() string {
	return fmt.Sprintf("too many concurrent eth_sendRawTransactionSync calls (limit %d); the transaction was not submitted", e.Limit)
}

// ErrorCode implements rpc.Error.
func (e *TxSyncLimitError) ErrorCode() int { return ErrCodeLimitExceeded }

// TxSyncTimeoutError reports that a transaction was accepted but not included
// within the wait window. The hash is exposed as error data so clients can
// keep polling eth_getTransactionReceipt.
type TxSyncTimeoutError struct {
	Hash    common.Hash
	Timeout time.Duration
}

// NewTxSyncTimeoutError returns a TxSyncTimeoutError for hash and timeout.
func NewTxSyncTimeoutError(hash common.Hash, timeout time.Duration) *TxSyncTimeoutError {
	return &TxSyncTimeoutError{Hash: hash, Timeout: timeout}
}

func (e *TxSyncTimeoutError) Error() string {
	return fmt.Sprintf("The transaction was added to the transaction pool but wasn't processed in %v", e.Timeout)
}

// ErrorCode implements rpc.Error.
func (e *TxSyncTimeoutError) ErrorCode() int { return ErrCodeTxSyncTimeout }

// ErrorData implements rpc.DataError.
func (e *TxSyncTimeoutError) ErrorData() interface{} { return e.Hash.Hex() }

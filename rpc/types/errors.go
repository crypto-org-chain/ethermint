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
	_ rpc.Error     = (*ReceiptWaitTimeoutError)(nil)
	_ rpc.DataError = (*ReceiptWaitTimeoutError)(nil)
	_ rpc.Error     = (*ReceiptWaitersLimitError)(nil)
)

const (
	// ErrCodeReceiptWaitTimeout is the EIP-7966 timeout error code.
	ErrCodeReceiptWaitTimeout = 4
	ErrCodeLimitExceeded      = -32005
)

// ReceiptWaitersLimitError rejects eth_sendRawTransactionSync before submission when max-receipt-waiters is reached.
type ReceiptWaitersLimitError struct {
	Limit int
}

func (e *ReceiptWaitersLimitError) Error() string {
	return fmt.Sprintf("too many eth_sendRawTransactionSync calls waiting for a receipt (limit %d); the transaction was not submitted", e.Limit)
}

// ErrorCode implements rpc.Error.
func (e *ReceiptWaitersLimitError) ErrorCode() int { return ErrCodeLimitExceeded }

// ReceiptWaitTimeoutError carries the hash as error data so clients can keep polling for the receipt.
type ReceiptWaitTimeoutError struct {
	Hash    common.Hash
	Timeout time.Duration
}

func NewReceiptWaitTimeoutError(hash common.Hash, timeout time.Duration) *ReceiptWaitTimeoutError {
	return &ReceiptWaitTimeoutError{Hash: hash, Timeout: timeout}
}

func (e *ReceiptWaitTimeoutError) Error() string {
	return fmt.Sprintf("The transaction was added to the transaction pool but wasn't processed in %v", e.Timeout)
}

// ErrorCode implements rpc.Error.
func (e *ReceiptWaitTimeoutError) ErrorCode() int { return ErrCodeReceiptWaitTimeout }

// ErrorData implements rpc.DataError.
func (e *ReceiptWaitTimeoutError) ErrorData() interface{} { return e.Hash.Hex() }

package types

import (
	"errors"
	"fmt"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/core"
	"github.com/ethereum/go-ethereum/core/vm"
)

// CallError is an error type used in simulated call results.
type CallError struct {
	Message string `json:"message"`
	Code    int    `json:"code"`
	Data    string `json:"data,omitempty"`
}

// InvalidTxError is returned when a transaction fails validation.
type InvalidTxError struct {
	Message string `json:"message"`
	Code    int    `json:"code"`
}

func (e *InvalidTxError) Error() string  { return e.Message }
func (e *InvalidTxError) ErrorCode() int { return e.Code }

// Simulation error codes matching go-ethereum.
const (
	ErrCodeNonceTooLow             = -38010
	ErrCodeNonceTooHigh            = -38011
	ErrCodeIntrinsicGas            = -38013
	ErrCodeInsufficientFunds       = -38014
	ErrCodeBlockGasLimitReached    = -38015
	ErrCodeBlockNumberInvalid      = -38020
	ErrCodeBlockTimestampInvalid   = -38021
	ErrCodeSenderIsNotEOA          = -38024
	ErrCodeMaxInitCodeSizeExceeded = -38025
	ErrCodeClientLimitExceeded     = -38026
	ErrCodeInternalError           = -32603
	ErrCodeInvalidParams           = -32602
	ErrCodeVMError                 = -32015
	ErrCodeServerError             = -32000
)

// TxValidationError maps core transaction validation errors to JSON-RPC error codes.
func TxValidationError(err error) *InvalidTxError {
	if err == nil {
		return nil
	}
	switch {
	case errors.Is(err, core.ErrNonceTooHigh):
		return &InvalidTxError{Message: err.Error(), Code: ErrCodeNonceTooHigh}
	case errors.Is(err, core.ErrNonceTooLow):
		return &InvalidTxError{Message: err.Error(), Code: ErrCodeNonceTooLow}
	case errors.Is(err, core.ErrSenderNoEOA):
		return &InvalidTxError{Message: err.Error(), Code: ErrCodeSenderIsNotEOA}
	case errors.Is(err, core.ErrFeeCapVeryHigh):
		return &InvalidTxError{Message: err.Error(), Code: ErrCodeInvalidParams}
	case errors.Is(err, core.ErrTipVeryHigh):
		return &InvalidTxError{Message: err.Error(), Code: ErrCodeInvalidParams}
	case errors.Is(err, core.ErrTipAboveFeeCap):
		return &InvalidTxError{Message: err.Error(), Code: ErrCodeInvalidParams}
	case errors.Is(err, core.ErrFeeCapTooLow):
		return &InvalidTxError{Message: err.Error(), Code: ErrCodeInvalidParams}
	case errors.Is(err, core.ErrInsufficientFunds):
		return &InvalidTxError{Message: err.Error(), Code: ErrCodeInsufficientFunds}
	case errors.Is(err, core.ErrIntrinsicGas):
		return &InvalidTxError{Message: err.Error(), Code: ErrCodeIntrinsicGas}
	case errors.Is(err, core.ErrInsufficientFundsForTransfer):
		return &InvalidTxError{Message: err.Error(), Code: ErrCodeInsufficientFunds}
	case errors.Is(err, core.ErrMaxInitCodeSizeExceeded):
		return &InvalidTxError{Message: err.Error(), Code: ErrCodeMaxInitCodeSizeExceeded}
	}
	return &InvalidTxError{
		Message: err.Error(),
		Code:    ErrCodeInternalError,
	}
}

// InvalidParamsError is returned for invalid RPC parameters.
type InvalidParamsError struct{ Message string }

func (e *InvalidParamsError) Error() string  { return e.Message }
func (e *InvalidParamsError) ErrorCode() int { return ErrCodeInvalidParams }

// ClientLimitExceededError is returned when the client limit is exceeded.
type ClientLimitExceededError struct{ Message string }

func (e *ClientLimitExceededError) Error() string  { return e.Message }
func (e *ClientLimitExceededError) ErrorCode() int { return ErrCodeClientLimitExceeded }

// InvalidBlockNumberError is returned for invalid block numbers.
type InvalidBlockNumberError struct{ Message string }

func (e *InvalidBlockNumberError) Error() string  { return e.Message }
func (e *InvalidBlockNumberError) ErrorCode() int { return ErrCodeBlockNumberInvalid }

// InvalidBlockTimestampError is returned for invalid block timestamps.
type InvalidBlockTimestampError struct{ Message string }

func (e *InvalidBlockTimestampError) Error() string  { return e.Message }
func (e *InvalidBlockTimestampError) ErrorCode() int { return ErrCodeBlockTimestampInvalid }

// BlockGasLimitReachedError is returned when block gas limit is reached.
type BlockGasLimitReachedError struct{ Message string }

func (e *BlockGasLimitReachedError) Error() string  { return e.Message }
func (e *BlockGasLimitReachedError) ErrorCode() int { return ErrCodeBlockGasLimitReached }

type ServerError struct{ Message string }

func (e *ServerError) Error() string  { return e.Message }
func (e *ServerError) ErrorCode() int { return ErrCodeServerError }

// NewRevertError creates a CallError from EVM revert data.
func NewRevertError(revert []byte) *CallError {
	err := vm.ErrExecutionReverted

	reason, errUnpack := abi.UnpackRevert(revert)
	if errUnpack == nil {
		err = fmt.Errorf("%w: %v", vm.ErrExecutionReverted, reason)
	}
	return &CallError{
		Message: err.Error(),
		Code:    3,
		Data:    hexutil.Encode(revert),
	}
}

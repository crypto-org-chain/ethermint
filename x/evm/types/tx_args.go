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
	"errors"
	"fmt"
	"math"
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/core"
	"github.com/ethereum/go-ethereum/core/types"
	ethermint "github.com/evmos/ethermint/types"
	"github.com/holiman/uint256"
)

// TransactionArgs represents the arguments to construct a new transaction
// or a message call using JSON-RPC.
// Duplicate struct definition since geth struct is in internal package
// Ref: https://github.com/ethereum/go-ethereum/blob/release/1.10.4/internal/ethapi/transaction_args.go#L36
type TransactionArgs struct {
	From                 *common.Address `json:"from"`
	To                   *common.Address `json:"to"`
	Gas                  *hexutil.Uint64 `json:"gas"`
	GasPrice             *hexutil.Big    `json:"gasPrice"`
	MaxFeePerGas         *hexutil.Big    `json:"maxFeePerGas"`
	MaxPriorityFeePerGas *hexutil.Big    `json:"maxPriorityFeePerGas"`
	Value                *hexutil.Big    `json:"value"`
	Nonce                *hexutil.Uint64 `json:"nonce"`

	// We accept "data" and "input" for backwards-compatibility reasons.
	// "input" is the newer name and should be preferred by clients.
	// Issue detail: https://github.com/ethereum/go-ethereum/issues/15628
	Data  *hexutil.Bytes `json:"data"`
	Input *hexutil.Bytes `json:"input"`

	// Introduced by AccessListTxType transaction.
	AccessList *types.AccessList `json:"accessList,omitempty"`
	// For SetCodeTxType
	AuthorizationList []types.SetCodeAuthorization `json:"authorizationList"`
	ChainID           *hexutil.Big                 `json:"chainId,omitempty"`
}

// String return the struct in a string format
func (args *TransactionArgs) String() string {
	// Todo: There is currently a bug with hexutil.Big when the value its nil, printing would trigger an exception
	return fmt.Sprintf("TransactionArgs{From:%v, To:%v, Gas:%v,"+
		" Nonce:%v, Data:%v, Input:%v, AccessList:%v}",
		args.From,
		args.To,
		args.Gas,
		args.Nonce,
		args.Data,
		args.Input,
		args.AccessList)
}

// ToTransaction converts the arguments to an ethereum transaction.
// This assumes that setTxDefaults has been called.
func (args *TransactionArgs) ToTransaction() *MsgEthereumTx {
	var gas, nonce uint64
	if args.Nonce != nil {
		nonce = uint64(*args.Nonce)
	}

	if args.Gas != nil {
		gas = uint64(*args.Gas)
	}

	var data types.TxData
	switch {
	case args.AuthorizationList != nil:
		al := types.AccessList{}
		if args.AccessList != nil {
			al = *args.AccessList
		}
		data = &types.SetCodeTx{
			To:         *args.To,
			ChainID:    uint256.MustFromBig(args.ChainID.ToInt()),
			Nonce:      uint64(*args.Nonce),
			Gas:        uint64(*args.Gas),
			GasFeeCap:  uint256.MustFromBig((*big.Int)(args.MaxFeePerGas)),
			GasTipCap:  uint256.MustFromBig((*big.Int)(args.MaxPriorityFeePerGas)),
			Value:      uint256.MustFromBig((*big.Int)(args.Value)),
			Data:       args.GetData(),
			AccessList: al,
			AuthList:   args.AuthorizationList,
		}
	case args.MaxFeePerGas != nil:
		al := types.AccessList{}
		if args.AccessList != nil {
			al = *args.AccessList
		}
		data = &types.DynamicFeeTx{
			To:         args.To,
			ChainID:    (*big.Int)(args.ChainID),
			Nonce:      nonce,
			Gas:        gas,
			GasFeeCap:  (*big.Int)(args.MaxFeePerGas),
			GasTipCap:  (*big.Int)(args.MaxPriorityFeePerGas),
			Value:      (*big.Int)(args.Value),
			Data:       args.GetData(),
			AccessList: al,
		}
	case args.AccessList != nil:
		data = &types.AccessListTx{
			To:         args.To,
			ChainID:    (*big.Int)(args.ChainID),
			Nonce:      nonce,
			Gas:        gas,
			GasPrice:   (*big.Int)(args.GasPrice),
			Value:      (*big.Int)(args.Value),
			Data:       args.GetData(),
			AccessList: *args.AccessList,
		}
	default:
		data = &types.LegacyTx{
			To:       args.To,
			Nonce:    nonce,
			Gas:      gas,
			GasPrice: (*big.Int)(args.GasPrice),
			Value:    (*big.Int)(args.Value),
			Data:     args.GetData(),
		}
	}

	tx := NewTxWithData(data)
	if args.From != nil {
		tx.From = args.From.Bytes()
	}
	return tx
}

// ToMessage converts the arguments to the Message type used by the core evm.
// This assumes that setTxDefaults has been called.
func (args *TransactionArgs) ToMessage(globalGasCap uint64, baseFee *big.Int) (*core.Message, error) {
	// Reject invalid combinations of pre- and post-1559 fee styles
	if args.GasPrice != nil && (args.MaxFeePerGas != nil || args.MaxPriorityFeePerGas != nil) {
		return nil, errors.New("both gasPrice and (maxFeePerGas or maxPriorityFeePerGas) specified")
	}

	// Set sender address or use zero address if none specified.
	addr := args.GetFrom()
	// Ethereum block size is ~36000000, we use this value as default in case neither gas or global cap is specified, to protect against DOS
	defaultGas := uint64(100000000)
	gas := defaultGas
	if args.Gas != nil {
		gas = uint64(*args.Gas)
	}
	if globalGasCap != 0 && globalGasCap < gas {
		gas = globalGasCap
	}

	var (
		gasPrice  *big.Int
		gasFeeCap *big.Int
		gasTipCap *big.Int
	)
	if baseFee == nil {
		// If there's no basefee, then it must be a non-1559 execution
		gasPrice = new(big.Int)
		if args.GasPrice != nil {
			gasPrice = args.GasPrice.ToInt()
		}
		gasFeeCap, gasTipCap = gasPrice, gasPrice
	} else {
		// A basefee is provided, necessitating 1559-type execution
		if args.GasPrice != nil {
			// User specified the legacy gas field, convert to 1559 gas typing
			gasPrice = args.GasPrice.ToInt()
			gasFeeCap, gasTipCap = gasPrice, gasPrice
		} else {
			// User specified 1559 gas fields (or none), use those
			gasFeeCap = new(big.Int)
			if args.MaxFeePerGas != nil {
				gasFeeCap = args.MaxFeePerGas.ToInt()
			}
			gasTipCap = new(big.Int)
			if args.MaxPriorityFeePerGas != nil {
				gasTipCap = args.MaxPriorityFeePerGas.ToInt()
			}
			// Backfill the legacy gasPrice for EVM execution, unless we're all zeroes
			gasPrice = new(big.Int)
			if gasFeeCap.BitLen() > 0 || gasTipCap.BitLen() > 0 {
				gasPrice = ethermint.BigMin(new(big.Int).Add(gasTipCap, baseFee), gasFeeCap)
			}
		}
	}
	value := new(big.Int)
	if args.Value != nil {
		value = args.Value.ToInt()
	}
	data := args.GetData()
	var accessList types.AccessList
	if args.AccessList != nil {
		accessList = *args.AccessList
	}

	nonce := uint64(0)
	if args.Nonce != nil {
		nonce = uint64(*args.Nonce)
	}

	msg := &core.Message{
		From:                  addr,
		To:                    args.To,
		Nonce:                 nonce,
		Value:                 value,
		GasLimit:              gas,
		GasPrice:              gasPrice,
		GasFeeCap:             gasFeeCap,
		GasTipCap:             gasTipCap,
		Data:                  data,
		AccessList:            accessList,
		SetCodeAuthorizations: args.AuthorizationList,
		SkipNonceChecks:       true,
		SkipFromEOACheck:      true,
	}
	return msg, nil
}

// GetFrom retrieves the transaction sender address.
func (args *TransactionArgs) GetFrom() common.Address {
	if args.From == nil {
		return common.Address{}
	}
	return *args.From
}

// GetData retrieves the transaction calldata. Input field is preferred.
func (args *TransactionArgs) GetData() []byte {
	if args.Input != nil {
		return *args.Input
	}
	if args.Data != nil {
		return *args.Data
	}
	return nil
}

// ToSimMessage converts the arguments to a core.Message for eth_simulateV1.
// It controls SkipNonceChecks based on validation mode and always skips EOA checks.
// Gas is not capped by globalGasCap (the simulator manages gas budgets separately).
func (args *TransactionArgs) ToSimMessage(baseFee *big.Int, skipNonce bool) (*core.Message, error) {
	// Reject invalid combinations of pre- and post-1559 fee styles
	if args.GasPrice != nil && (args.MaxFeePerGas != nil || args.MaxPriorityFeePerGas != nil) {
		return nil, errors.New("both gasPrice and (maxFeePerGas or maxPriorityFeePerGas) specified")
	}

	addr := args.GetFrom()
	gas := uint64(0)
	if args.Gas != nil {
		gas = uint64(*args.Gas)
	}

	var (
		gasPrice  *big.Int
		gasFeeCap *big.Int
		gasTipCap *big.Int
	)
	if baseFee == nil {
		gasPrice = new(big.Int)
		if args.GasPrice != nil {
			gasPrice = args.GasPrice.ToInt()
		}
		gasFeeCap, gasTipCap = gasPrice, gasPrice
	} else {
		if args.GasPrice != nil {
			gasPrice = args.GasPrice.ToInt()
			gasFeeCap, gasTipCap = gasPrice, gasPrice
		} else {
			gasFeeCap = new(big.Int)
			if args.MaxFeePerGas != nil {
				gasFeeCap = args.MaxFeePerGas.ToInt()
			}
			gasTipCap = new(big.Int)
			if args.MaxPriorityFeePerGas != nil {
				gasTipCap = args.MaxPriorityFeePerGas.ToInt()
			}
			gasPrice = new(big.Int)
			if gasFeeCap.BitLen() > 0 || gasTipCap.BitLen() > 0 {
				gasPrice = ethermint.BigMin(new(big.Int).Add(gasTipCap, baseFee), gasFeeCap)
			}
		}
	}
	value := new(big.Int)
	if args.Value != nil {
		value = args.Value.ToInt()
	}
	data := args.GetData()
	var accessList types.AccessList
	if args.AccessList != nil {
		accessList = *args.AccessList
	}
	nonce := uint64(0)
	if args.Nonce != nil {
		nonce = uint64(*args.Nonce)
	}
	return &core.Message{
		From:                  addr,
		To:                    args.To,
		Nonce:                 nonce,
		Value:                 value,
		GasLimit:              gas,
		GasPrice:              gasPrice,
		GasFeeCap:             gasFeeCap,
		GasTipCap:             gasTipCap,
		Data:                  data,
		AccessList:            accessList,
		SetCodeAuthorizations: args.AuthorizationList,
		SkipNonceChecks:       skipNonce,
		SkipFromEOACheck:      true,
	}, nil
}

// CallDefaults fills in default values for unset fields in TransactionArgs
// for simulation calls. This is adapted from go-ethereum's TransactionArgs.CallDefaults.
func (args *TransactionArgs) CallDefaults(globalGasCap uint64, baseFee *big.Int, chainID *big.Int) error {
	if args.GasPrice != nil && (args.MaxFeePerGas != nil || args.MaxPriorityFeePerGas != nil) {
		return errors.New("both gasPrice and (maxFeePerGas or maxPriorityFeePerGas) specified")
	}
	if args.ChainID == nil {
		args.ChainID = (*hexutil.Big)(chainID)
	} else {
		if have := (*big.Int)(args.ChainID); have.Cmp(chainID) != 0 {
			return fmt.Errorf("chainId does not match node's (have=%v, want=%v)", have, chainID)
		}
	}
	if args.Gas == nil {
		gas := globalGasCap
		if gas == 0 {
			gas = uint64(math.MaxUint64 / 2)
		}
		args.Gas = (*hexutil.Uint64)(&gas)
	} else if globalGasCap > 0 && globalGasCap < uint64(*args.Gas) {
		args.Gas = (*hexutil.Uint64)(&globalGasCap)
	}
	if args.Nonce == nil {
		args.Nonce = new(hexutil.Uint64)
	}
	if args.Value == nil {
		args.Value = new(hexutil.Big)
	}
	if baseFee == nil {
		if args.GasPrice == nil {
			args.GasPrice = new(hexutil.Big)
		}
	} else if args.GasPrice == nil {
		if args.MaxFeePerGas == nil {
			args.MaxFeePerGas = new(hexutil.Big)
		}
		if args.MaxPriorityFeePerGas == nil {
			args.MaxPriorityFeePerGas = new(hexutil.Big)
		}
	}
	return nil
}

// ToEthTransaction converts the arguments to an ethereum types.Transaction.
// Used by the simulator to produce transactions for block construction.
func (args *TransactionArgs) ToEthTransaction() *types.Transaction {
	var txData types.TxData
	nonce := uint64(0)
	if args.Nonce != nil {
		nonce = uint64(*args.Nonce)
	}
	gas := uint64(0)
	if args.Gas != nil {
		gas = uint64(*args.Gas)
	}

	switch {
	case args.AuthorizationList != nil:
		al := types.AccessList{}
		if args.AccessList != nil {
			al = *args.AccessList
		}
		to := common.Address{}
		if args.To != nil {
			to = *args.To
		}
		txData = &types.SetCodeTx{
			To:         to,
			ChainID:    uint256.MustFromBig(args.ChainID.ToInt()),
			Nonce:      nonce,
			Gas:        gas,
			GasFeeCap:  uint256.MustFromBig((*big.Int)(args.MaxFeePerGas)),
			GasTipCap:  uint256.MustFromBig((*big.Int)(args.MaxPriorityFeePerGas)),
			Value:      uint256.MustFromBig((*big.Int)(args.Value)),
			Data:       args.GetData(),
			AccessList: al,
			AuthList:   args.AuthorizationList,
		}
	case args.MaxFeePerGas != nil:
		al := types.AccessList{}
		if args.AccessList != nil {
			al = *args.AccessList
		}
		txData = &types.DynamicFeeTx{
			To:         args.To,
			ChainID:    (*big.Int)(args.ChainID),
			Nonce:      nonce,
			Gas:        gas,
			GasFeeCap:  (*big.Int)(args.MaxFeePerGas),
			GasTipCap:  (*big.Int)(args.MaxPriorityFeePerGas),
			Value:      (*big.Int)(args.Value),
			Data:       args.GetData(),
			AccessList: al,
		}
	case args.AccessList != nil:
		txData = &types.AccessListTx{
			To:         args.To,
			ChainID:    (*big.Int)(args.ChainID),
			Nonce:      nonce,
			Gas:        gas,
			GasPrice:   (*big.Int)(args.GasPrice),
			Value:      (*big.Int)(args.Value),
			Data:       args.GetData(),
			AccessList: *args.AccessList,
		}
	default:
		txData = &types.LegacyTx{
			To:       args.To,
			Nonce:    nonce,
			Gas:      gas,
			GasPrice: (*big.Int)(args.GasPrice),
			Value:    (*big.Int)(args.Value),
			Data:     args.GetData(),
		}
	}
	return types.NewTx(txData)
}

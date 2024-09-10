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
	fmt "fmt"
	math "math"
	"math/big"
	"math/bits"

	sdkmath "cosmossdk.io/math"
	"github.com/ethereum/go-ethereum/common/hexutil"
)

const (
	maxWordLen = sdkmath.MaxBitLen / bits.UintSize
)

var MaxInt256 *big.Int

func init() {
	var tmp big.Int
	MaxInt256 = tmp.Lsh(big.NewInt(1), sdkmath.MaxBitLen).Sub(&tmp, big.NewInt(1))
}

// SafeInt64 checks for overflows while casting a uint64 to int64 value.
func SafeInt64(value uint64) (int64, error) {
	if value > uint64(math.MaxInt64) {
		return 0, fmt.Errorf("uint64 value %v cannot exceed %v", value, math.MaxInt64)
	}

	return int64(value), nil
}

func SafeUint64ToInt32(value uint64) (int32, error) {
	if value > uint64(math.MaxInt64) {
		return 0, fmt.Errorf("uint64 value %v cannot exceed %v", value, math.MaxInt64)
	}

	return int32(value), nil //nolint:gosec // checked
}

func SafeUint64ToInt(value uint64) (int, error) {
	if value > uint64(math.MaxInt64) {
		return 0, fmt.Errorf("uint64 value %v cannot exceed %v", value, math.MaxInt64)
	}

	return int(value), nil
}

func SafeHexToInt64(value hexutil.Uint64) (int64, error) {
	if value > math.MaxInt64 {
		return 0, fmt.Errorf("hexutil.Uint64 value %v cannot exceed %v", value, math.MaxInt64)
	}

	return int64(value), nil //nolint:gosec // checked
}

func SafeUint32(value int) (uint32, error) {
	if value > math.MaxUint32 {
		return 0, fmt.Errorf("int value %v cannot exceed %v", value, math.MaxUint32)
	}

	return uint32(value), nil //nolint:gosec // checked
}

func SafeUint64(value int64) (uint64, error) {
	if value < 0 {
		return 0, fmt.Errorf("invalid value: %d", value)
	}
	return uint64(value), nil
}

func SafeIntToUint64(value int) (uint64, error) {
	if value < 0 {
		return 0, fmt.Errorf("invalid value: %d", value)
	}
	return uint64(value), nil
}

func SafeInt32ToUint64(value int32) (uint64, error) {
	if value < 0 {
		return 0, fmt.Errorf("invalid value: %d", value)
	}
	return uint64(value), nil
}

func SafeUint(value int) (uint, error) {
	if value < 0 {
		return 0, fmt.Errorf("invalid value: %d", value)
	}
	return uint(value), nil
}

func SafeUintToInt32(value uint) (int32, error) {
	if value > uint(math.MaxInt32) {
		return 0, fmt.Errorf("uint value %v cannot exceed %v", value, math.MaxUint32)
	}

	return int32(value), nil
}

func SafeIntToInt32(value int) (int32, error) {
	if value > int(math.MaxInt32) {
		return 0, fmt.Errorf("int value %v cannot exceed %v", value, math.MaxUint32)
	}

	return int32(value), nil //nolint:gosec // checked
}

func SafeInt(value uint) (int, error) {
	if value > uint(math.MaxInt64) {
		return 0, fmt.Errorf("uint value %v cannot exceed %v", value, math.MaxInt64)
	}

	return int(value), nil
}

func SafeHexToInt(value hexutil.Uint) (int, error) {
	if value > hexutil.Uint(math.MaxInt) {
		return 0, fmt.Errorf("hexutil.Uint value %v cannot exceed %v", value, math.MaxInt)
	}

	return int(value), nil //nolint:gosec // checked
}

// SafeNewIntFromBigInt constructs Int from big.Int, return error if more than 256bits
func SafeNewIntFromBigInt(i *big.Int) (sdkmath.Int, error) {
	if !IsValidInt256(i) {
		return sdkmath.NewInt(0), fmt.Errorf("big int out of bound: %s", i)
	}
	return sdkmath.NewIntFromBigInt(i), nil
}

// SaturatedNewInt constructs Int from big.Int, truncate if more than 256bits
func SaturatedNewInt(i *big.Int) sdkmath.Int {
	if !IsValidInt256(i) {
		i = MaxInt256
	}
	return sdkmath.NewIntFromBigInt(i)
}

// IsValidInt256 check the bound of 256 bit number
func IsValidInt256(i *big.Int) bool {
	return i == nil || !bigIntOverflows(i)
}

// check if the big int overflows,
// NOTE: copied from cosmos-sdk.
func bigIntOverflows(i *big.Int) bool {
	// overflow is defined as i.BitLen() > MaxBitLen
	// however this check can be expensive when doing many operations.
	// So we first check if the word length is greater than maxWordLen.
	// However the most significant word could be zero, hence we still do the bitlen check.
	if len(i.Bits()) > maxWordLen {
		return i.BitLen() > sdkmath.MaxBitLen
	}
	return false
}

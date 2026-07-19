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
package ante

import (
	"bytes"

	errorsmod "cosmossdk.io/errors"
	sdk "github.com/cosmos/cosmos-sdk/types"
	errortypes "github.com/cosmos/cosmos-sdk/types/errors"
	ethtypes "github.com/ethereum/go-ethereum/core/types"
	"github.com/evmos/ethermint/ante/cache"
	evmtypes "github.com/evmos/ethermint/x/evm/types"
)

// VerifyEthSig checks that the chain id and signer address match the message, using senderCache
// (may be nil) to skip ecrecover on a hash hit.
func VerifyEthSig(tx sdk.Tx, signer ethtypes.Signer, senderCache *cache.SenderCache) error {
	for _, msg := range tx.GetMsgs() {
		msgEthTx, ok := msg.(*evmtypes.MsgEthereumTx)
		if !ok {
			return errorsmod.Wrapf(errortypes.ErrUnknownRequest, "invalid message type %T, expected %T", msg, (*evmtypes.MsgEthereumTx)(nil))
		}

		ethTx := msgEthTx.AsTransaction()
		if ethTx == nil {
			return errorsmod.Wrapf(errortypes.ErrUnknownRequest, "failed to build ethereum tx from msg")
		}

		hash := ethTx.Hash()
		if cached, ok := senderCache.Get(hash); ok {
			if !bytes.Equal(msgEthTx.From, cached.Bytes()) {
				return errorsmod.Wrapf(errortypes.ErrorInvalidSigner,
					"signature verification failed: recovered %s does not match claimed sender %s", cached.Hex(), evmtypes.HexAddress(msgEthTx.From))
			}
			continue
		}

		from, err := msgEthTx.VerifySenderAndGet(signer)
		if err != nil {
			return errorsmod.Wrapf(errortypes.ErrorInvalidSigner, "signature verification failed: %s", err.Error())
		}
		senderCache.Set(hash, from)
	}

	return nil
}

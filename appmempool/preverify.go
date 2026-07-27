package appmempool

import (
	sdk "github.com/cosmos/cosmos-sdk/types"
	ethtypes "github.com/ethereum/go-ethereum/core/types"

	"github.com/evmos/ethermint/ante"
	"github.com/evmos/ethermint/ante/cache"
	ethermint "github.com/evmos/ethermint/types"
	evmtypes "github.com/evmos/ethermint/x/evm/types"
)

// SigPreVerifier pre-verifies a raw tx's signature for app mempool admission. A
// nil error defers to the locked admission path; non-nil rejects early.
type SigPreVerifier func([]byte) error

// NewEVMSigPreVerifier returns a stateless pre-check that rejects pure-EVM txs with
// bad signatures, deferring non-EVM/undecodable/bad-chain-ID txs to the locked path.
func NewEVMSigPreVerifier(chainID string, decoder sdk.TxDecoder, senderCache *cache.SenderCache) SigPreVerifier {
	cid, err := ethermint.ParseChainID(chainID)
	if err != nil {
		return nil
	}
	signer := ethtypes.LatestSignerForChainID(cid)

	return func(raw []byte) error {
		tx, err := decoder(raw)
		if err != nil {
			return nil // let the locked path surface the canonical decode error
		}
		msgs := tx.GetMsgs()
		if len(msgs) == 0 {
			return nil
		}
		for _, msg := range msgs {
			if _, ok := msg.(*evmtypes.MsgEthereumTx); !ok {
				return nil // not a pure EVM tx; the locked path verifies it
			}
		}
		return ante.VerifyEthSig(tx, signer, senderCache)
	}
}

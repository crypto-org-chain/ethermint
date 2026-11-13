package appmempool

import (
	sdk "github.com/cosmos/cosmos-sdk/types"
	ethtypes "github.com/ethereum/go-ethereum/core/types"

	"github.com/evmos/ethermint/ante"
	"github.com/evmos/ethermint/ante/cache"
	ethermint "github.com/evmos/ethermint/types"
	evmtypes "github.com/evmos/ethermint/x/evm/types"
)

// SigPreVerifier pre-verifies a decoded tx's signature for app mempool admission. A
// nil error defers to the locked admission path; non-nil rejects early.
// The tx must be non-nil: the caller handles decode errors.
type SigPreVerifier func(sdk.Tx) error

// NewEVMSigPreVerifier returns a pre-check that rejects pure-EVM txs with bad signatures
// and defers other txs to the locked path. It returns nil for an unparsable chain ID.
func NewEVMSigPreVerifier(chainID string, senderCache *cache.SenderCache) SigPreVerifier {
	cid, err := ethermint.ParseChainID(chainID)
	if err != nil {
		return nil
	}
	signer := ethtypes.LatestSignerForChainID(cid)

	return func(tx sdk.Tx) error {
		msgs := tx.GetMsgs()
		if len(msgs) == 0 {
			return nil
		}
		for _, msg := range msgs {
			ethMsg, ok := msg.(*evmtypes.MsgEthereumTx)
			if !ok {
				return nil // not a pure EVM tx; the locked path verifies it
			}
			// AsTransaction would write Raw, and the tx may be shared; ValidateBasic rejects it on the locked path.
			if ethMsg.Raw.Transaction == nil {
				return nil
			}
		}
		return ante.VerifyEthSig(tx, signer, senderCache)
	}
}

package appmempool

import (
	sdk "github.com/cosmos/cosmos-sdk/types"
	ethtypes "github.com/ethereum/go-ethereum/core/types"

	"github.com/evmos/ethermint/ante"
	ethermint "github.com/evmos/ethermint/types"
	evmtypes "github.com/evmos/ethermint/x/evm/types"
)

// SigPreVerifier pre-verifies a raw tx's signature for app mempool admission. A
// nil error defers to the locked admission path; non-nil rejects early.
type SigPreVerifier func([]byte) error

// NewEVMSigPreVerifier builds a stateless, lock-free signature pre-check. It
// defers (returns nil) on anything it can't cheaply verify — bad chain ID,
// undecodable bytes, no msgs, any non-EVM tx — and rejects only a genuine
// pure-EVM signature failure.
func NewEVMSigPreVerifier(chainID string, decoder sdk.TxDecoder) SigPreVerifier {
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
		return ante.VerifyEthSig(tx, signer)
	}
}

// PreVerifierRegistry collects signature pre-verifiers contributed by modules.
// The app composes them once and runs Verify on the mempool admission path.
type PreVerifierRegistry struct {
	verifiers []SigPreVerifier
}

// Register adds a pre-verifier; nil is ignored.
func (r *PreVerifierRegistry) Register(v SigPreVerifier) {
	if v != nil {
		r.verifiers = append(r.verifiers, v)
	}
}

// Verify runs every registered pre-verifier and returns the first rejection, or
// nil if all defer or pass.
func (r *PreVerifierRegistry) Verify(raw []byte) error {
	for _, v := range r.verifiers {
		if err := v(raw); err != nil {
			return err
		}
	}
	return nil
}

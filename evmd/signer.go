package evmd

import (
	"fmt"

	sdk "github.com/cosmos/cosmos-sdk/types"
	mempool "github.com/cosmos/cosmos-sdk/types/mempool"
	authante "github.com/cosmos/cosmos-sdk/x/auth/ante"
	evmtypes "github.com/evmos/ethermint/x/evm/types"
)

var _ mempool.SignerExtractionAdapter = EthSignerExtractionAdapter{}

// EthSignerExtractionAdapter is the default implementation of SignerExtractionAdapter. It extracts the signers
// from a cosmos-sdk tx via GetSignaturesV2.
type EthSignerExtractionAdapter struct {
	fallback mempool.SignerExtractionAdapter
}

// NewEthSignerExtractionAdapter constructs a new EthSignerExtractionAdapter instance
func NewEthSignerExtractionAdapter(fallback mempool.SignerExtractionAdapter) EthSignerExtractionAdapter {
	return EthSignerExtractionAdapter{fallback}
}

// GetSigners implements the Adapter interface
func (s EthSignerExtractionAdapter) GetSigners(tx sdk.Tx) ([]mempool.SignerData, error) {
	if txWithExtensions, ok := tx.(authante.HasExtensionOptionsTx); ok {
		opts := txWithExtensions.GetExtensionOptions()
		if len(opts) > 0 && opts[0].GetTypeUrl() == "/ethermint.evm.v1.ExtensionOptionsEthereumTx" {
			type innerLaneKey struct {
				signer string
				nonce  uint64
			}

			msgs := tx.GetMsgs()
			signers := make([]mempool.SignerData, 0, len(msgs))
			seen := make(map[innerLaneKey]struct{}, len(msgs))

			for _, msg := range msgs {
				ethMsg, ok := msg.(*evmtypes.MsgEthereumTx)
				if !ok {
					continue
				}

				txData := ethMsg.AsTransaction()
				if txData == nil {
					continue
				}

				from := ethMsg.GetFrom()
				// GetFrom() is safe here because mempool insertion happens after
				// EthSigVerificationDecorator populates msg.From via signer recovery.
				lane := innerLaneKey{
					signer: string(from),
					nonce:  txData.Nonce(),
				}

				if _, duplicate := seen[lane]; duplicate {
					continue
				}
				seen[lane] = struct{}{}

				signers = append(signers, mempool.NewSignerData(from, lane.nonce))
			}

			if len(signers) > 0 {
				return signers, nil
			}
		}
	}

	if s.fallback == nil {
		return nil, fmt.Errorf("fallback signer extraction adapter is nil")
	}

	return s.fallback.GetSigners(tx)
}

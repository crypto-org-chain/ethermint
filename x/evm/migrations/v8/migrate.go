package v8

import (
	sdk "github.com/cosmos/cosmos-sdk/types"
)

// MigrateStore migrates the x/evm module state from consensus version 7 to 8.
// This version enforces EIP-7623 (Prague calldata floor gas) in all execution
// contexts, not just CheckTx. No on-chain state is modified by this migration;
// the version bump signals a consensus-breaking change in validation logic.
func MigrateStore(_ sdk.Context) error {
	return nil
}

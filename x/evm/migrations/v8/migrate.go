package v8

import (
	sdk "github.com/cosmos/cosmos-sdk/types"
)

// MigrateStore migrates the x/evm module state from consensus version 8 to
// version 9. The MaxEthMsgsPerTx field added in v9 defaults to 0, which is
// handled at read time by MaxEthMsgsPerTxOrDefault(), so no data migration is
// required.
func MigrateStore(_ sdk.Context) error {
	return nil
}

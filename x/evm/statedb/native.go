package statedb

import (
	"github.com/cosmos/cosmos-sdk/store/v2/cachemulti"
	"github.com/ethereum/go-ethereum/common"
)

var _ JournalEntry = nativeChange{}

type nativeChange struct {
	previousStore      cachemulti.Store
	previousLayerCount int
	events             int
}

func (native nativeChange) Dirtied() *common.Address {
	return nil
}

func (native nativeChange) Revert(s *StateDB) {
	s.restoreNativeState(native.previousStore, native.previousLayerCount)
	s.nativeEvents = s.nativeEvents[:len(s.nativeEvents)-native.events]
}

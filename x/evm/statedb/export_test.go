package statedb

import "github.com/ethereum/go-ethereum/common"

func (s *StateDB) JournalLen() int { return s.journal.length() }

func (s *StateDB) DirtyStorageLen(addr common.Address) int {
	obj := s.getStateObject(addr)
	if obj == nil {
		return 0
	}
	return len(obj.dirtyStorage)
}

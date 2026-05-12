package statedb

import (
	"bytes"
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/stretchr/testify/require"
)

func TestStorageSortedKeys(t *testing.T) {
	t.Run("empty storage", func(t *testing.T) {
		s := make(Storage)
		keys := s.SortedKeys()
		require.Empty(t, keys)
	})

	t.Run("single entry", func(t *testing.T) {
		s := make(Storage)
		h := common.HexToHash("0x01")
		s[h] = common.HexToHash("0xff")
		keys := s.SortedKeys()
		require.Len(t, keys, 1)
		require.Equal(t, h, keys[0])
	})

	t.Run("multiple entries are sorted", func(t *testing.T) {
		s := make(Storage)
		h1 := common.HexToHash("0x03")
		h2 := common.HexToHash("0x01")
		h3 := common.HexToHash("0x02")
		s[h1] = common.Hash{}
		s[h2] = common.Hash{}
		s[h3] = common.Hash{}

		keys := s.SortedKeys()
		require.Len(t, keys, 3)
		// Verify sorted order
		for i := 1; i < len(keys); i++ {
			require.True(t, bytes.Compare(keys[i-1][:], keys[i][:]) < 0,
				"keys should be in ascending order: %x >= %x", keys[i-1], keys[i])
		}
	})

	t.Run("deterministic across calls", func(t *testing.T) {
		s := make(Storage)
		for i := 0; i < 20; i++ {
			h := common.BigToHash(big.NewInt(int64(i * 7 % 20)))
			s[h] = common.Hash{}
		}
		keys1 := s.SortedKeys()
		keys2 := s.SortedKeys()
		require.Equal(t, keys1, keys2)
	})
}

func TestJournalSortedDirties(t *testing.T) {
	t.Run("empty journal", func(t *testing.T) {
		j := newJournal()
		keys := j.sortedDirties()
		require.Empty(t, keys)
	})

	t.Run("multiple dirties are sorted", func(t *testing.T) {
		j := newJournal()
		a1 := common.HexToAddress("0x03")
		a2 := common.HexToAddress("0x01")
		a3 := common.HexToAddress("0x02")
		j.dirties[a1] = 1
		j.dirties[a2] = 1
		j.dirties[a3] = 1

		keys := j.sortedDirties()
		require.Len(t, keys, 3)
		for i := 1; i < len(keys); i++ {
			require.True(t, bytes.Compare(keys[i-1][:], keys[i][:]) < 0,
				"addresses should be in ascending order: %x >= %x", keys[i-1], keys[i])
		}
	})
}

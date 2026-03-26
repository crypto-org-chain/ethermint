package types

import (
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/stretchr/testify/require"
)

func TestAddressStoragePrefix(t *testing.T) {
	addr := common.HexToAddress("0x1234567890abcdef1234567890abcdef12345678")

	prefix := AddressStoragePrefix(addr)

	// Should be 1 byte prefix + 20 byte address = 21 bytes
	require.Len(t, prefix, 1+common.AddressLength)
	require.Equal(t, byte(prefixStorage), prefix[0])
	require.Equal(t, addr.Bytes(), prefix[1:])
}

func TestStateKey(t *testing.T) {
	addr := common.HexToAddress("0x1234567890abcdef1234567890abcdef12345678")
	key := common.HexToHash("0xdeadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeef")

	stateKey := StateKey(addr, key)

	// Should be 1 byte prefix + 20 byte address + 32 byte hash = 53 bytes
	require.Len(t, stateKey, 1+common.AddressLength+common.HashLength)
	require.Equal(t, byte(prefixStorage), stateKey[0])
	require.Equal(t, addr.Bytes(), stateKey[1:1+common.AddressLength])
	require.Equal(t, key.Bytes(), stateKey[1+common.AddressLength:])
}

func TestStateKeyConsistentWithAddressStoragePrefix(t *testing.T) {
	addr := common.HexToAddress("0x1234567890abcdef1234567890abcdef12345678")
	key := common.HexToHash("0xdeadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeef")

	stateKey := StateKey(addr, key)
	prefix := AddressStoragePrefix(addr)

	// The state key should start with the address storage prefix
	require.Equal(t, prefix, stateKey[:len(prefix)])
}

func TestStateKeyZeroValues(t *testing.T) {
	addr := common.Address{}
	key := common.Hash{}

	stateKey := StateKey(addr, key)

	require.Len(t, stateKey, 1+common.AddressLength+common.HashLength)
	require.Equal(t, byte(prefixStorage), stateKey[0])
}

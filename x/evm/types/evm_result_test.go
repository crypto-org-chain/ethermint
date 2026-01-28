package types

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestEVMResult_ToMsgResponse(t *testing.T) {
	logs := []*Log{
		{
			Address: "0x1234567890abcdef1234567890abcdef12345678",
			Topics:  []string{"0xabcd"},
			Data:    []byte("test data"),
		},
	}
	blockHash := []byte("block_hash_bytes")

	result := &EVMResult{
		Hash:             "0xabc123",
		Logs:             logs,
		Ret:              []byte("return data"),
		VmError:          "",
		GasUsed:          21000,
		BlockHash:        blockHash,
		ExecutionGasUsed: 20000,
	}

	msgResponse := result.ToMsgResponse()

	require.Equal(t, result.Hash, msgResponse.Hash)
	require.Equal(t, result.Logs, msgResponse.Logs)
	require.Equal(t, result.Ret, msgResponse.Ret)
	require.Equal(t, result.VmError, msgResponse.VmError)
	require.Equal(t, result.GasUsed, msgResponse.GasUsed)
	require.Equal(t, result.BlockHash, msgResponse.BlockHash)
}

func TestEVMResult_ToEthCallResponse(t *testing.T) {
	logs := []*Log{
		{
			Address: "0x1234567890abcdef1234567890abcdef12345678",
			Topics:  []string{"0xabcd"},
			Data:    []byte("test data"),
		},
	}
	blockHash := []byte("block_hash_bytes")

	result := &EVMResult{
		Hash:             "0xabc123",
		Logs:             logs,
		Ret:              []byte("return data"),
		VmError:          "execution reverted",
		GasUsed:          21000,
		BlockHash:        blockHash,
		ExecutionGasUsed: 20000,
	}

	ethCallResponse := result.ToEthCallResponse()

	require.Equal(t, result.Hash, ethCallResponse.Hash)
	require.Equal(t, result.Logs, ethCallResponse.Logs)
	require.Equal(t, result.Ret, ethCallResponse.Ret)
	require.Equal(t, result.VmError, ethCallResponse.VmError)
	require.Equal(t, result.GasUsed, ethCallResponse.GasUsed)
	require.Equal(t, result.BlockHash, ethCallResponse.BlockHash)
}

func TestEVMResult_Failed(t *testing.T) {
	testCases := []struct {
		name     string
		vmError  string
		expected bool
	}{
		{
			name:     "no error - success",
			vmError:  "",
			expected: false,
		},
		{
			name:     "with error - failed",
			vmError:  "execution reverted",
			expected: true,
		},
		{
			name:     "out of gas error",
			vmError:  "out of gas",
			expected: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := &EVMResult{
				VmError: tc.vmError,
			}
			require.Equal(t, tc.expected, result.Failed())
		})
	}
}

func TestEVMResult_ToMsgResponse_NilLogs(t *testing.T) {
	result := &EVMResult{
		Hash:             "0xabc123",
		Logs:             nil,
		Ret:              nil,
		VmError:          "",
		GasUsed:          21000,
		BlockHash:        nil,
		ExecutionGasUsed: 20000,
	}

	msgResponse := result.ToMsgResponse()

	require.Equal(t, result.Hash, msgResponse.Hash)
	require.Nil(t, msgResponse.Logs)
	require.Nil(t, msgResponse.Ret)
	require.Equal(t, result.VmError, msgResponse.VmError)
	require.Equal(t, result.GasUsed, msgResponse.GasUsed)
	require.Nil(t, msgResponse.BlockHash)
}

func TestEVMResult_ToEthCallResponse_NilLogs(t *testing.T) {
	result := &EVMResult{
		Hash:             "0xabc123",
		Logs:             nil,
		Ret:              nil,
		VmError:          "",
		GasUsed:          21000,
		BlockHash:        nil,
		ExecutionGasUsed: 20000,
	}

	ethCallResponse := result.ToEthCallResponse()

	require.Equal(t, result.Hash, ethCallResponse.Hash)
	require.Nil(t, ethCallResponse.Logs)
	require.Nil(t, ethCallResponse.Ret)
	require.Equal(t, result.VmError, ethCallResponse.VmError)
	require.Equal(t, result.GasUsed, ethCallResponse.GasUsed)
	require.Nil(t, ethCallResponse.BlockHash)
}

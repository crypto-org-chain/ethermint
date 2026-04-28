package types

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParamsMaxEthMsgsPerTxValidate(t *testing.T) {
	testCases := []struct {
		name    string
		value   uint32
		expPass bool
	}{
		{name: "zero sentinel", value: 0, expPass: true},
		{name: "lower bound", value: 1, expPass: true},
		{name: "upper bound", value: MaxMaxEthMsgsPerTx, expPass: true},
		{name: "over hard ceiling", value: MaxMaxEthMsgsPerTx + 1, expPass: false},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			params := DefaultParams()
			params.MaxEthMsgsPerTx = tc.value

			err := params.Validate()
			if tc.expPass {
				require.NoError(t, err)
			} else {
				require.Error(t, err)
			}
		})
	}
}

func TestParamsMaxEthMsgsPerTxDefault(t *testing.T) {
	params := DefaultParams()
	params.MaxEthMsgsPerTx = 0
	require.Equal(t, DefaultMaxEthMsgsPerTx, params.MaxEthMsgsPerTxOrDefault())

	params.MaxEthMsgsPerTx = 7
	require.Equal(t, uint32(7), params.MaxEthMsgsPerTxOrDefault())
}

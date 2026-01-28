package config

import (
	"testing"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/require"
)

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()
	require.True(t, cfg.JSONRPC.Enable)
	require.Equal(t, cfg.JSONRPC.Address, DefaultJSONRPCAddress)
	require.Equal(t, cfg.JSONRPC.WsAddress, DefaultJSONRPCWsAddress)
	require.Equal(t, cfg.JSONRPC.AllowUnprotectedTxs, DefaultAllowUnprotectedTxs)
}

func TestGetConfig_AllowUnprotectedTxs(t *testing.T) {
	tests := []struct {
		name     string
		viperVal interface{}
		expected bool
	}{
		{
			name:     "allow unprotected txs enabled",
			viperVal: true,
			expected: true,
		},
		{
			name:     "allow unprotected txs disabled",
			viperVal: false,
			expected: false,
		},
		{
			name:     "allow unprotected txs not set (default)",
			viperVal: nil,
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v := viper.New()
			// Set the test value
			if tt.viperVal != nil {
				v.Set("json-rpc.allow-unprotected-txs", tt.viperVal)
			}

			cfg, err := GetConfig(v)
			require.NoError(t, err)
			require.Equal(t, tt.expected, cfg.JSONRPC.AllowUnprotectedTxs)
		})
	}
}

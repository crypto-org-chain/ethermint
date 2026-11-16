package config

import (
	"testing"
	"time"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/require"
)

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()
	require.True(t, cfg.JSONRPC.Enable)
	require.Equal(t, cfg.JSONRPC.Address, DefaultJSONRPCAddress)
	require.Equal(t, cfg.JSONRPC.WsAddress, DefaultJSONRPCWsAddress)
	require.Empty(t, cfg.JSONRPC.WsOrigins)
	require.Equal(t, cfg.JSONRPC.AllowUnprotectedTxs, DefaultAllowUnprotectedTxs)
	require.Equal(t, DefaultBatchRequestLimit, cfg.JSONRPC.BatchRequestLimit)
	require.Equal(t, DefaultBatchResponseMaxSize, cfg.JSONRPC.BatchResponseMaxSize)
}

func TestGetConfig_BatchLimits(t *testing.T) {
	v := viper.New()
	v.Set("json-rpc.batch-request-limit", 500)
	v.Set("json-rpc.batch-response-max-size", 10_000_000)

	cfg, err := GetConfig(v)
	require.NoError(t, err)
	require.Equal(t, 500, cfg.JSONRPC.BatchRequestLimit)
	require.Equal(t, 10_000_000, cfg.JSONRPC.BatchResponseMaxSize)
}

func TestGetConfig_BatchLimitsDefaultWhenUnset(t *testing.T) {
	v := viper.New()
	cfg, err := GetConfig(v)
	require.NoError(t, err)
	require.Equal(t, DefaultBatchRequestLimit, cfg.JSONRPC.BatchRequestLimit)
	require.Equal(t, DefaultBatchResponseMaxSize, cfg.JSONRPC.BatchResponseMaxSize)
}

func TestGetConfig_BatchLimitsExplicitZero(t *testing.T) {
	v := viper.New()
	v.Set("json-rpc.batch-request-limit", 0)
	v.Set("json-rpc.batch-response-max-size", 0)

	cfg, err := GetConfig(v)
	require.NoError(t, err)
	require.Equal(t, 0, cfg.JSONRPC.BatchRequestLimit)
	require.Equal(t, 0, cfg.JSONRPC.BatchResponseMaxSize)
}

func TestJSONRPCConfig_Validate_BatchLimits(t *testing.T) {
	t.Run("rejectsNegativeBatchRequestLimit", func(t *testing.T) {
		cfg := DefaultJSONRPCConfig()
		cfg.BatchRequestLimit = -1
		require.ErrorContains(t, cfg.Validate(), "batch request limit cannot be negative")
	})

	t.Run("rejectsNegativeBatchResponseMaxSize", func(t *testing.T) {
		cfg := DefaultJSONRPCConfig()
		cfg.BatchResponseMaxSize = -1
		require.ErrorContains(t, cfg.Validate(), "batch response max size cannot be negative")
	})

	t.Run("allowsZeroToDisableLimits", func(t *testing.T) {
		cfg := DefaultJSONRPCConfig()
		cfg.BatchRequestLimit = 0
		cfg.BatchResponseMaxSize = 0
		require.NoError(t, cfg.Validate())
	})
}

func TestGetConfig_ReceiptWaitTimeouts(t *testing.T) {
	cfg, err := GetConfig(viper.New())
	require.NoError(t, err, "existing app.toml files without the keys must keep working")
	require.Equal(t, DefaultReceiptWaitTimeout, cfg.JSONRPC.ReceiptWaitTimeout)
	require.Equal(t, DefaultReceiptWaitMaxTimeout, cfg.JSONRPC.ReceiptWaitMaxTimeout)

	v := viper.New()
	v.Set("json-rpc.receipt-wait-timeout", "3s")
	v.Set("json-rpc.receipt-wait-max-timeout", "10s")
	cfg, err = GetConfig(v)
	require.NoError(t, err)
	require.Equal(t, 3*time.Second, cfg.JSONRPC.ReceiptWaitTimeout)
	require.Equal(t, 10*time.Second, cfg.JSONRPC.ReceiptWaitMaxTimeout)
}

func TestJSONRPCConfig_Validate_ReceiptWait(t *testing.T) {
	cfg := DefaultJSONRPCConfig()
	cfg.ReceiptWaitTimeout = -1
	require.ErrorContains(t, cfg.Validate(), "receipt wait timeout must be positive")

	cfg = DefaultJSONRPCConfig()
	cfg.ReceiptWaitMaxTimeout = -1
	require.ErrorContains(t, cfg.Validate(), "receipt wait max timeout must be positive")

	cfg = DefaultJSONRPCConfig()
	cfg.ReceiptWaitMaxTimeout = 0
	require.ErrorContains(t, cfg.Validate(), "receipt wait max timeout must be positive")

	// Lowering only the max must not stop the node; the default is clamped at runtime.
	cfg = DefaultJSONRPCConfig()
	cfg.ReceiptWaitMaxTimeout = 10 * time.Second
	require.NoError(t, cfg.Validate())

	cfg = DefaultJSONRPCConfig()
	cfg.MaxReceiptWaiters = -1
	require.ErrorContains(t, cfg.Validate(), "max receipt waiters cannot be negative")
}

func TestGetConfig_MaxReceiptWaiters(t *testing.T) {
	cfg, err := GetConfig(viper.New())
	require.NoError(t, err)
	require.Equal(t, DefaultMaxReceiptWaiters, cfg.JSONRPC.MaxReceiptWaiters)

	v := viper.New()
	v.Set("json-rpc.max-receipt-waiters", 0)
	cfg, err = GetConfig(v)
	require.NoError(t, err)
	require.Equal(t, 0, cfg.JSONRPC.MaxReceiptWaiters)
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

func TestJSONRPCWsOriginsValidation(t *testing.T) {
	newConfig := func() *JSONRPCConfig {
		return DefaultJSONRPCConfig()
	}

	t.Run("allowsEmpty", func(t *testing.T) {
		cfg := newConfig()
		cfg.WsOrigins = nil
		require.NoError(t, cfg.Validate())
	})

	t.Run("allowsStar", func(t *testing.T) {
		cfg := newConfig()
		cfg.WsOrigins = []string{"*"}
		require.NoError(t, cfg.Validate())
	})

	t.Run("rejectsStarMixedWithOthers", func(t *testing.T) {
		cfg := newConfig()
		cfg.WsOrigins = []string{"*", "http://example.com"}
		require.Error(t, cfg.Validate())
	})

	t.Run("rejectsInvalidOrigin", func(t *testing.T) {
		cfg := newConfig()
		cfg.WsOrigins = []string{"not a url"}
		require.Error(t, cfg.Validate())
	})

	t.Run("allowsDuplicateOriginMixedCase", func(t *testing.T) {
		cfg := newConfig()
		cfg.WsOrigins = []string{"HTTP://Example.COM", "http://example.com/"}
		require.NoError(t, cfg.Validate())
	})

	t.Run("ignoresWhitespaceOnly", func(t *testing.T) {
		cfg := newConfig()
		cfg.WsOrigins = []string{"  \t "}
		require.NoError(t, cfg.Validate())
	})
}

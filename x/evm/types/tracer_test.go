package types

import (
	"testing"

	"github.com/ethereum/go-ethereum/core"
	"github.com/ethereum/go-ethereum/core/tracing"
	"github.com/ethereum/go-ethereum/params"
	"github.com/stretchr/testify/require"
)

func TestNewTracerDefaultIsNil(t *testing.T) {
	require.Nil(t, NewTracer("", core.Message{}, params.TestChainConfig, 1, 0))
}

func TestNewNoOpTracer(t *testing.T) {
	tracer := NewNoOpTracer()
	require.IsType(t, &tracing.Hooks{}, tracer)
	require.NotNil(t, tracer)
}

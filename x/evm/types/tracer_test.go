package types

import (
	"testing"

	"github.com/ethereum/go-ethereum/core/tracing"
	"github.com/stretchr/testify/require"
)

func TestNewNoOpTracer(t *testing.T) {
	tracer := NewNoOpTracer()
	require.IsType(t, &tracing.Hooks{}, tracer)
	require.NotNil(t, tracer)
}

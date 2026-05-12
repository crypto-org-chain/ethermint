package types

import (
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	ethtypes "github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/core/vm"
	"github.com/stretchr/testify/require"
)

var (
	simTracerBlockHash = common.HexToHash("0xaabbccdd")
	simTracerTxHash    = common.HexToHash("0x11223344")
	simTracerFrom      = common.HexToAddress("0x1111111111111111111111111111111111111111")
	simTracerTo        = common.HexToAddress("0x2222222222222222222222222222222222222222")
)

// ---------------------------------------------------------------------------
// Basic log capture
// ---------------------------------------------------------------------------

func TestSimTracer_CapturesLogs(t *testing.T) {
	tracer := NewSimTracer(false, 1, simTracerBlockHash, simTracerTxHash, 0)
	hooks := tracer.Hooks()

	// Simulate entering a call frame.
	hooks.OnEnter(0, byte(vm.CALL), simTracerFrom, simTracerTo, nil, 21000, big.NewInt(0))

	// Emit a log.
	hooks.OnLog(&ethtypes.Log{
		Address: simTracerTo,
		Topics:  []common.Hash{common.HexToHash("0xdeadbeef")},
		Data:    []byte{0x01},
	})

	// Exit the call frame without revert.
	hooks.OnExit(0, nil, 21000, nil, false)

	logs := tracer.Logs()
	require.Len(t, logs, 1)
	require.Equal(t, simTracerTo, logs[0].Address)
	require.Equal(t, simTracerBlockHash, logs[0].BlockHash)
	require.Equal(t, simTracerTxHash, logs[0].TxHash)
}

// ---------------------------------------------------------------------------
// Reverted call clears logs
// ---------------------------------------------------------------------------

func TestSimTracer_RevertClearsLogs(t *testing.T) {
	tracer := NewSimTracer(false, 1, simTracerBlockHash, simTracerTxHash, 0)
	hooks := tracer.Hooks()

	hooks.OnEnter(0, byte(vm.CALL), simTracerFrom, simTracerTo, nil, 21000, big.NewInt(0))
	hooks.OnLog(&ethtypes.Log{Address: simTracerTo})
	// Revert the top-level call.
	hooks.OnExit(0, nil, 21000, vm.ErrExecutionReverted, true)

	logs := tracer.Logs()
	require.Empty(t, logs)
}

// ---------------------------------------------------------------------------
// Nested call: inner revert should NOT propagate logs to parent
// ---------------------------------------------------------------------------

func TestSimTracer_NestedRevertDoesNotPropagate(t *testing.T) {
	tracer := NewSimTracer(false, 1, simTracerBlockHash, simTracerTxHash, 0)
	hooks := tracer.Hooks()

	// Outer call.
	hooks.OnEnter(0, byte(vm.CALL), simTracerFrom, simTracerTo, nil, 100000, big.NewInt(0))

	// Inner call.
	hooks.OnEnter(1, byte(vm.CALL), simTracerTo, simTracerFrom, nil, 50000, big.NewInt(0))
	hooks.OnLog(&ethtypes.Log{Address: simTracerFrom, Topics: []common.Hash{{0x01}}})
	// Inner call reverts – its log should be dropped.
	hooks.OnExit(1, nil, 50000, vm.ErrExecutionReverted, true)

	// Outer call emits a log.
	hooks.OnLog(&ethtypes.Log{Address: simTracerTo, Topics: []common.Hash{{0x02}}})
	hooks.OnExit(0, nil, 100000, nil, false)

	logs := tracer.Logs()
	require.Len(t, logs, 1)
	require.Equal(t, common.Hash{0x02}, logs[0].Topics[0])
}

// ---------------------------------------------------------------------------
// Nested call success: inner logs propagate to parent
// ---------------------------------------------------------------------------

func TestSimTracer_NestedSuccessPropagate(t *testing.T) {
	tracer := NewSimTracer(false, 1, simTracerBlockHash, simTracerTxHash, 0)
	hooks := tracer.Hooks()

	hooks.OnEnter(0, byte(vm.CALL), simTracerFrom, simTracerTo, nil, 100000, big.NewInt(0))
	hooks.OnEnter(1, byte(vm.CALL), simTracerTo, simTracerFrom, nil, 50000, big.NewInt(0))
	hooks.OnLog(&ethtypes.Log{Address: simTracerFrom, Topics: []common.Hash{{0x01}}})
	hooks.OnExit(1, nil, 50000, nil, false) // inner succeeds

	hooks.OnLog(&ethtypes.Log{Address: simTracerTo, Topics: []common.Hash{{0x02}}})
	hooks.OnExit(0, nil, 100000, nil, false) // outer succeeds

	logs := tracer.Logs()
	require.Len(t, logs, 2)
}

// ---------------------------------------------------------------------------
// Transfer tracing (traceTransfers = true)
// ---------------------------------------------------------------------------

func TestSimTracer_TraceTransfers_Enabled(t *testing.T) {
	tracer := NewSimTracer(true, 1, simTracerBlockHash, simTracerTxHash, 0)
	hooks := tracer.Hooks()

	value := big.NewInt(1e18)
	hooks.OnEnter(0, byte(vm.CALL), simTracerFrom, simTracerTo, nil, 21000, value)
	hooks.OnExit(0, nil, 21000, nil, false)

	logs := tracer.Logs()
	// Expect exactly one synthetic transfer log (ERC-7528).
	require.Len(t, logs, 1)
	require.Equal(t, common.HexToAddress("0xEeeeeEeeeEeEeeEeEeEeeEEEeeeeEeeeeeeeEEeE"), logs[0].Address)
	require.Len(t, logs[0].Topics, 3)
}

func TestSimTracer_TraceTransfers_Disabled(t *testing.T) {
	tracer := NewSimTracer(false, 1, simTracerBlockHash, simTracerTxHash, 0)
	hooks := tracer.Hooks()

	value := big.NewInt(1e18)
	hooks.OnEnter(0, byte(vm.CALL), simTracerFrom, simTracerTo, nil, 21000, value)
	hooks.OnExit(0, nil, 21000, nil, false)

	// No logs expected – transfer tracing is disabled.
	logs := tracer.Logs()
	require.Empty(t, logs)
}

func TestSimTracer_TraceTransfers_ZeroValue(t *testing.T) {
	tracer := NewSimTracer(true, 1, simTracerBlockHash, simTracerTxHash, 0)
	hooks := tracer.Hooks()

	// Zero value transfer – no synthetic log expected.
	hooks.OnEnter(0, byte(vm.CALL), simTracerFrom, simTracerTo, nil, 21000, big.NewInt(0))
	hooks.OnExit(0, nil, 21000, nil, false)

	require.Empty(t, tracer.Logs())
}

func TestSimTracer_TraceTransfers_DelegateCall(t *testing.T) {
	tracer := NewSimTracer(true, 1, simTracerBlockHash, simTracerTxHash, 0)
	hooks := tracer.Hooks()

	// DELEGATECALL with non-zero value should NOT emit a transfer log.
	hooks.OnEnter(0, byte(vm.DELEGATECALL), simTracerFrom, simTracerTo, nil, 21000, big.NewInt(1e18))
	hooks.OnExit(0, nil, 21000, nil, false)

	require.Empty(t, tracer.Logs())
}

// ---------------------------------------------------------------------------
// Reset
// ---------------------------------------------------------------------------

func TestSimTracer_Reset(t *testing.T) {
	tracer := NewSimTracer(false, 1, simTracerBlockHash, simTracerTxHash, 0)
	hooks := tracer.Hooks()

	hooks.OnEnter(0, byte(vm.CALL), simTracerFrom, simTracerTo, nil, 21000, big.NewInt(0))
	hooks.OnLog(&ethtypes.Log{Address: simTracerTo})
	hooks.OnExit(0, nil, 21000, nil, false)
	require.Len(t, tracer.Logs(), 1)

	newTxHash := common.HexToHash("0x99887766")
	tracer.Reset(newTxHash, 1)
	require.Empty(t, tracer.Logs())
}

// ---------------------------------------------------------------------------
// Log index counter across multiple calls
// ---------------------------------------------------------------------------

func TestSimTracer_LogIndexIncreases(t *testing.T) {
	tracer := NewSimTracer(false, 1, simTracerBlockHash, simTracerTxHash, 0)
	hooks := tracer.Hooks()

	hooks.OnEnter(0, byte(vm.CALL), simTracerFrom, simTracerTo, nil, 21000, big.NewInt(0))
	hooks.OnLog(&ethtypes.Log{Address: simTracerTo})
	hooks.OnLog(&ethtypes.Log{Address: simTracerTo})
	hooks.OnExit(0, nil, 21000, nil, false)

	logs := tracer.Logs()
	require.Len(t, logs, 2)
	require.Equal(t, uint(0), logs[0].Index)
	require.Equal(t, uint(1), logs[1].Index)
}

func TestSimTracer_ResetKeepsCumulativeLogIndex(t *testing.T) {
	tracer := NewSimTracer(false, 1, simTracerBlockHash, simTracerTxHash, 0)
	hooks := tracer.Hooks()

	hooks.OnEnter(0, byte(vm.CALL), simTracerFrom, simTracerTo, nil, 21000, big.NewInt(0))
	hooks.OnLog(&ethtypes.Log{Address: simTracerTo})
	hooks.OnExit(0, nil, 21000, nil, false)
	require.Equal(t, uint(0), tracer.Logs()[0].Index)

	tracer.Reset(common.HexToHash("0x55"), 1)
	hooks.OnEnter(0, byte(vm.CALL), simTracerFrom, simTracerTo, nil, 21000, big.NewInt(0))
	hooks.OnLog(&ethtypes.Log{Address: simTracerTo})
	hooks.OnExit(0, nil, 21000, nil, false)

	require.Equal(t, uint(1), tracer.Logs()[0].Index)
}

// ---------------------------------------------------------------------------
// No logs returns nil when no frames
// ---------------------------------------------------------------------------

func TestSimTracer_NoFrames_ReturnsNilLogs(t *testing.T) {
	tracer := NewSimTracer(false, 1, simTracerBlockHash, simTracerTxHash, 0)
	require.Nil(t, tracer.Logs())
}

// ---------------------------------------------------------------------------
// onExit depth>0 with no nested frames (size <= 1 early return)
// ---------------------------------------------------------------------------

func TestSimTracer_OnExit_DepthNonzero_SizeOne(t *testing.T) {
	tracer := NewSimTracer(false, 1, simTracerBlockHash, simTracerTxHash, 0)
	hooks := tracer.Hooks()

	// Enter the top-level call (depth 0): t.logs has 1 element.
	hooks.OnEnter(0, byte(vm.CALL), simTracerFrom, simTracerTo, nil, 21000, big.NewInt(0))

	// Exit at depth=1 without a prior OnEnter(1,...): size == 1 → early return.
	hooks.OnExit(1, nil, 21000, nil, false)

	// Top-level exit.
	hooks.OnExit(0, nil, 21000, nil, false)

	require.Empty(t, tracer.Logs())
}

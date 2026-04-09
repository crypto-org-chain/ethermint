package types

import (
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/tracing"
	ethtypes "github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/core/vm"
)

var (
	// keccak256("Transfer(address,address,uint256)")
	transferTopic = common.HexToHash("ddf252ad1be2c89b69c2b068fc378daa952ba7f163c4a11628f55a4df523b3ef")
	// ERC-7528: synthetic address for traced ETH transfers.
	transferAddress = common.HexToAddress("0xEeeeeEeeeEeEeeEeEeEeeEEEeeeeEeeeeeeeEEeE")
	// TransferTraceAddress is exported so callers can filter traced
	// transfer logs out of receipt blooms.
	TransferTraceAddress = transferAddress
)

// SimTracer is a simple tracer that records all logs and
// ether transfers. Transfers are recorded as if they
// were logs following the ERC-7528 standard.
type SimTracer struct {
	// logs keeps logs for all open call frames.
	// This lets us clear logs for failed calls.
	logs           [][]*ethtypes.Log
	count          uint
	traceTransfers bool
	blockNumber    uint64
	blockHash      common.Hash
	txHash         common.Hash
	txIdx          uint
}

// NewSimTracer creates a new tracer for simulation.
func NewSimTracer(traceTransfers bool, blockNumber uint64, blockHash, txHash common.Hash, txIndex uint) *SimTracer {
	return &SimTracer{
		traceTransfers: traceTransfers,
		blockNumber:    blockNumber,
		blockHash:      blockHash,
		txHash:         txHash,
		txIdx:          txIndex,
	}
}

// Hooks returns the tracing hooks for this tracer.
func (t *SimTracer) Hooks() *tracing.Hooks {
	return &tracing.Hooks{
		OnEnter: t.onEnter,
		OnExit:  t.onExit,
		OnLog:   t.onLog,
	}
}

func (t *SimTracer) onEnter(depth int, typ byte, from common.Address, to common.Address, input []byte, gas uint64, value *big.Int) {
	t.logs = append(t.logs, make([]*ethtypes.Log, 0))
	if vm.OpCode(typ) != vm.DELEGATECALL && value != nil && value.Cmp(common.Big0) > 0 {
		t.captureTransfer(from, to, value)
	}
}

func (t *SimTracer) onExit(depth int, output []byte, gasUsed uint64, err error, reverted bool) {
	if depth == 0 {
		t.onEnd(reverted)
		return
	}
	size := len(t.logs)
	if size <= 1 {
		return
	}
	// pop call
	call := t.logs[size-1]
	t.logs = t.logs[:size-1]
	size--

	// Clear logs if call failed.
	if !reverted {
		t.logs[size-1] = append(t.logs[size-1], call...)
	}
}

func (t *SimTracer) onEnd(reverted bool) {
	if reverted && len(t.logs) > 0 {
		t.logs[0] = nil
	}
}

func (t *SimTracer) onLog(log *ethtypes.Log) {
	t.captureLog(log.Address, log.Topics, log.Data)
}

func (t *SimTracer) captureLog(address common.Address, topics []common.Hash, data []byte) {
	t.logs[len(t.logs)-1] = append(t.logs[len(t.logs)-1], &ethtypes.Log{
		Address:     address,
		Topics:      topics,
		Data:        data,
		BlockNumber: t.blockNumber,
		BlockHash:   t.blockHash,
		TxHash:      t.txHash,
		TxIndex:     t.txIdx,
		Index:       t.count,
	})
	t.count++
}

func (t *SimTracer) captureTransfer(from, to common.Address, value *big.Int) {
	if !t.traceTransfers {
		return
	}
	topics := []common.Hash{
		transferTopic,
		common.BytesToHash(from.Bytes()),
		common.BytesToHash(to.Bytes()),
	}
	t.captureLog(transferAddress, topics, common.BigToHash(value).Bytes())
}

// Reset prepares the tracer for the next transaction.
func (t *SimTracer) Reset(txHash common.Hash, txIdx uint) {
	t.logs = nil
	t.txHash = txHash
	t.txIdx = txIdx
}

// Logs returns the final logs for the current transaction.
func (t *SimTracer) Logs() []*ethtypes.Log {
	if len(t.logs) == 0 {
		return nil
	}
	return t.logs[0]
}

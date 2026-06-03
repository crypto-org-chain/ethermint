package types

import (
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"math"
	"math/big"
	"slices"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/core/tracing"
	ethtypes "github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/core/vm"
	"github.com/ethereum/go-ethereum/params"
	ethermint "github.com/evmos/ethermint/types"
	"github.com/evmos/ethermint/x/evm/statedb"
	"github.com/holiman/uint256"
)

const (
	// MaxSimulateBlocks is the maximum number of blocks that can be simulated.
	MaxSimulateBlocks = 256
	// TimestampIncrement is the default increment between block timestamps.
	TimestampIncrement = 1
)

// SimOpts are the inputs to eth_simulateV1.
type SimOpts struct {
	BlockStateCalls        []SimBlock `json:"blockStateCalls"`
	TraceTransfers         bool       `json:"traceTransfers"`
	Validation             bool       `json:"validation"`
	ReturnFullTransactions bool       `json:"returnFullTransactions"`
}

// SimulateV1Args is the internal keeper payload for eth_simulateV1.
// It includes the user RPC options plus the resolved canonical base header.
type SimulateV1Args struct {
	Opts       SimOpts          `json:"opts"`
	BaseHeader *ethtypes.Header `json:"baseHeader"`
}

// SimBlock is a batch of calls to be simulated sequentially.
type SimBlock struct {
	BlockOverrides *SimBlockOverrides `json:"blockOverrides"`
	StateOverrides *SimStateOverride  `json:"stateOverrides"`
	Calls          []json.RawMessage  `json:"calls"`
}

// SimBlockOverrides extends BlockOverrides with additional fields for simulation.
type SimBlockOverrides struct {
	Number        *hexutil.Big          `json:"number"`
	Difficulty    *hexutil.Big          `json:"difficulty"`
	Time          *hexutil.Uint64       `json:"time"`
	GasLimit      *hexutil.Uint64       `json:"gasLimit"`
	FeeRecipient  *common.Address       `json:"feeRecipient"`
	PrevRandao    *common.Hash          `json:"prevRandao"`
	BaseFeePerGas *hexutil.Big          `json:"baseFeePerGas"`
	BlobBaseFee   *hexutil.Big          `json:"blobBaseFee"`
	BeaconRoot    *common.Hash          `json:"beaconRoot"`
	Withdrawals   *ethtypes.Withdrawals `json:"withdrawals"`
}

// MakeHeader returns a new header object with the overridden fields.
func (o *SimBlockOverrides) MakeHeader(header *ethtypes.Header) *ethtypes.Header {
	if o == nil {
		return header
	}
	h := ethtypes.CopyHeader(header)
	if o.Number != nil {
		h.Number = o.Number.ToInt()
	}
	if o.Difficulty != nil {
		h.Difficulty = o.Difficulty.ToInt()
	}
	if o.Time != nil {
		h.Time = uint64(*o.Time)
	}
	if o.GasLimit != nil {
		h.GasLimit = uint64(*o.GasLimit)
	}
	if o.FeeRecipient != nil {
		h.Coinbase = *o.FeeRecipient
	}
	if o.PrevRandao != nil {
		h.MixDigest = *o.PrevRandao
	}
	if o.BaseFeePerGas != nil {
		h.BaseFee = o.BaseFeePerGas.ToInt()
	}
	return h
}

// SimOverrideAccount indicates the overriding fields of account during simulation.
// Extends OverrideAccount with MovePrecompileTo support.
type SimOverrideAccount struct {
	Nonce            *hexutil.Uint64             `json:"nonce"`
	Code             *hexutil.Bytes              `json:"code"`
	Balance          *hexutil.Big                `json:"balance"`
	State            map[common.Hash]common.Hash `json:"state"`
	StateDiff        map[common.Hash]common.Hash `json:"stateDiff"`
	MovePrecompileTo *common.Address             `json:"movePrecompileToAddress"`
}

// SimStateOverride is the collection of overridden accounts for simulation.
type SimStateOverride map[common.Address]SimOverrideAccount

func (diff *SimStateOverride) has(address common.Address) bool {
	_, ok := (*diff)[address]
	return ok
}

// Apply overrides the fields of specified accounts into the given state.
// Supports precompile moves via MovePrecompileTo.
func (diff *SimStateOverride) Apply(stateDB *statedb.StateDB, precompiles vm.PrecompiledContracts) error {
	if diff == nil {
		return nil
	}
	// Iterate in deterministic order.
	addrs := slices.SortedFunc(maps.Keys(*diff), common.Address.Cmp)

	// Tracks destinations of precompiles that were moved.
	dirtyAddrs := make(map[common.Address]struct{})
	for _, addr := range addrs {
		account := (*diff)[addr]
		// If a precompile was moved to this address already, it can't be overridden.
		if _, ok := dirtyAddrs[addr]; ok {
			return &InvalidParamsError{Message: fmt.Sprintf("account %s has already been overridden by a precompile", addr.Hex())}
		}
		p, isPrecompile := precompiles[addr]
		if account.MovePrecompileTo != nil {
			if !isPrecompile {
				return &ServerError{Message: fmt.Sprintf("account %s is not a precompile", addr.Hex())}
			}
			if diff.has(*account.MovePrecompileTo) {
				return &InvalidParamsError{Message: fmt.Sprintf("account %s is already overridden", account.MovePrecompileTo.Hex())}
			}
			precompiles[*account.MovePrecompileTo] = p
			dirtyAddrs[*account.MovePrecompileTo] = struct{}{}
		}
		if isPrecompile {
			delete(precompiles, addr)
		}
		// Override account nonce.
		if account.Nonce != nil {
			stateDB.SetNonce(addr, uint64(*account.Nonce), tracing.NonceChangeUnspecified)
		}
		// Override account(contract) code.
		if account.Code != nil {
			stateDB.SetCode(addr, *account.Code, tracing.CodeChangeUnspecified)
		}
		// Override account balance.
		if account.Balance != nil {
			u256Balance, overflow := uint256.FromBig((*big.Int)(account.Balance))
			if overflow {
				return &InvalidParamsError{Message: fmt.Sprintf("account %s balance overflows uint256", addr.Hex())}
			}
			stateDB.SetBalance(addr, *u256Balance)
		}
		if account.State != nil && account.StateDiff != nil {
			return &InvalidParamsError{Message: fmt.Sprintf("account %s has both 'state' and 'stateDiff'", addr.Hex())}
		}
		// Replace entire state if caller requires.
		if account.State != nil {
			stateDB.SetStorage(addr, account.State)
		}
		// Apply state diff into specified accounts.
		if account.StateDiff != nil {
			for key, value := range account.StateDiff {
				stateDB.SetState(addr, key, value)
			}
		}
	}
	// Finalize overrides so they behave as if applied in a preceding transaction.
	//nolint:misspell
	stateDB.Finalise(false)
	return nil
}

// SimLog reuses ethtypes.Log JSON encoding and injects blockTimestamp until the
// pinned go-ethereum fork carries BlockTimestamp on ethtypes.Log.
type SimLog struct {
	ethtypes.Log
	BlockTimestamp hexutil.Uint64
}

// NewSimLog converts an ethtypes.Log and block timestamp into a SimLog.
func NewSimLog(l *ethtypes.Log, blockTimestamp uint64) *SimLog {
	log := *l
	if log.Topics == nil {
		log.Topics = []common.Hash{}
	}
	return &SimLog{
		Log:            log,
		BlockTimestamp: hexutil.Uint64(blockTimestamp),
	}
}

// MarshalJSON preserves the upstream log JSON shape and adds blockTimestamp for
// eth_simulateV1, avoiding a local copy of every ethtypes.Log field.
func (l *SimLog) MarshalJSON() ([]byte, error) {
	bz, err := json.Marshal(l.Log)
	if err != nil {
		return nil, err
	}
	fields := make(map[string]json.RawMessage)
	if err := json.Unmarshal(bz, &fields); err != nil {
		return nil, err
	}
	blockTimestamp, err := json.Marshal(l.BlockTimestamp)
	if err != nil {
		return nil, err
	}
	fields["blockTimestamp"] = blockTimestamp
	return json.Marshal(fields)
}

// SimCallResult is the result of a simulated call.
type SimCallResult struct {
	ReturnValue hexutil.Bytes  `json:"returnData"`
	Logs        []*SimLog      `json:"logs"`
	GasUsed     hexutil.Uint64 `json:"gasUsed"`
	MaxUsedGas  hexutil.Uint64 `json:"maxUsedGas"`
	Status      hexutil.Uint64 `json:"status"`
	Error       *CallError     `json:"error,omitempty"`
}

// MarshalJSON ensures logs is an empty array instead of nil when empty.
func (r *SimCallResult) MarshalJSON() ([]byte, error) {
	type callResultAlias SimCallResult
	if r.Logs == nil {
		r.Logs = []*SimLog{}
	}
	return json.Marshal((*callResultAlias)(r))
}

// SimBlockResult is the result of a simulated block.
type SimBlockResult struct {
	FullTx      bool
	ChainConfig *params.ChainConfig
	Block       *ethtypes.Block
	Calls       []SimCallResult
	// Senders is a map of transaction hashes to their Senders.
	Senders map[common.Hash]common.Address
}

func (r *SimBlockResult) MarshalJSON() ([]byte, error) {
	blockData, err := RPCMarshalBlock(r.Block, true, r.FullTx, r.ChainConfig)
	if err != nil {
		return nil, err
	}
	blockData["calls"] = r.Calls
	// Set tx sender and block timestamp if user requested full tx objects.
	if r.FullTx {
		blockTime := hexutil.Uint64(r.Block.Time())
		if raw, ok := blockData["transactions"].([]any); ok {
			for _, tx := range raw {
				if tx, ok := tx.(*RPCTransaction); ok {
					tx.From = r.Senders[tx.Hash]
					// All transactions in the simulated block share the same timestamp.
					tx.BlockTimestamp = &blockTime
				} else {
					return nil, errors.New("simulated transaction result has invalid type")
				}
			}
		}
	}
	return json.Marshal(blockData)
}

// RPCMarshalHeader converts the given header to the RPC output .
func RPCMarshalHeader(head *ethtypes.Header) map[string]interface{} {
	result := map[string]interface{}{
		"number":           (*hexutil.Big)(head.Number),
		"hash":             head.Hash(),
		"parentHash":       head.ParentHash,
		"nonce":            head.Nonce,
		"mixHash":          head.MixDigest,
		"sha3Uncles":       head.UncleHash,
		"logsBloom":        head.Bloom,
		"stateRoot":        head.Root,
		"miner":            head.Coinbase,
		"difficulty":       (*hexutil.Big)(head.Difficulty),
		"extraData":        hexutil.Bytes(head.Extra),
		"gasLimit":         hexutil.Uint64(head.GasLimit),
		"gasUsed":          hexutil.Uint64(head.GasUsed),
		"timestamp":        hexutil.Uint64(head.Time),
		"transactionsRoot": head.TxHash,
		"receiptsRoot":     head.ReceiptHash,
	}
	if head.BaseFee != nil {
		result["baseFeePerGas"] = (*hexutil.Big)(head.BaseFee)
	}
	if head.WithdrawalsHash != nil {
		result["withdrawalsRoot"] = head.WithdrawalsHash
	}
	if head.BlobGasUsed != nil {
		result["blobGasUsed"] = hexutil.Uint64(*head.BlobGasUsed)
	}
	if head.ExcessBlobGas != nil {
		result["excessBlobGas"] = hexutil.Uint64(*head.ExcessBlobGas)
	}
	if head.ParentBeaconRoot != nil {
		result["parentBeaconBlockRoot"] = head.ParentBeaconRoot
	}
	if head.RequestsHash != nil {
		result["requestsHash"] = head.RequestsHash
	}

	return result
}

// RPCMarshalBlock converts the given block to the RPC output which depends on fullTx. If inclTx is true transactions are
// returned. When fullTx is true the returned block contains full transaction details, otherwise it will only contain
// transaction hashes.
func RPCMarshalBlock(block *ethtypes.Block, inclTx bool, fullTx bool, config *params.ChainConfig) (map[string]interface{}, error) {
	fields := RPCMarshalHeader(block.Header())
	fields["size"] = hexutil.Uint64(block.Size())

	if inclTx {
		formatTx := func(idx int, tx *ethtypes.Transaction) (interface{}, error) {
			return tx.Hash(), nil
		}
		if fullTx {
			formatTx = func(idx int, tx *ethtypes.Transaction) (interface{}, error) {
				signer := ethtypes.MakeSigner(config, new(big.Int).SetUint64(block.NumberU64()), block.Time())
				sender, _ := ethtypes.Sender(signer, tx)
				index, err := ethermint.SafeIntToUint64(idx)
				if err != nil {
					return nil, err
				}
				return NewRPCTransactionFromTx(tx, sender, block.Hash(), block.NumberU64(), index, block.BaseFee(), config.ChainID)
			}
		}
		txs := block.Transactions()
		transactions := make([]interface{}, len(txs))
		for i, tx := range txs {
			tx, err := formatTx(i, tx)
			if err != nil {
				return nil, err
			}
			transactions[i] = tx
		}
		fields["transactions"] = transactions
	}
	uncles := block.Uncles()
	uncleHashes := make([]common.Hash, len(uncles))
	for i, uncle := range uncles {
		uncleHashes[i] = uncle.Hash()
	}
	fields["uncles"] = uncleHashes
	if block.Withdrawals() != nil {
		fields["withdrawals"] = block.Withdrawals()
	}
	return fields, nil
}

// GasBudget tracks the remaining gas allowed across all simulated blocks.
type GasBudget struct {
	remaining uint64
}

// NewGasBudget creates a gas budget with the given cap. A cap of 0 is unlimited.
func NewGasBudget(gasCap uint64) *GasBudget {
	if gasCap == 0 {
		gasCap = math.MaxUint64
	}
	return &GasBudget{remaining: gasCap}
}

// Cap returns the given gas value clamped to the remaining budget.
func (b *GasBudget) Cap(gas uint64) uint64 {
	if gas > b.remaining {
		return b.remaining
	}
	return gas
}

// Consume deducts the given amount from the budget.
func (b *GasBudget) Consume(amount uint64) error {
	if amount > b.remaining {
		return fmt.Errorf("RPC gas cap exhausted: need %d, remaining %d", amount, b.remaining)
	}
	b.remaining -= amount
	return nil
}

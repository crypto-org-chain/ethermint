package keeper

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"math/big"
	"sort"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/consensus/misc/eip1559"
	"github.com/ethereum/go-ethereum/consensus/misc/eip4844"
	"github.com/ethereum/go-ethereum/core"
	"github.com/ethereum/go-ethereum/core/tracing"
	ethtypes "github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/core/vm"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/params"
	"github.com/ethereum/go-ethereum/trie"
	"github.com/holiman/uint256"

	rpctypes "github.com/evmos/ethermint/rpc/types"
	"github.com/evmos/ethermint/x/evm/statedb"
	evmtypes "github.com/evmos/ethermint/x/evm/types"
)

// Simulator is a stateful object that simulates a series of blocks.
type Simulator struct {
	keeper         *Keeper
	state          *statedb.StateDB
	base           *ethtypes.Header
	chainConfig    *params.ChainConfig
	budget         *rpctypes.GasBudget
	traceTransfers bool
	validate       bool
	fullTx         bool
}

// Execute runs the simulation of a series of blocks.
func (sim *Simulator) Execute(ctx sdk.Context, blocks []rpctypes.SimBlock) ([]*rpctypes.SimBlockResult, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	var err error
	blocks, err = sim.sanitizeChain(blocks)
	if err != nil {
		return nil, err
	}
	headers, err := sim.makeHeaders(blocks)
	if err != nil {
		return nil, err
	}
	var (
		results = make([]*rpctypes.SimBlockResult, len(blocks))
		parent  = sim.base
	)
	for bi, block := range blocks {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		result, callResults, senders, err := sim.processBlock(ctx, &block, headers[bi], parent, headers[:bi])
		if err != nil {
			return nil, err
		}
		headers[bi] = result.Header()
		results[bi] = &rpctypes.SimBlockResult{
			FullTx:      sim.fullTx,
			ChainConfig: sim.chainConfig,
			Block:       result,
			Calls:       callResults,
			Senders:     senders,
		}
		parent = result.Header()
	}
	return results, nil
}

func (sim *Simulator) processBlock(
	ctx sdk.Context,
	block *rpctypes.SimBlock,
	header, parent *ethtypes.Header,
	headers []*ethtypes.Header,
) (*ethtypes.Block, []rpctypes.SimCallResult, map[common.Hash]common.Address, error) {
	header.ParentHash = parent.Hash()

	// Set base fee
	if sim.chainConfig.IsLondon(header.Number) {
		if header.BaseFee == nil {
			if sim.validate {
				header.BaseFee = eip1559.CalcBaseFee(sim.chainConfig, parent)
			} else {
				header.BaseFee = big.NewInt(0)
			}
		}
	}
	if sim.chainConfig.IsCancun(header.Number, header.Time) {
		var excess uint64
		if sim.chainConfig.IsCancun(parent.Number, parent.Time) {
			excess = eip4844.CalcExcessBlobGas(sim.chainConfig, parent, header.Time)
		}
		header.ExcessBlobGas = &excess
	}

	blockCtx := vm.BlockContext{
		CanTransfer: core.CanTransfer,
		Transfer:    statedb.Transfer,
		GetHash:     sim.getHashFn(ctx, headers),
		Coinbase:    header.Coinbase,
		GasLimit:    header.GasLimit,
		BlockNumber: new(big.Int).Set(header.Number),
		Time:        header.Time,
		Difficulty:  header.Difficulty,
		BaseFee:     header.BaseFee,
	}
	// Post-merge: always set Random so EVM recognizes merge status.
	// vm.NewEVM uses `blockCtx.Random != nil` to determine isMerge.
	if header.Difficulty != nil && header.Difficulty.Sign() == 0 {
		random := header.MixDigest
		blockCtx.Random = &random
	}
	// Apply BlobBaseFee override, or derive the default from ExcessBlobGas for Cancun blocks.
	if block.BlockOverrides != nil && block.BlockOverrides.BlobBaseFee != nil {
		blockCtx.BlobBaseFee = block.BlockOverrides.BlobBaseFee.ToInt()
	} else if sim.chainConfig.IsCancun(header.Number, header.Time) {
		blockCtx.BlobBaseFee = eip4844.CalcBlobFee(sim.chainConfig, header)
	}

	// Get precompiles. Use a cache context when calling custom contract fns so that
	// any accidental store writes inside them do not persist (simulation must not
	// mutate global keeper state).
	isMerge := header.Difficulty == nil || header.Difficulty.Sign() == 0
	rules := sim.chainConfig.Rules(header.Number, isMerge, header.Time)
	defaultPrecompiles := vm.DefaultPrecompiles(rules)
	precompiles := make(vm.PrecompiledContracts, len(defaultPrecompiles))
	active := make([]common.Address, 0, len(defaultPrecompiles))
	for addr, c := range defaultPrecompiles {
		precompiles[addr] = c
		active = append(active, addr)
	}
	cacheCtx, _ := ctx.CacheContext()
	for _, fn := range sim.keeper.customContractFns {
		c := fn(cacheCtx, rules)
		addr := c.Address()
		precompiles[addr] = c
		active = append(active, addr)
	}
	sort.SliceStable(active, func(i, j int) bool {
		return bytes.Compare(active[i].Bytes(), active[j].Bytes()) < 0
	})

	if err := block.StateOverrides.Apply(sim.state, precompiles); err != nil {
		return nil, nil, nil, err
	}
	remainingBlockGas := blockCtx.GasLimit

	// Create tracer and VM config
	tracer := rpctypes.NewSimTracer(sim.traceTransfers, blockCtx.BlockNumber.Uint64(), common.Hash{}, common.Hash{}, 0)
	vmConfig := vm.Config{
		NoBaseFee: !sim.validate,
		Tracer:    tracer.Hooks(),
	}

	// Create tracing statedb
	tracingStateDB := vm.StateDB(sim.state)
	if hooks := tracer.Hooks(); hooks != nil {
		tracingStateDB = statedb.NewHookedState(sim.state, hooks)
	}

	evm := vm.NewEVM(blockCtx, tracingStateDB, sim.chainConfig, vmConfig)
	evm.SetPrecompiles(precompiles)

	// Process calls
	callResults := make([]rpctypes.SimCallResult, len(block.Calls))
	transactions := make([]*ethtypes.Transaction, len(block.Calls))
	// senders is a map of transaction hashes to their senders.
	// Transaction objects contain only the signature, and we lose track
	// of the sender when translating the arguments into a transaction object.
	senders := make(map[common.Hash]common.Address)
	receipts := make(ethtypes.Receipts, len(block.Calls))
	cumulativeGasUsed := uint64(0)
	var allLogs []*ethtypes.Log

	for i, callJSON := range block.Calls {
		var call evmtypes.TransactionArgs
		if err := json.Unmarshal(callJSON, &call); err != nil {
			return nil, nil, nil, &rpctypes.InvalidParamsError{Message: fmt.Sprintf("invalid call args at index %d: %v", i, err)}
		}

		if err := sim.sanitizeCall(&call, sim.state, header, &remainingBlockGas); err != nil {
			return nil, nil, nil, err
		}

		tx := call.ToEthTransaction()
		txHash := tx.Hash()
		transactions[i] = tx
		senders[txHash] = call.GetFrom()

		tracer.Reset(txHash, uint(i)) //nolint:gosec // G115: i is a range index over block.Calls, always non-negative.

		msg, err := call.ToSimMessage(header.BaseFee, !sim.validate)
		if err != nil {
			return nil, nil, nil, &rpctypes.InvalidParamsError{Message: err.Error()}
		}

		evm.SetTxContext(core.NewEVMTxContext(msg))
		result, err := sim.applyCall(evm, msg, rules, active)
		if err != nil {
			txErr := rpctypes.TxValidationError(err)
			return nil, nil, nil, txErr
		}
		gasUsed := result.GasUsed
		remainingBlockGas -= gasUsed
		//nolint:misspell
		tracingStateDB.Finalise(true)

		// Consume gas budget
		if err := sim.budget.Consume(gasUsed); err != nil {
			return nil, nil, nil, err
		}

		logs := tracer.Logs()
		receiptLogs := filterReceiptLogs(logs)
		simLogs := make([]*rpctypes.SimLog, len(logs))
		for li, l := range logs {
			simLogs[li] = rpctypes.NewSimLog(l, header.Time)
		}
		callRes := rpctypes.SimCallResult{
			ReturnValue: hexutil.Bytes(result.Ret),
			Logs:        simLogs,
			GasUsed:     hexutil.Uint64(result.GasUsed),
			MaxUsedGas:  hexutil.Uint64(result.MaxUsedGas),
		}
		if result.VMErr != nil {
			callRes.Status = hexutil.Uint64(ethtypes.ReceiptStatusFailed)
			callRes.ReturnValue = nil
			if errors.Is(result.VMErr, vm.ErrExecutionReverted) {
				revertErr := rpctypes.NewRevertError(result.Ret)
				callRes.Error = revertErr
			} else {
				callRes.Error = &rpctypes.CallError{Message: result.VMErr.Error(), Code: rpctypes.ErrCodeVMError}
			}
		} else {
			callRes.Status = hexutil.Uint64(ethtypes.ReceiptStatusSuccessful)
			allLogs = append(allLogs, receiptLogs...)
		}
		callResults[i] = callRes

		cumulativeGasUsed += gasUsed
		receipt := &ethtypes.Receipt{
			Type:              tx.Type(),
			CumulativeGasUsed: cumulativeGasUsed,
			TxHash:            txHash,
			GasUsed:           gasUsed,
			Logs:              receiptLogs,
			Status:            uint64(callRes.Status),
		}
		if tx.To() == nil && uint64(callRes.Status) == ethtypes.ReceiptStatusSuccessful {
			receipt.ContractAddress = crypto.CreateAddress(call.GetFrom(), tx.Nonce())
		}
		receipt.Bloom = ethtypes.CreateBloom(receipt)
		receipts[i] = receipt
	}

	// Update header gas used
	header.GasUsed = cumulativeGasUsed
	if sim.chainConfig.IsCancun(header.Number, header.Time) {
		blobGasUsed := uint64(0)
		header.BlobGasUsed = &blobGasUsed
	}

	// EIP-7685 requests (Prague)
	var requests [][]byte
	if sim.chainConfig.IsPrague(header.Number, header.Time) {
		requests = [][]byte{}
		if err := core.ParseDepositLogs(&requests, allLogs, sim.chainConfig); err != nil {
			return nil, nil, nil, err
		}
		if err := core.ProcessWithdrawalQueue(&requests, evm); err != nil {
			return nil, nil, nil, err
		}
		if err := core.ProcessConsolidationQueue(&requests, evm); err != nil {
			return nil, nil, nil, err
		}
	}
	if requests != nil {
		reqHash := ethtypes.CalcRequestsHash(requests)
		header.RequestsHash = &reqHash
	}

	var withdrawals ethtypes.Withdrawals
	if block.BlockOverrides != nil && block.BlockOverrides.Withdrawals != nil {
		withdrawals = *block.BlockOverrides.Withdrawals
	}
	ethBlock := ethtypes.NewBlock(
		header,
		&ethtypes.Body{Transactions: transactions, Uncles: nil, Withdrawals: withdrawals},
		receipts,
		trie.NewStackTrie(nil),
	)
	*header = *ethBlock.Header()

	// Build block hash for log repair
	blockHash := ethBlock.Hash()
	repairLogs(callResults, blockHash)

	return ethBlock, callResults, senders, nil
}

func filterReceiptLogs(logs []*ethtypes.Log) []*ethtypes.Log {
	out := make([]*ethtypes.Log, 0, len(logs))
	for _, l := range logs {
		if l.Address != rpctypes.TransferTraceAddress {
			out = append(out, l)
		}
	}
	return out
}

// repairLogs updates the block hash in logs.
func repairLogs(calls []rpctypes.SimCallResult, hash common.Hash) {
	for i := range calls {
		for j := range calls[i].Logs {
			calls[i].Logs[j].BlockHash = hash
		}
	}
}

// applyCallResult holds the output of a single simulated call.
type applyCallResult struct {
	GasUsed    uint64 // net gas after refunds
	MaxUsedGas uint64 // peak gas before refunds (go-ethereum ExecutionResult.MaxUsedGas)
	Ret        []byte
	VMErr      error
}

// applyCall executes a single simulated call using the same logic as
// ApplyMessageWithConfig (intrinsic gas → Prepare → evm.Call/Create → refund)
// but operates on the shared statedb/evm instead of creating new ones.
// Returns (result, fatalError); a non-nil fatalError aborts the whole simulation.
func (sim *Simulator) applyCall(
	evm *vm.EVM,
	msg *core.Message,
	rules params.Rules,
	activePrecompiles []common.Address,
) (applyCallResult, error) {
	leftoverGas := msg.GasLimit
	contractCreation := msg.To == nil
	sender := msg.From

	value, overflow := uint256.FromBig(msg.Value)
	if overflow {
		return applyCallResult{}, fmt.Errorf("%w: address %v", core.ErrInsufficientFunds, msg.From.Hex())
	}
	if sim.validate {
		if msg.Nonce == math.MaxUint64 {
			return applyCallResult{}, fmt.Errorf("%w: address %v, nonce: %d",
				core.ErrNonceMax, sender.Hex(), msg.Nonce)
		}
		stateNonce := sim.state.GetNonce(sender)
		if msg.Nonce < stateNonce {
			return applyCallResult{}, fmt.Errorf("%w: address %v, tx: %d state: %d",
				core.ErrNonceTooLow, sender.Hex(), msg.Nonce, stateNonce)
		}
		if msg.Nonce > stateNonce {
			return applyCallResult{}, fmt.Errorf("%w: address %v, tx: %d state: %d",
				core.ErrNonceTooHigh, sender.Hex(), msg.Nonce, stateNonce)
		}
		// London fee checks
		if sim.chainConfig.IsLondon(evm.Context.BlockNumber) {
			if msg.GasFeeCap.Cmp(evm.Context.BaseFee) < 0 {
				return applyCallResult{}, fmt.Errorf("%w: address %v, maxFeePerGas: %s, baseFee: %s",
					core.ErrFeeCapTooLow, sender.Hex(), msg.GasFeeCap, evm.Context.BaseFee)
			}
		}
	}
	// Balance check: gasLimit * gasFeeCap + value.
	// This mirrors geth's buyGas() which always runs regardless of validation
	// mode. Without this, insufficient-funds scenarios silently proceed as
	// VM-level failures instead of aborting the block.
	{
		balanceCheck := new(big.Int).SetUint64(msg.GasLimit)
		if msg.GasFeeCap != nil {
			balanceCheck.Mul(balanceCheck, msg.GasFeeCap)
		}
		if msg.Value != nil {
			balanceCheck.Add(balanceCheck, msg.Value)
		}
		balance := sim.state.GetBalance(sender)
		balanceU256, _ := uint256.FromBig(balanceCheck)
		if balance.Cmp(balanceU256) < 0 {
			return applyCallResult{}, fmt.Errorf("%w: address %v have %v want %v (supplied gas %d)",
				core.ErrInsufficientFunds, sender.Hex(), balance, balanceCheck, msg.GasLimit)
		}
	}

	// Intrinsic gas check (same as ApplyMessageWithConfig)
	intrinsicGas, err := sim.keeper.GetEthIntrinsicGas(msg, rules, contractCreation)
	if err != nil {
		return applyCallResult{}, err
	}
	if leftoverGas < intrinsicGas {
		return applyCallResult{}, core.ErrIntrinsicGas
	}
	leftoverGas -= intrinsicGas

	// Enforce EIP-7623 floor data gas for Prague (mirrors ApplyMessageWithConfig).
	if rules.IsPrague {
		floorDataGas, err := core.FloorDataGas(msg.Data)
		if err != nil {
			return applyCallResult{}, err
		}
		if msg.GasLimit < floorDataGas {
			return applyCallResult{}, core.ErrFloorDataGas
		}
	}

	// Shanghai init code size check
	if rules.IsShanghai && contractCreation && len(msg.Data) > params.MaxInitCodeSize {
		return applyCallResult{}, fmt.Errorf("%w: code size %v limit %v", core.ErrMaxInitCodeSizeExceeded, len(msg.Data), params.MaxInitCodeSize)
	}

	// Prepare access list and transient storage
	sim.state.Prepare(rules, msg.From, evm.Context.Coinbase, msg.To, activePrecompiles, msg.AccessList)

	// Set the nonce for the sender before execution so that CREATE addresses
	// are computed correctly. evm.Create will internally bump it to nonce+1.
	sim.state.SetNonce(msg.From, msg.Nonce, tracing.NonceChangeUnspecified)

	var (
		ret   []byte
		vmErr error
	)
	if contractCreation {
		ret, _, leftoverGas, vmErr = evm.Create(msg.From, msg.Data, leftoverGas, value)
	} else {
		if msg.SetCodeAuthorizations != nil {
			for _, auth := range msg.SetCodeAuthorizations {
				if _, err := sim.keeper.applyAuthorization(&auth, sim.state); err != nil {
					sim.keeper.Logger(sim.state.Context()).Debug("simulation: failed to apply authorization",
						"error", err, "authorization", auth)
				}
			}
		}
		ret, leftoverGas, vmErr = evm.Call(msg.From, *msg.To, msg.Data, leftoverGas, value)
		sim.state.SetNonce(msg.From, msg.Nonce+1, tracing.NonceChangeUnspecified)
	}

	// Gas refund (same logic as ApplyMessageWithConfig)
	if msg.GasLimit < leftoverGas {
		return applyCallResult{}, fmt.Errorf("gas overflow: limit %d < leftover %d", msg.GasLimit, leftoverGas)
	}
	temporaryGasUsed := msg.GasLimit - leftoverGas
	refundQuotient := params.RefundQuotient
	if rules.IsLondon {
		refundQuotient = params.RefundQuotientEIP3529
	}
	refund := GasToRefund(sim.state.GetRefund(), temporaryGasUsed, refundQuotient)
	leftoverGas += refund

	// Apply EIP-7623 post-execution floor on post-refund gas used.
	if rules.IsPrague {
		floorDataGas, err := core.FloorDataGas(msg.Data)
		if err != nil {
			return applyCallResult{}, err
		}
		if msg.GasLimit-leftoverGas < floorDataGas {
			leftoverGas = msg.GasLimit - floorDataGas
			if temporaryGasUsed < floorDataGas {
				temporaryGasUsed = floorDataGas
			}
		}
	}

	gasUsed := msg.GasLimit - leftoverGas

	// charge gas
	if msg.GasPrice.Sign() > 0 {
		gasPriceU256, _ := uint256.FromBig(msg.GasPrice)
		cost := new(uint256.Int).SetUint64(gasUsed)
		cost.Mul(cost, gasPriceU256)
		sim.state.SubBalance(sender, cost, tracing.BalanceDecreaseGasBuy)
	}
	if !evm.Config.NoBaseFee || msg.GasFeeCap.Sign() != 0 || msg.GasTipCap.Sign() != 0 {
		effectiveTip := new(big.Int).Set(msg.GasTipCap)
		if evm.Context.BaseFee != nil {
			if maxTip := new(big.Int).Sub(msg.GasFeeCap, evm.Context.BaseFee); effectiveTip.Cmp(maxTip) > 0 {
				effectiveTip = maxTip
			}
		}
		if effectiveTip.Sign() > 0 {
			tipU256, _ := uint256.FromBig(new(big.Int).Mul(new(big.Int).SetUint64(gasUsed), effectiveTip))
			sim.state.AddBalance(evm.Context.Coinbase, tipU256, tracing.BalanceIncreaseRewardTransactionFee)
		}
	}

	return applyCallResult{
		GasUsed:    gasUsed,
		MaxUsedGas: temporaryGasUsed,
		Ret:        ret,
		VMErr:      vmErr,
	}, nil
}

func (sim *Simulator) sanitizeCall(call *evmtypes.TransactionArgs, state *statedb.StateDB, header *ethtypes.Header, remainingBlockGas *uint64) error {
	if call.Nonce == nil {
		nonce := state.GetNonce(call.GetFrom())
		call.Nonce = (*hexutil.Uint64)(&nonce)
	}
	// Let the call run wild unless explicitly specified.
	if call.Gas == nil {
		call.Gas = (*hexutil.Uint64)(remainingBlockGas)
	}
	if *remainingBlockGas < uint64(*call.Gas) {
		return &rpctypes.BlockGasLimitReachedError{
			Message: fmt.Sprintf(
				"block gas limit reached: remaining: %d, required: %d",
				*remainingBlockGas,
				*call.Gas,
			),
		}
	}
	// Clamp to the cross-block gas budget.
	gas := sim.budget.Cap(uint64(*call.Gas))
	call.Gas = (*hexutil.Uint64)(&gas)

	return call.CallDefaults(0, header.BaseFee, sim.chainConfig.ChainID)
}

// sanitizeChain checks the chain integrity. Specifically it checks that
// block numbers and timestamp are strictly increasing, setting default values
// when necessary. Gaps in block numbers are filled with empty blocks.
// Note: It modifies the block's override object.
func (sim *Simulator) sanitizeChain(blocks []rpctypes.SimBlock) ([]rpctypes.SimBlock, error) {
	var (
		res           = make([]rpctypes.SimBlock, 0, len(blocks))
		base          = sim.base
		prevNumber    = base.Number
		prevTimestamp = base.Time
	)
	for _, block := range blocks {
		if block.BlockOverrides == nil {
			block.BlockOverrides = new(rpctypes.SimBlockOverrides)
		}
		if block.BlockOverrides.Number == nil {
			n := new(big.Int).Add(prevNumber, big.NewInt(1))
			block.BlockOverrides.Number = (*hexutil.Big)(n)
		}
		if block.BlockOverrides.Withdrawals == nil {
			block.BlockOverrides.Withdrawals = &ethtypes.Withdrawals{}
		}
		diff := new(big.Int).Sub(block.BlockOverrides.Number.ToInt(), prevNumber)
		if diff.Cmp(common.Big0) <= 0 {
			return nil, &rpctypes.InvalidBlockNumberError{
				Message: fmt.Sprintf(
					"block numbers must be in order: %d <= %d",
					block.BlockOverrides.Number.ToInt().Uint64(),
					prevNumber,
				),
			}
		}
		if total := new(big.Int).Sub(block.BlockOverrides.Number.ToInt(), base.Number); total.Cmp(big.NewInt(rpctypes.MaxSimulateBlocks)) > 0 {
			return nil, &rpctypes.ClientLimitExceededError{Message: "too many blocks"}
		}
		if diff.Cmp(big.NewInt(1)) > 0 {
			// Fill the gap with empty blocks.
			gap := new(big.Int).Sub(diff, big.NewInt(1))
			for i := uint64(0); i < gap.Uint64(); i++ {
				n := new(big.Int).Add(prevNumber, new(big.Int).SetUint64(i+1))
				t := prevTimestamp + rpctypes.TimestampIncrement
				b := rpctypes.SimBlock{
					BlockOverrides: &rpctypes.SimBlockOverrides{
						Number:      (*hexutil.Big)(n),
						Time:        (*hexutil.Uint64)(&t),
						Withdrawals: &ethtypes.Withdrawals{},
					},
				}
				prevTimestamp = t
				res = append(res, b)
			}
		}
		// Only append block after filling a potential gap.
		prevNumber = block.BlockOverrides.Number.ToInt()
		var t uint64
		if block.BlockOverrides.Time == nil {
			t = prevTimestamp + rpctypes.TimestampIncrement
			block.BlockOverrides.Time = (*hexutil.Uint64)(&t)
		} else {
			t = uint64(*block.BlockOverrides.Time)
			if t <= prevTimestamp {
				return nil, &rpctypes.InvalidBlockTimestampError{Message: fmt.Sprintf("block timestamps must be in order: %d <= %d", t, prevTimestamp)}
			}
		}
		prevTimestamp = t
		res = append(res, block)
	}
	return res, nil
}

// makeHeaders creates block headers from overrides.
func (sim *Simulator) makeHeaders(blocks []rpctypes.SimBlock) ([]*ethtypes.Header, error) {
	var (
		res    = make([]*ethtypes.Header, len(blocks))
		header = sim.base
	)
	for bi, block := range blocks {
		if block.BlockOverrides == nil || block.BlockOverrides.Number == nil {
			return nil, errors.New("empty block number")
		}
		overrides := block.BlockOverrides
		number := overrides.Number.ToInt()
		timestamp := uint64(*overrides.Time)

		var withdrawalsHash *common.Hash
		if sim.chainConfig.IsShanghai(number, timestamp) {
			withdrawalsHash = &ethtypes.EmptyWithdrawalsHash
		}

		var parentBeaconRoot *common.Hash
		if sim.chainConfig.IsCancun(number, timestamp) {
			root := common.Hash{}
			parentBeaconRoot = &root
			if overrides.BeaconRoot != nil {
				parentBeaconRoot = overrides.BeaconRoot
			}
		}

		// Post-merge: difficulty = 0
		difficulty := header.Difficulty
		if sim.chainConfig.MergeNetsplitBlock != nil {
			difficulty = big.NewInt(0)
		}

		header = overrides.MakeHeader(&ethtypes.Header{
			UncleHash:        ethtypes.EmptyUncleHash,
			ReceiptHash:      ethtypes.EmptyReceiptsHash,
			TxHash:           ethtypes.EmptyTxsHash,
			Coinbase:         header.Coinbase,
			Difficulty:       difficulty,
			GasLimit:         header.GasLimit,
			WithdrawalsHash:  withdrawalsHash,
			ParentBeaconRoot: parentBeaconRoot,
		})
		res[bi] = header
	}
	return res, nil
}

// getHashFn returns a GetHashFunc that looks up simulated headers first,
// then falls back to the keeper's historical block hashes.
// The base block hash is handled specially: keeperGetHash's range check excludes
// num64 >= upper (upper = ctx.BlockHeight() = base block), so we call GetHeaderHash
// directly, which reads from the store (populated by BeginBlock's SetHeaderHash).
func (sim *Simulator) getHashFn(ctx sdk.Context, prevHeaders []*ethtypes.Header) vm.GetHashFunc {
	keeperGetHash := sim.keeper.GetHashFn(ctx, sim.keeper.GetParams(ctx).HeaderHashNum)
	baseNum := sim.base.Number.Uint64()
	return func(num64 uint64) common.Hash {
		// Check simulated headers
		for _, h := range prevHeaders {
			if h.Number.Uint64() == num64 {
				return h.Hash()
			}
		}
		// For the base block, keeperGetHash's range check (num64 >= upper) would
		// return zero, so read from the store directly.
		if num64 == baseNum {
			if hash := sim.keeper.GetHeaderHash(ctx, num64); hash != (common.Hash{}) {
				return hash
			}
		}
		// Fall back to keeper for all other historical blocks
		return keeperGetHash(num64)
	}
}

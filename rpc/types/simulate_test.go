package types

import (
	"crypto/ecdsa"
	"encoding/hex"
	"encoding/json"
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/core"
	ethtypes "github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/core/vm"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/params"
	"github.com/ethereum/go-ethereum/trie"
	"github.com/holiman/uint256"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// GasBudget
// ---------------------------------------------------------------------------

func TestNewGasBudget_Unlimited(t *testing.T) {
	b := NewGasBudget(0)
	// A zero cap means unlimited – Cap should return any requested amount as-is.
	require.Equal(t, uint64(1_000_000), b.Cap(1_000_000))
}

func TestNewGasBudget_Cap(t *testing.T) {
	b := NewGasBudget(100)
	// Requesting more than the budget should clamp to the remaining amount.
	require.Equal(t, uint64(100), b.Cap(999))
	// Requesting exactly the budget is fine.
	require.Equal(t, uint64(100), b.Cap(100))
	// Requesting less than the budget passes through.
	require.Equal(t, uint64(50), b.Cap(50))
}

func TestGasBudget_Consume(t *testing.T) {
	b := NewGasBudget(100)

	require.NoError(t, b.Consume(30))
	require.Equal(t, uint64(70), b.Cap(999))

	require.NoError(t, b.Consume(70))
	require.Equal(t, uint64(0), b.Cap(999))

	// One more consume should exceed the budget.
	err := b.Consume(1)
	require.Error(t, err)
	require.Contains(t, err.Error(), "RPC gas cap exhausted")
}

func TestGasBudget_ConsumeExact(t *testing.T) {
	b := NewGasBudget(50)
	require.NoError(t, b.Consume(50))
	require.Error(t, b.Consume(1))
}

// ---------------------------------------------------------------------------
// SimBlockOverrides.MakeHeader
// ---------------------------------------------------------------------------

func TestSimBlockOverrides_MakeHeader_NilOverrides(t *testing.T) {
	base := &ethtypes.Header{Number: big.NewInt(5), Time: 100}
	var o *SimBlockOverrides
	result := o.MakeHeader(base)
	require.Equal(t, base.Number, result.Number)
	require.Equal(t, base.Time, result.Time)
}

func TestSimBlockOverrides_MakeHeader_AllFields(t *testing.T) {
	base := &ethtypes.Header{
		Number: big.NewInt(5),
		Time:   100,
	}
	newNumber := (*hexutil.Big)(big.NewInt(42))
	newTime := hexutil.Uint64(200)
	newGasLimit := hexutil.Uint64(8_000_000)
	feeRecipient := common.HexToAddress("0xdeadbeefdeadbeefdeadbeefdeadbeefdeadbeef")
	prevRandao := common.HexToHash("0x1234")
	baseFee := (*hexutil.Big)(big.NewInt(1e9))

	o := &SimBlockOverrides{
		Number:        newNumber,
		Time:          &newTime,
		GasLimit:      &newGasLimit,
		FeeRecipient:  &feeRecipient,
		PrevRandao:    &prevRandao,
		BaseFeePerGas: baseFee,
	}

	result := o.MakeHeader(base)

	require.Equal(t, big.NewInt(42), result.Number)
	require.Equal(t, uint64(200), result.Time)
	require.Equal(t, uint64(8_000_000), result.GasLimit)
	require.Equal(t, feeRecipient, result.Coinbase)
	require.Equal(t, prevRandao, result.MixDigest)
	require.Equal(t, big.NewInt(1e9), result.BaseFee)

	// The original header must not be mutated.
	require.Equal(t, big.NewInt(5), base.Number)
}

func TestSimBlockOverrides_MakeHeader_PartialOverride(t *testing.T) {
	base := &ethtypes.Header{Number: big.NewInt(10), Time: 50}
	newTime := hexutil.Uint64(99)
	o := &SimBlockOverrides{Time: &newTime}
	result := o.MakeHeader(base)
	// Number is unchanged, only time is overridden.
	require.Equal(t, big.NewInt(10), result.Number)
	require.Equal(t, uint64(99), result.Time)
}

// ---------------------------------------------------------------------------
// SimStateOverride.Apply
// ---------------------------------------------------------------------------

func TestSimStateOverride_Nil(t *testing.T) {
	var diff *SimStateOverride
	err := diff.Apply(nil, vm.PrecompiledContracts{})
	require.NoError(t, err)
}

func TestSimStateOverride_StatePlusStateDiff_Error(t *testing.T) {
	addr := common.HexToAddress("0xaabbcc")
	stateKey := common.HexToHash("0x01")
	stateVal := common.HexToHash("0xff")

	diff := SimStateOverride{
		addr: SimOverrideAccount{
			State:     map[common.Hash]common.Hash{stateKey: stateVal},
			StateDiff: map[common.Hash]common.Hash{stateKey: stateVal},
		},
	}
	err := diff.Apply(nil, vm.PrecompiledContracts{})
	require.Error(t, err)
	require.Contains(t, err.Error(), "both 'state' and 'stateDiff'")
}

func TestSimStateOverride_MoveNonPrecompileError(t *testing.T) {
	addr := common.HexToAddress("0xaabbcc")
	dest := common.HexToAddress("0xddeeff")
	diff := SimStateOverride{
		addr: SimOverrideAccount{
			MovePrecompileTo: &dest,
		},
	}
	err := diff.Apply(nil, vm.PrecompiledContracts{})
	require.Error(t, err)
	require.Contains(t, err.Error(), "not a precompile")
}

// TestSimStateOverride_BalanceOverflow verifies that an account balance
// exceeding uint256 max triggers an InvalidParamsError from Apply.
func TestSimStateOverride_BalanceOverflow(t *testing.T) {
	addr := common.HexToAddress("0xaabbccdd")
	// 2^256 overflows uint256
	overflow := new(big.Int).Lsh(big.NewInt(1), 256)
	bal := (*hexutil.Big)(overflow)
	diff := SimStateOverride{
		addr: SimOverrideAccount{Balance: bal},
	}
	err := diff.Apply(nil, vm.PrecompiledContracts{})
	require.Error(t, err)
	require.Contains(t, err.Error(), "overflows uint256")
}

// ---------------------------------------------------------------------------
// SimCallResult JSON marshalling
// ---------------------------------------------------------------------------

func TestSimCallResult_MarshalJSON_NilLogs(t *testing.T) {
	r := &SimCallResult{
		ReturnValue: hexutil.Bytes{0x01},
		Logs:        nil,
		GasUsed:     hexutil.Uint64(21000),
		Status:      hexutil.Uint64(1),
	}
	bz, err := json.Marshal(r)
	require.NoError(t, err)

	var decoded map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(bz, &decoded))
	// logs must be an empty array, not null.
	require.Equal(t, `[]`, string(decoded["logs"]))
}

func TestSimCallResult_MarshalJSON_WithLogs(t *testing.T) {
	log := &ethtypes.Log{
		Address: common.HexToAddress("0xdeadbeef"),
		Topics:  []common.Hash{common.HexToHash("0x1234")},
		Data:    []byte{0xaa, 0xbb},
	}
	r := &SimCallResult{
		Logs:   []*ethtypes.Log{log},
		Status: hexutil.Uint64(1),
	}
	bz, err := json.Marshal(r)
	require.NoError(t, err)

	var decoded map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(bz, &decoded))

	var logs []json.RawMessage
	require.NoError(t, json.Unmarshal(decoded["logs"], &logs))
	require.Len(t, logs, 1)
}

func TestSimCallResult_MarshalJSON_WithError(t *testing.T) {
	callErr := &CallError{Message: "execution reverted", Code: 3, Data: "0xdeadbeef"}
	r := &SimCallResult{
		Status: hexutil.Uint64(0),
		Error:  callErr,
	}
	bz, err := json.Marshal(r)
	require.NoError(t, err)

	var decoded map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(bz, &decoded))
	require.Contains(t, string(decoded["error"]), "execution reverted")
}

// ---------------------------------------------------------------------------
// Error types
// ---------------------------------------------------------------------------

func TestInvalidParamsError(t *testing.T) {
	e := &InvalidParamsError{Message: "bad param"}
	require.Equal(t, ErrCodeInvalidParams, e.ErrorCode())
	require.Equal(t, "bad param", e.Error())
}

func TestClientLimitExceededError(t *testing.T) {
	e := &ClientLimitExceededError{Message: "too many blocks"}
	require.Equal(t, ErrCodeClientLimitExceeded, e.ErrorCode())
	require.Equal(t, "too many blocks", e.Error())
}

func TestInvalidBlockNumberError(t *testing.T) {
	e := &InvalidBlockNumberError{Message: "invalid block"}
	require.Equal(t, ErrCodeBlockNumberInvalid, e.ErrorCode())
	require.Equal(t, "invalid block", e.Error())
}

func TestInvalidBlockTimestampError(t *testing.T) {
	e := &InvalidBlockTimestampError{Message: "timestamp out of order"}
	require.Equal(t, ErrCodeBlockTimestampInvalid, e.ErrorCode())
	require.Equal(t, "timestamp out of order", e.Error())
}

func TestBlockGasLimitReachedError(t *testing.T) {
	e := &BlockGasLimitReachedError{Message: "block gas limit reached"}
	require.Equal(t, ErrCodeBlockGasLimitReached, e.ErrorCode())
	require.Equal(t, "block gas limit reached", e.Error())
}

func TestInvalidTxError(t *testing.T) {
	e := &InvalidTxError{Message: "nonce too low", Code: ErrCodeNonceTooLow}
	require.Equal(t, ErrCodeNonceTooLow, e.ErrorCode())
	require.Equal(t, "nonce too low", e.Error())
}

// ---------------------------------------------------------------------------
// TxValidationError
// ---------------------------------------------------------------------------

func TestTxValidationError_Nil(t *testing.T) {
	require.Nil(t, TxValidationError(nil))
}

func TestTxValidationError_Unknown(t *testing.T) {
	unknownErr := &unknownError{msg: "something weird"}
	result := TxValidationError(unknownErr)
	require.NotNil(t, result)
	require.Equal(t, ErrCodeInternalError, result.Code)
}

// unknownError is a local error type for testing TxValidationError fallback.
type unknownError struct{ msg string }

func (e *unknownError) Error() string { return e.msg }

// ---------------------------------------------------------------------------
// NewRevertError
// ---------------------------------------------------------------------------

func TestNewRevertError_UnpackableData(t *testing.T) {
	// Random bytes that can't be ABI-decoded as a revert reason.
	data := []byte{0xde, 0xad, 0xbe, 0xef}
	e := NewRevertError(data)
	require.Equal(t, 3, e.Code)
	require.Contains(t, e.Message, vm.ErrExecutionReverted.Error())
	require.Equal(t, hexutil.Encode(data), e.Data)
}

func TestNewRevertError_EmptyData(t *testing.T) {
	e := NewRevertError([]byte{})
	require.Equal(t, 3, e.Code)
}

// ---------------------------------------------------------------------------
// SimOpts JSON round-trip
// ---------------------------------------------------------------------------

func TestSimOpts_JSONRoundTrip(t *testing.T) {
	opts := SimOpts{
		BlockStateCalls:        []SimBlock{{}, {}},
		TraceTransfers:         true,
		Validation:             true,
		ReturnFullTransactions: false,
	}
	bz, err := json.Marshal(opts)
	require.NoError(t, err)

	var decoded SimOpts
	require.NoError(t, json.Unmarshal(bz, &decoded))
	require.Equal(t, opts.TraceTransfers, decoded.TraceTransfers)
	require.Equal(t, opts.Validation, decoded.Validation)
	require.Equal(t, opts.ReturnFullTransactions, decoded.ReturnFullTransactions)
	require.Len(t, decoded.BlockStateCalls, 2)
}

// ---------------------------------------------------------------------------
// SimBlock JSON round-trip
// ---------------------------------------------------------------------------

func TestSimBlock_JSONRoundTrip_WithOverrides(t *testing.T) {
	num := (*hexutil.Big)(big.NewInt(99))
	ts := hexutil.Uint64(12345)
	block := SimBlock{
		BlockOverrides: &SimBlockOverrides{
			Number: num,
			Time:   &ts,
		},
	}
	bz, err := json.Marshal(block)
	require.NoError(t, err)

	var decoded SimBlock
	require.NoError(t, json.Unmarshal(bz, &decoded))
	require.NotNil(t, decoded.BlockOverrides)
	require.Equal(t, big.NewInt(99), decoded.BlockOverrides.Number.ToInt())
	require.Equal(t, hexutil.Uint64(12345), *decoded.BlockOverrides.Time)
}

// ---------------------------------------------------------------------------
// TxValidationError - all error codes
// ---------------------------------------------------------------------------

func TestTxValidationError_AllCodes(t *testing.T) {
	testCases := []struct {
		err     error
		expCode int
	}{
		{core.ErrNonceTooHigh, ErrCodeNonceTooHigh},
		{core.ErrNonceTooLow, ErrCodeNonceTooLow},
		{core.ErrSenderNoEOA, ErrCodeSenderIsNotEOA},
		{core.ErrFeeCapVeryHigh, ErrCodeInvalidParams},
		{core.ErrTipVeryHigh, ErrCodeInvalidParams},
		{core.ErrTipAboveFeeCap, ErrCodeInvalidParams},
		{core.ErrFeeCapTooLow, ErrCodeInvalidParams},
		{core.ErrInsufficientFunds, ErrCodeInsufficientFunds},
		{core.ErrIntrinsicGas, ErrCodeIntrinsicGas},
		{core.ErrInsufficientFundsForTransfer, ErrCodeInsufficientFunds},
		{core.ErrMaxInitCodeSizeExceeded, ErrCodeMaxInitCodeSizeExceeded},
	}
	for _, tc := range testCases {
		result := TxValidationError(tc.err)
		require.NotNil(t, result)
		require.Equal(t, tc.expCode, result.Code)
	}
}

// ---------------------------------------------------------------------------
// RPCMarshalHeader
// ---------------------------------------------------------------------------

func TestRPCMarshalHeader_Basic(t *testing.T) {
	header := &ethtypes.Header{
		Number:     big.NewInt(10),
		Time:       12345,
		GasLimit:   8_000_000,
		GasUsed:    21_000,
		Difficulty: big.NewInt(0),
	}
	result := RPCMarshalHeader(header)
	require.Equal(t, (*hexutil.Big)(big.NewInt(10)), result["number"])
	require.Equal(t, hexutil.Uint64(8_000_000), result["gasLimit"])
	require.Equal(t, hexutil.Uint64(21_000), result["gasUsed"])
	require.Equal(t, hexutil.Uint64(12345), result["timestamp"])
}

func TestRPCMarshalHeader_WithOptionalFields(t *testing.T) {
	baseFee := big.NewInt(1e9)
	blobGasUsed := uint64(1000)
	excessBlobGas := uint64(2000)
	parentBeaconRoot := common.HexToHash("0xdeadbeef")
	withdrawalsHash := ethtypes.EmptyWithdrawalsHash

	header := &ethtypes.Header{
		Number:           big.NewInt(20),
		Time:             99999,
		Difficulty:       big.NewInt(0),
		BaseFee:          baseFee,
		WithdrawalsHash:  &withdrawalsHash,
		BlobGasUsed:      &blobGasUsed,
		ExcessBlobGas:    &excessBlobGas,
		ParentBeaconRoot: &parentBeaconRoot,
	}
	result := RPCMarshalHeader(header)
	require.Equal(t, (*hexutil.Big)(baseFee), result["baseFeePerGas"])
	require.Equal(t, &withdrawalsHash, result["withdrawalsRoot"])
	require.Equal(t, hexutil.Uint64(1000), result["blobGasUsed"])
	require.Equal(t, hexutil.Uint64(2000), result["excessBlobGas"])
	require.Equal(t, &parentBeaconRoot, result["parentBeaconBlockRoot"])
}

// ---------------------------------------------------------------------------
// RPCMarshalBlock
// ---------------------------------------------------------------------------

func TestRPCMarshalBlock_NoTxs(t *testing.T) {
	header := &ethtypes.Header{
		Number:     big.NewInt(5),
		Difficulty: big.NewInt(0),
		Time:       100,
	}
	block := ethtypes.NewBlock(header, nil, nil, nil)
	result, err := RPCMarshalBlock(block, false, false, params.TestChainConfig)
	require.NoError(t, err)
	require.NotNil(t, result["number"])
	// inclTx=false means no "transactions" key filled with tx hashes
	_, hasTransactions := result["transactions"]
	require.False(t, hasTransactions)
}

func TestRPCMarshalBlock_TxHashes(t *testing.T) {
	header := &ethtypes.Header{
		Number:     big.NewInt(5),
		Difficulty: big.NewInt(0),
		Time:       100,
	}
	tx := ethtypes.NewTx(&ethtypes.LegacyTx{Nonce: 1, Gas: 21000})
	block := ethtypes.NewBlock(
		header,
		&ethtypes.Body{Transactions: ethtypes.Transactions{tx}},
		nil, trie.NewStackTrie(nil),
	)
	result, err := RPCMarshalBlock(block, true, false, params.TestChainConfig)
	require.NoError(t, err)
	txs, ok := result["transactions"].([]interface{})
	require.True(t, ok)
	require.Len(t, txs, 1)
	// Hash-only mode: element is a common.Hash
	_, isHash := txs[0].(common.Hash)
	require.True(t, isHash)
}

func TestRPCMarshalBlock_FullTxs(t *testing.T) {
	chainConfig := params.TestChainConfig
	header := &ethtypes.Header{
		Number:     big.NewInt(5),
		Difficulty: big.NewInt(0),
		Time:       100,
	}
	// Sign a legacy tx so it has a valid sender
	key, _ := ethKey()
	signer := ethtypes.LatestSignerForChainID(chainConfig.ChainID)
	legacyTx := &ethtypes.LegacyTx{Nonce: 0, Gas: 21000, GasPrice: big.NewInt(1)}
	tx, err := ethtypes.SignTx(ethtypes.NewTx(legacyTx), signer, key)
	require.NoError(t, err)

	block := ethtypes.NewBlock(
		header,
		&ethtypes.Body{Transactions: ethtypes.Transactions{tx}},
		nil, trie.NewStackTrie(nil),
	)
	result, err := RPCMarshalBlock(block, true, true, chainConfig)
	require.NoError(t, err)
	txs, ok := result["transactions"].([]interface{})
	require.True(t, ok)
	require.Len(t, txs, 1)
	// Full tx mode: element is *RPCTransaction
	_, isRPC := txs[0].(*RPCTransaction)
	require.True(t, isRPC)
}

func TestRPCMarshalBlock_WithWithdrawals(t *testing.T) {
	header := &ethtypes.Header{
		Number:          big.NewInt(5),
		Difficulty:      big.NewInt(0),
		Time:            100,
		WithdrawalsHash: &ethtypes.EmptyWithdrawalsHash,
	}
	withdrawals := ethtypes.Withdrawals{{Index: 1, Validator: 1, Address: common.Address{}, Amount: 100}}
	block := ethtypes.NewBlock(
		header,
		&ethtypes.Body{Withdrawals: withdrawals},
		nil, trie.NewStackTrie(nil),
	)
	result, err := RPCMarshalBlock(block, false, false, params.TestChainConfig)
	require.NoError(t, err)
	require.NotNil(t, result["withdrawals"])
}

// ---------------------------------------------------------------------------
// SimBlockResult.MarshalJSON
// ---------------------------------------------------------------------------

func TestSimBlockResult_MarshalJSON_TxHashes(t *testing.T) {
	header := &ethtypes.Header{
		Number:     big.NewInt(1),
		Difficulty: big.NewInt(0),
		Time:       100,
	}
	block := ethtypes.NewBlock(header, nil, nil, nil)
	r := &SimBlockResult{
		FullTx:      false,
		ChainConfig: params.TestChainConfig,
		Block:       block,
		Calls:       []SimCallResult{},
		Senders:     map[common.Hash]common.Address{},
	}
	bz, err := json.Marshal(r)
	require.NoError(t, err)
	require.NotNil(t, bz)

	var decoded map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(bz, &decoded))
	require.Contains(t, decoded, "calls")
}

func TestSimBlockResult_MarshalJSON_FullTx(t *testing.T) {
	chainConfig := params.TestChainConfig
	header := &ethtypes.Header{
		Number:     big.NewInt(1),
		Difficulty: big.NewInt(0),
		Time:       100,
	}

	// Build a signed tx so RPCMarshalBlock can recover the sender
	key, _ := ethKey()
	signer := ethtypes.LatestSignerForChainID(chainConfig.ChainID)
	tx, err := ethtypes.SignTx(
		ethtypes.NewTx(&ethtypes.LegacyTx{Nonce: 0, Gas: 21000, GasPrice: big.NewInt(1)}),
		signer, key,
	)
	require.NoError(t, err)

	sender, err := ethtypes.Sender(signer, tx)
	require.NoError(t, err)

	block := ethtypes.NewBlock(
		header,
		&ethtypes.Body{Transactions: ethtypes.Transactions{tx}},
		nil, trie.NewStackTrie(nil),
	)

	r := &SimBlockResult{
		FullTx:      true,
		ChainConfig: chainConfig,
		Block:       block,
		Calls:       []SimCallResult{},
		Senders:     map[common.Hash]common.Address{tx.Hash(): sender},
	}
	bz, err := json.Marshal(r)
	require.NoError(t, err)
	require.NotNil(t, bz)
}

// ethKey generates a throwaway ECDSA key for signing test transactions.
func ethKey() (*ecdsa.PrivateKey, common.Address) {
	key, err := crypto.GenerateKey()
	if err != nil {
		panic(err)
	}
	return key, crypto.PubkeyToAddress(key.PublicKey)
}

// ---------------------------------------------------------------------------
// SimStateOverride.has — MovePrecompileTo destination already in diff
// ---------------------------------------------------------------------------

// fakePrecompile is a minimal vm.PrecompiledContract for testing.
type fakePrecompile struct{ addr common.Address }

func (p *fakePrecompile) Address() common.Address     { return p.addr }
func (p *fakePrecompile) Name() string                { return "fake" }
func (p *fakePrecompile) RequiredGas(_ []byte) uint64 { return 0 }
func (p *fakePrecompile) Run(_ *vm.EVM, _ *vm.Contract, _ bool) ([]byte, error) {
	return nil, nil
}

// TestSimStateOverride_MovePrecompile_DestAlreadyInDiff verifies that when
// MovePrecompileTo targets an address that is explicitly listed in the
// SimStateOverride map, Apply returns an "already overridden" error.  This
// exercises the diff.has() helper which otherwise has 0% coverage.
func TestSimStateOverride_MovePrecompile_DestAlreadyInDiff(t *testing.T) {
	// addr1 is treated as a precompile; addr2 is also in the override map.
	addr1 := common.HexToAddress("0x0000000000000000000000000000000000000001")
	addr2 := common.HexToAddress("0xbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb")

	diff := SimStateOverride{
		addr1: SimOverrideAccount{MovePrecompileTo: &addr2},
		addr2: SimOverrideAccount{},
	}
	precompiles := vm.PrecompiledContracts{
		addr1: &fakePrecompile{addr: addr1},
	}
	err := diff.Apply(nil, precompiles)
	require.Error(t, err)
	require.Contains(t, err.Error(), "already overridden")
}

// ---------------------------------------------------------------------------
// newRPCTransaction — DynamicFee / AccessList / effectiveGasPrice branches
// ---------------------------------------------------------------------------

// TestRPCMarshalBlock_DynamicFeeTx exercises newRPCTransaction for
// DynamicFeeTxType (including the effectiveGasPrice path) via
// RPCMarshalBlock with fullTx=true.
func TestRPCMarshalBlock_DynamicFeeTx(t *testing.T) {
	chainID := big.NewInt(1)
	signer := ethtypes.NewLondonSigner(chainID)
	key, _ := ethKey()

	// tip + baseFee == gasFeeCap: fee = min(tip+base, feeCap) = feeCap = baseFee
	baseFee := big.NewInt(2e9)
	tip := big.NewInt(0)
	feeCap := new(big.Int).Add(tip, baseFee) // tip+base == feeCap

	rawTx := ethtypes.NewTx(&ethtypes.DynamicFeeTx{
		ChainID:   chainID,
		Nonce:     0,
		Gas:       21000,
		GasFeeCap: feeCap,
		GasTipCap: tip,
	})
	signed, err := ethtypes.SignTx(rawTx, signer, key)
	require.NoError(t, err)

	header := &ethtypes.Header{
		Number:     big.NewInt(10),
		Difficulty: big.NewInt(0),
		BaseFee:    baseFee,
	}
	body := &ethtypes.Body{Transactions: ethtypes.Transactions{signed}}
	block := ethtypes.NewBlock(header, body, nil, trie.NewStackTrie(nil))

	cfg := params.MainnetChainConfig
	fields, err := RPCMarshalBlock(block, true, true, cfg)
	require.NoError(t, err)
	txs, ok := fields["transactions"].([]any)
	require.True(t, ok)
	require.Len(t, txs, 1)
	rpcTx, ok := txs[0].(*RPCTransaction)
	require.True(t, ok)
	// With baseFee, GasPrice = effectiveGasPrice = min(tip+base, feeCap).
	require.NotNil(t, rpcTx.GasFeeCap)
	require.NotNil(t, rpcTx.GasTipCap)
}

// TestRPCMarshalBlock_DynamicFeeTx_CapClamped tests the branch of
// effectiveGasPrice where tip+baseFee > gasFeeCap, so the result is clamped
// to gasFeeCap.
func TestRPCMarshalBlock_DynamicFeeTx_CapClamped(t *testing.T) {
	chainID := big.NewInt(1)
	signer := ethtypes.NewLondonSigner(chainID)
	key, _ := ethKey()

	// tip + baseFee > feeCap → effectiveGasPrice = feeCap
	feeCap := big.NewInt(1e9)
	tip := big.NewInt(5e8)
	baseFee := big.NewInt(2e9) // tip+base = 2.5e9 > 1e9

	rawTx := ethtypes.NewTx(&ethtypes.DynamicFeeTx{
		ChainID:   chainID,
		Nonce:     0,
		Gas:       21000,
		GasFeeCap: feeCap,
		GasTipCap: tip,
	})
	signed, err := ethtypes.SignTx(rawTx, signer, key)
	require.NoError(t, err)

	header := &ethtypes.Header{
		Number:     big.NewInt(10),
		Difficulty: big.NewInt(0),
		BaseFee:    baseFee,
	}
	body := &ethtypes.Body{Transactions: ethtypes.Transactions{signed}}
	block := ethtypes.NewBlock(header, body, nil, trie.NewStackTrie(nil))

	cfg := params.MainnetChainConfig
	fields, err := RPCMarshalBlock(block, true, true, cfg)
	require.NoError(t, err)
	txs, ok := fields["transactions"].([]any)
	require.True(t, ok)
	rpcTx, ok := txs[0].(*RPCTransaction)
	require.True(t, ok)
	require.Equal(t, (*hexutil.Big)(feeCap), rpcTx.GasPrice)
}

// TestRPCMarshalBlock_AccessListTx exercises newRPCTransaction for
// AccessListTxType via RPCMarshalBlock with fullTx=true.
func TestRPCMarshalBlock_AccessListTx(t *testing.T) {
	chainID := big.NewInt(1)
	signer := ethtypes.NewEIP2930Signer(chainID)
	key, from := ethKey()

	rawTx := ethtypes.NewTx(&ethtypes.AccessListTx{
		ChainID:  chainID,
		Nonce:    0,
		Gas:      21000,
		GasPrice: big.NewInt(1e9),
		AccessList: ethtypes.AccessList{
			{Address: from, StorageKeys: []common.Hash{{0x01}}},
		},
	})
	signed, err := ethtypes.SignTx(rawTx, signer, key)
	require.NoError(t, err)

	header := &ethtypes.Header{Number: big.NewInt(5), Difficulty: big.NewInt(0)}
	body := &ethtypes.Body{Transactions: ethtypes.Transactions{signed}}
	block := ethtypes.NewBlock(header, body, nil, trie.NewStackTrie(nil))

	cfg := params.MainnetChainConfig
	fields, err := RPCMarshalBlock(block, true, true, cfg)
	require.NoError(t, err)
	txs, ok := fields["transactions"].([]any)
	require.True(t, ok)
	rpcTx, ok := txs[0].(*RPCTransaction)
	require.True(t, ok)
	require.NotNil(t, rpcTx.Accesses)
	require.NotNil(t, rpcTx.YParity)
}

// TestRPCMarshalBlock_LegacyTx_WithChainID exercises newRPCTransaction for
// LegacyTxType with a non-zero chain ID (EIP-155), which sets result.ChainID.
func TestRPCMarshalBlock_LegacyTx_WithChainID(t *testing.T) {
	chainID := big.NewInt(1)
	signer := ethtypes.NewEIP155Signer(chainID)
	key, _ := ethKey()

	rawTx := ethtypes.NewTx(&ethtypes.LegacyTx{
		Nonce:    0,
		Gas:      21000,
		GasPrice: big.NewInt(1e9),
	})
	signed, err := ethtypes.SignTx(rawTx, signer, key)
	require.NoError(t, err)

	header := &ethtypes.Header{Number: big.NewInt(3), Difficulty: big.NewInt(0)}
	body := &ethtypes.Body{Transactions: ethtypes.Transactions{signed}}
	block := ethtypes.NewBlock(header, body, nil, trie.NewStackTrie(nil))

	cfg := params.MainnetChainConfig
	fields, err := RPCMarshalBlock(block, true, true, cfg)
	require.NoError(t, err)
	txs, ok := fields["transactions"].([]any)
	require.True(t, ok)
	rpcTx, ok := txs[0].(*RPCTransaction)
	require.True(t, ok)
	require.NotNil(t, rpcTx.ChainID)
}

// TestRPCMarshalBlock_SetCodeTx exercises newRPCTransaction for SetCodeTxType
// via RPCMarshalBlock with fullTx=true.
func TestRPCMarshalBlock_SetCodeTx(t *testing.T) {
	chainID := big.NewInt(1)
	baseFee := big.NewInt(1e9)

	// SetCodeTx without signing: sender recovery yields zero address.
	rawTx := ethtypes.NewTx(&ethtypes.SetCodeTx{
		ChainID:   uint256.NewInt(1),
		Nonce:     0,
		Gas:       21000,
		GasFeeCap: uint256.NewInt(2e9),
		GasTipCap: uint256.NewInt(1e8),
	})

	header := &ethtypes.Header{
		Number:     big.NewInt(10),
		Difficulty: big.NewInt(0),
		BaseFee:    baseFee,
	}
	body := &ethtypes.Body{Transactions: ethtypes.Transactions{rawTx}}
	block := ethtypes.NewBlock(header, body, nil, trie.NewStackTrie(nil))

	cfg := params.ChainConfig{
		ChainID:             chainID,
		LondonBlock:         big.NewInt(0),
		CancunTime:          new(uint64),
		PragueTime:          new(uint64),
		BerlinBlock:         big.NewInt(0),
		MuirGlacierBlock:    big.NewInt(0),
		IstanbulBlock:       big.NewInt(0),
		PetersburgBlock:     big.NewInt(0),
		ConstantinopleBlock: big.NewInt(0),
		ByzantiumBlock:      big.NewInt(0),
		HomesteadBlock:      big.NewInt(0),
		EIP150Block:         big.NewInt(0),
		EIP155Block:         big.NewInt(0),
		EIP158Block:         big.NewInt(0),
	}
	fields, err := RPCMarshalBlock(block, true, true, &cfg)
	require.NoError(t, err)
	txs, ok := fields["transactions"].([]any)
	require.True(t, ok)
	require.Len(t, txs, 1)
	rpcTx, ok := txs[0].(*RPCTransaction)
	require.True(t, ok)
	require.NotNil(t, rpcTx.GasFeeCap)
	require.NotNil(t, rpcTx.GasTipCap)
}

// ---------------------------------------------------------------------------
// RPCMarshalHeader with RequestsHash
// ---------------------------------------------------------------------------

// TestRPCMarshalHeader_WithRequestsHash covers the RequestsHash non-nil branch.
func TestRPCMarshalHeader_WithRequestsHash(t *testing.T) {
	h := common.HexToHash("0xdeadbeef")
	header := &ethtypes.Header{
		Number:       big.NewInt(1),
		Difficulty:   big.NewInt(0),
		RequestsHash: &h,
	}
	result := RPCMarshalHeader(header)
	require.Equal(t, &h, result["requestsHash"])
}

// ---------------------------------------------------------------------------
// MakeHeader with Difficulty override
// ---------------------------------------------------------------------------

// TestMakeHeader_WithDifficulty covers the Difficulty override branch.
func TestMakeHeader_WithDifficulty(t *testing.T) {
	base := &ethtypes.Header{Number: big.NewInt(1), Difficulty: big.NewInt(0)}
	diff := big.NewInt(999)
	override := &SimBlockOverrides{Difficulty: (*hexutil.Big)(diff)}
	result := override.MakeHeader(base)
	require.Equal(t, diff, result.Difficulty)
}

// ---------------------------------------------------------------------------
// NewRevertError with valid ABI-encoded reason
// ---------------------------------------------------------------------------

// TestNewRevertError_WithReason covers the errUnpack == nil branch.
func TestNewRevertError_WithReason(t *testing.T) {
	// ABI-encoded: Error(string) = selector + offset + length + data
	// Selector: 0x08c379a0, reason: "out of gas"
	data, _ := hex.DecodeString(
		"08c379a0" +
			"0000000000000000000000000000000000000000000000000000000000000020" +
			"000000000000000000000000000000000000000000000000000000000000000a" +
			"6f7574206f662067617300000000000000000000000000000000000000000000",
	)
	e := NewRevertError(data)
	require.Contains(t, e.Message, "out of gas")
	require.Equal(t, 3, e.Code)
}

// ---------------------------------------------------------------------------
// RPCMarshalBlock with uncle headers
// ---------------------------------------------------------------------------

// TestRPCMarshalBlock_WithUncles covers the uncle-hash loop.
func TestRPCMarshalBlock_WithUncles(t *testing.T) {
	chainID := big.NewInt(1)
	cfg := &params.ChainConfig{
		ChainID:             chainID,
		HomesteadBlock:      big.NewInt(0),
		EIP150Block:         big.NewInt(0),
		EIP155Block:         big.NewInt(0),
		EIP158Block:         big.NewInt(0),
		ByzantiumBlock:      big.NewInt(0),
		ConstantinopleBlock: big.NewInt(0),
		PetersburgBlock:     big.NewInt(0),
		IstanbulBlock:       big.NewInt(0),
		MuirGlacierBlock:    big.NewInt(0),
		BerlinBlock:         big.NewInt(0),
	}
	uncle := &ethtypes.Header{Number: big.NewInt(1), Difficulty: big.NewInt(0)}
	header := &ethtypes.Header{
		Number:     big.NewInt(2),
		Difficulty: big.NewInt(0),
	}
	body := &ethtypes.Body{Uncles: []*ethtypes.Header{uncle}}
	block := ethtypes.NewBlock(header, body, nil, trie.NewStackTrie(nil))
	fields, err := RPCMarshalBlock(block, false, false, cfg)
	require.NoError(t, err)
	uncles, ok := fields["uncles"].([]common.Hash)
	require.True(t, ok)
	require.Len(t, uncles, 1)
	require.Equal(t, uncle.Hash(), uncles[0])
}

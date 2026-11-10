package keeper_test

import (
	"fmt"
	"math/big"
	"math/rand"
	"strings"

	sdkmath "cosmossdk.io/math"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core"
	"github.com/ethereum/go-ethereum/core/tracing"
	"github.com/ethereum/go-ethereum/core/vm"
	"github.com/ethereum/go-ethereum/params"
	"github.com/holiman/uint256"

	"github.com/evmos/ethermint/x/evm/statedb"
	"github.com/evmos/ethermint/x/evm/types"
)

// The EVM runs a generated interpreter loop when no tracer is attached and the
// table-driven loop otherwise (including for debug_trace*). Consensus uses the
// generated loop, so both must produce identical results through ethermint's
// StateDB, or traces would disagree with executed blocks.

type loopOutcome struct {
	err     string
	ret     string
	gasUsed uint64
	vmErr   string
	logs    string
	state   string
}

type loopStats struct {
	succeeded int
	withLogs  int
	gasValues map[uint64]struct{}
}

func (suite *KeeperTestSuite) runInterpreterLoop(msg *core.Message, tracer *tracing.Hooks, watch []common.Address) loopOutcome {
	ctx, _ := suite.Ctx.CacheContext()
	m := *msg
	res, err := suite.App.EvmKeeper.ApplyMessage(ctx, &m, tracer, true)
	if err != nil {
		return loopOutcome{err: err.Error()}
	}
	out := loopOutcome{
		ret:     common.Bytes2Hex(res.Ret),
		gasUsed: res.GasUsed,
		vmErr:   res.VmError,
		logs:    fmt.Sprintf("%v", res.Logs),
	}
	for _, addr := range watch {
		out.state += watchedState(ctx, suite, addr)
	}
	return out
}

func watchedState(ctx sdk.Context, suite *KeeperTestSuite, addr common.Address) string {
	k := suite.App.EvmKeeper
	var s strings.Builder
	for i := range int64(8) {
		s.WriteString(k.GetState(ctx, addr, common.BigToHash(big.NewInt(i))).Hex())
	}
	s.WriteString(k.GetEVMDenomBalance(ctx, addr).String())
	if acct := k.GetAccount(ctx, addr); acct != nil {
		fmt.Fprintf(&s, "%d%x", acct.Nonce, acct.CodeHash)
	}
	return s.String()
}

func (suite *KeeperTestSuite) requireSameLoopOutcome(name string, msg *core.Message, watch []common.Address, stats *loopStats) loopOutcome {
	generated := suite.runInterpreterLoop(msg, nil, watch)
	table := suite.runInterpreterLoop(msg, &tracing.Hooks{}, watch)
	suite.Require().Equal(table, generated, "interpreter loops diverge on %s", name)

	if stats == nil {
		return generated
	}
	if generated.err == "" && generated.vmErr == "" {
		stats.succeeded++
	}
	if generated.logs != "[]" {
		stats.withLogs++
	}
	stats.gasValues[generated.gasUsed] = struct{}{}
	return generated
}

// requireLoopResult pins a hand-written case to its intended result, so an edit
// that makes it halt early cannot pass by both loops failing the same way.
func (suite *KeeperTestSuite) requireLoopResult(out loopOutcome, wantErr string) {
	suite.Require().Empty(out.err)
	if wantErr == "" {
		suite.Require().Empty(out.vmErr)
		return
	}
	suite.Require().Contains(out.vmErr, wantErr)
}

func newCallMessage(from common.Address, to *common.Address, data []byte, gas uint64) *core.Message {
	return &core.Message{
		From:      from,
		To:        to,
		Value:     new(uint256.Int),
		GasLimit:  gas,
		GasPrice:  new(uint256.Int),
		GasFeeCap: new(uint256.Int),
		GasTipCap: new(uint256.Int),
		Data:      data,
	}
}

func (suite *KeeperTestSuite) installContract(addr common.Address, code []byte) {
	db := statedb.New(suite.Ctx, suite.App.EvmKeeper, statedb.NewEmptyTxConfig(common.BytesToHash(suite.Ctx.HeaderHash())))
	db.SetNonce(addr, 1, tracing.NonceChangeUnspecified)
	db.SetCode(addr, code, tracing.CodeChangeUnspecified)
	db.SetState(addr, common.BigToHash(big.NewInt(1)), common.BigToHash(big.NewInt(7)))
	db.AddBalance(addr, uint256.NewInt(1_000_000), tracing.BalanceChangeUnspecified)
	suite.Require().NoError(db.Commit())
}

// asm assembles opcodes; an int becomes PUSH1 and a []byte becomes PUSHn.
func asm(parts ...any) []byte {
	var out []byte
	for _, part := range parts {
		switch v := part.(type) {
		case vm.OpCode:
			out = append(out, byte(v))
		case int:
			out = append(out, byte(vm.PUSH1), byte(v))
		case []byte:
			out = append(out, byte(vm.PUSH1)+byte(len(v)-1))
			out = append(out, v...)
		}
	}
	return out
}

// arithmeticProgram applies every arithmetic, comparison and bitwise opcode to
// signed and unsigned operands and returns all results, so a wrong result from
// any single opcode changes the return data.
func arithmeticProgram() []byte {
	operands := [][]byte{
		common.FromHex("0xfffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff0"),
		{0x07},
		{0x80},
		common.FromHex("0x8000000000000000000000000000000000000000000000000000000000000000"),
	}
	binary := []vm.OpCode{
		vm.ADD, vm.MUL, vm.SUB, vm.DIV, vm.SDIV, vm.MOD, vm.SMOD, vm.EXP, vm.SIGNEXTEND,
		vm.LT, vm.GT, vm.SLT, vm.SGT, vm.EQ, vm.AND, vm.OR, vm.XOR, vm.BYTE, vm.SHL, vm.SHR, vm.SAR,
	}
	var code []byte
	offset := 0
	store := func() {
		code = append(code, asm([]byte{byte(offset >> 8), byte(offset)}, vm.MSTORE)...)
		offset += 32
	}
	for _, a := range operands {
		for _, b := range operands {
			for _, op := range binary {
				code = append(code, asm(b, a, op)...)
				store()
			}
			for _, op := range []vm.OpCode{vm.ADDMOD, vm.MULMOD} {
				code = append(code, asm([]byte{0x0b}, b, a, op)...)
				store()
			}
		}
		for _, op := range []vm.OpCode{vm.ISZERO, vm.NOT, vm.CLZ} {
			code = append(code, asm(a, op)...)
			store()
		}
	}
	code = append(code, asm([]byte{byte(offset >> 8), byte(offset)}, 0, vm.KECCAK256)...)
	store()
	return append(code, asm([]byte{byte(offset >> 8), byte(offset)}, 0, vm.RETURN)...)
}

func (suite *KeeperTestSuite) TestInterpreterLoopsAgree() {
	suite.SetupTest()
	k := suite.App.EvmKeeper
	from := suite.Address

	suite.Require().Nil(
		k.Tracer(suite.Ctx, core.Message{}, params.TestChainConfig),
		"the default tracer must be nil, otherwise both runs use the table loop",
	)
	suite.Require().NoError(k.SetBalance(suite.Ctx, from, *uint256.NewInt(1e18), types.DefaultEVMDenom))

	// Report raw EVM gas rather than max(gasLimit*multiplier, used), which would
	// hide gas divergences.
	fmParams := suite.App.FeeMarketKeeper.GetParams(suite.Ctx)
	fmParams.MinGasMultiplier = sdkmath.LegacyZeroDec()
	suite.Require().NoError(suite.App.FeeMarketKeeper.SetParams(suite.Ctx, fmParams))
	k.RemoveParamsCache(suite.Ctx)

	suite.Run("erc20", func() {
		ctorArgs, err := types.ERC20Contract.ABI.Pack("", from, big.NewInt(1_000_000))
		suite.Require().NoError(err)
		deploy := append(append([]byte{}, types.ERC20Contract.Bin...), ctorArgs...)
		out := suite.requireSameLoopOutcome("deploy", newCallMessage(from, nil, deploy, 5_000_000), nil, nil)
		suite.requireLoopResult(out, "")

		erc20 := suite.DeployTestContract(suite.T(), from, big.NewInt(1_000_000), false)
		recipient := common.HexToAddress("0x1234")
		transfer, err := types.ERC20Contract.ABI.Pack("transfer", recipient, big.NewInt(10))
		suite.Require().NoError(err)
		overdraw, err := types.ERC20Contract.ABI.Pack("transfer", recipient, big.NewInt(1e18))
		suite.Require().NoError(err)
		balanceOf, err := types.ERC20Contract.ABI.Pack("balanceOf", from)
		suite.Require().NoError(err)

		watch := []common.Address{erc20}
		for _, tc := range []struct {
			name    string
			data    []byte
			gas     uint64
			wantErr string
		}{
			{"transfer", transfer, 200_000, ""},
			{"transfer revert", overdraw, 200_000, "execution reverted"},
			{"balanceOf", balanceOf, 200_000, ""},
			{"transfer out of gas", transfer, 30_000, "out of gas"},
		} {
			out := suite.requireSameLoopOutcome(tc.name, newCallMessage(from, &erc20, tc.data, tc.gas), watch, nil)
			suite.requireLoopResult(out, tc.wantErr)
		}
	})

	target := common.HexToAddress("0xc0de")
	beneficiary := common.HexToAddress("0x99")
	programs := []struct {
		name    string
		wantErr string
		code    []byte
	}{
		{"arithmetic and bitwise", "", arithmeticProgram()},
		{"sstore refunds", "", asm(0, 1, vm.SSTORE, 5, 2, vm.SSTORE, 0, 2, vm.SSTORE, vm.STOP)},
		{"sload cold and warm", "", asm(1, vm.SLOAD, 1, vm.SLOAD, vm.ADD, 3, vm.SSTORE, vm.STOP)},
		{"transient storage", "", asm(9, 1, vm.TSTORE, 1, vm.TLOAD, 4, vm.SSTORE, vm.STOP)},
		{"log2", "", asm(0xaa, 0, vm.MSTORE, 0xbb, 0xcc, 32, 0, vm.LOG2, vm.STOP)},
		{"sha256 precompile", "", asm(
			0x42, 0, vm.MSTORE, 32, 32, 32, 0, 2, vm.GAS, vm.STATICCALL, 0, vm.SSTORE,
			32, vm.MLOAD, 5, vm.SSTORE, vm.STOP,
		)},
		{"create2", "", asm(
			[]byte{0x60, 0x00, 0x60, 0x00, 0xf3}, 0, vm.MSTORE,
			7, 5, 27, 0, vm.CREATE2, 6, vm.SSTORE, vm.STOP,
		)},
		{"selfdestruct", "", asm([]byte{0x99}, vm.SELFDESTRUCT)},
		{"invalid jump", "invalid jump destination", asm(3, vm.JUMP, vm.STOP)},
		{"revert with data", "execution reverted", asm(0xee, 0, vm.MSTORE, 32, 0, vm.REVERT)},
		{"stack underflow", "stack underflow", asm(vm.ADD)},
		{"environment", "", asm(
			vm.NUMBER, vm.TIMESTAMP, vm.ADD, vm.BASEFEE, vm.ADD, vm.CHAINID, vm.ADD, vm.GASLIMIT, vm.ADD,
			vm.SELFBALANCE, vm.ADD, vm.CALLER, vm.ADD, vm.PREVRANDAO, vm.ADD, 0, vm.SSTORE,
			1, vm.NUMBER, vm.SUB, vm.BLOCKHASH, 7, vm.SSTORE, vm.STOP,
		)},
		{"mcopy and clz", "", asm(0x77, 0, vm.MSTORE, 32, 0, 64, vm.MCOPY, 64, vm.MLOAD, vm.CLZ, 3, vm.SSTORE, vm.STOP)},
		{"memory expansion out of gas", "out of gas", asm([]byte{0xff, 0xff, 0xff}, vm.MLOAD, vm.STOP)},
		{"recursive self call", "", asm(0, 0, 0, 0, 0, vm.ADDRESS, vm.GAS, vm.CALL, 1, vm.SLOAD, 1, vm.ADD, 1, vm.SSTORE, vm.STOP)},
		{"loop until out of gas", "out of gas", asm(0, vm.JUMPDEST, 1, vm.ADD, 2, vm.JUMP)},
	}
	for _, tc := range programs {
		suite.Run(tc.name, func() {
			suite.installContract(target, tc.code)
			out := suite.requireSameLoopOutcome(tc.name, newCallMessage(from, &target, nil, 2_000_000), []common.Address{target, beneficiary}, nil)
			suite.requireLoopResult(out, tc.wantErr)
		})
	}

	suite.Run("random programs", func() {
		var opcodes []vm.OpCode
		for op := range 256 {
			o := vm.OpCode(op)
			if o.IsPush() || o == vm.SELFDESTRUCT {
				continue
			}
			if o.String() != fmt.Sprintf("opcode %#x not defined", op) {
				opcodes = append(opcodes, o)
			}
		}

		stats := loopStats{gasValues: make(map[uint64]struct{})}
		rng := rand.New(rand.NewSource(1)) //nolint:gosec // deterministic test input
		const count = 3000
		for i := range count {
			// Seed the stack with small values so they work as offsets, sizes and
			// slots, letting execution get past the first few opcodes.
			var code []byte
			for range 12 {
				code = append(code, byte(vm.PUSH1), byte(rng.Intn(64)))
			}
			for range 1 + rng.Intn(60) {
				if rng.Intn(3) == 0 {
					code = append(code, byte(vm.PUSH1), byte(rng.Intn(64)))
					continue
				}
				code = append(code, byte(opcodes[rng.Intn(len(opcodes))]))
			}
			suite.installContract(target, code)
			suite.requireSameLoopOutcome(
				fmt.Sprintf("random program #%d %x", i, code),
				newCallMessage(from, &target, []byte{1, 2, 3, 4}, 500_000),
				[]common.Address{target},
				&stats,
			)
		}

		// Guard against the generator degrading into programs that halt
		// immediately, which would make the comparison meaningless.
		suite.Require().Greater(stats.succeeded, count/20, "too few random programs ran to completion")
		suite.Require().Greater(stats.withLogs, count/200, "too few random programs emitted logs")
		suite.Require().Greater(len(stats.gasValues), count/30, "too few distinct gas values")
	})
}

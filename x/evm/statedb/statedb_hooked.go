package statedb

import (
	"bytes"
	"math/big"
	"slices"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/ethereum/go-ethereum/common"
	ethstate "github.com/ethereum/go-ethereum/core/state"
	"github.com/ethereum/go-ethereum/core/stateless"
	"github.com/ethereum/go-ethereum/core/tracing"
	ethtypes "github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/core/types/bal"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/params"
	"github.com/holiman/uint256"
)

// hookedStateDB represents a statedb which emits calls to tracing-hooks
// on state operations.
type HookedStateDB struct {
	inner *StateDB
	hooks *tracing.Hooks
}

// NewHookedState wraps the given stateDb with the given hooks
func NewHookedState(stateDB *StateDB, hooks *tracing.Hooks) *HookedStateDB {
	s := &HookedStateDB{stateDB, hooks}
	if s.hooks == nil {
		s.hooks = new(tracing.Hooks)
	}
	return s
}

func (s *HookedStateDB) CreateAccount(addr common.Address) {
	s.inner.CreateAccount(addr)
}

func (s *HookedStateDB) CreateContract(addr common.Address) {
	s.inner.CreateContract(addr)
}

func (s *HookedStateDB) GetBalance(addr common.Address) *uint256.Int {
	return s.inner.GetBalance(addr)
}

func (s *HookedStateDB) GetNonce(addr common.Address) uint64 {
	return s.inner.GetNonce(addr)
}

func (s *HookedStateDB) GetCodeHash(addr common.Address) common.Hash {
	return s.inner.GetCodeHash(addr)
}

func (s *HookedStateDB) GetCode(addr common.Address) []byte {
	return s.inner.GetCode(addr)
}

func (s *HookedStateDB) GetCodeSize(addr common.Address) int {
	return s.inner.GetCodeSize(addr)
}

func (s *HookedStateDB) AddRefund(u uint64) {
	s.inner.AddRefund(u)
}

func (s *HookedStateDB) SubRefund(u uint64) {
	s.inner.SubRefund(u)
}

func (s *HookedStateDB) GetRefund() uint64 {
	return s.inner.GetRefund()
}

func (s *HookedStateDB) GetCommittedState(addr common.Address, hash common.Hash) common.Hash {
	return s.inner.GetCommittedState(addr, hash)
}

func (s *HookedStateDB) GetStateAndCommittedState(addr common.Address, hash common.Hash) (common.Hash, common.Hash) {
	return s.inner.GetStateAndCommittedState(addr, hash)
}

func (s *HookedStateDB) GetState(addr common.Address, hash common.Hash) common.Hash {
	return s.inner.GetState(addr, hash)
}

func (s *HookedStateDB) GetStorageRoot(addr common.Address) common.Hash {
	return s.inner.GetStorageRoot(addr)
}

func (s *HookedStateDB) GetTransientState(addr common.Address, key common.Hash) common.Hash {
	return s.inner.GetTransientState(addr, key)
}

func (s *HookedStateDB) SetTransientState(addr common.Address, key, value common.Hash) {
	s.inner.SetTransientState(addr, key, value)
}

func (s *HookedStateDB) HasSelfDestructed(addr common.Address) bool {
	return s.inner.HasSelfDestructed(addr)
}

func (s *HookedStateDB) Exist(addr common.Address) bool {
	return s.inner.Exist(addr)
}

func (s *HookedStateDB) Empty(addr common.Address) bool {
	return s.inner.Empty(addr)
}

func (s *HookedStateDB) AddressInAccessList(addr common.Address) bool {
	return s.inner.AddressInAccessList(addr)
}

func (s *HookedStateDB) SlotInAccessList(addr common.Address, slot common.Hash) (addressOk bool, slotOk bool) {
	return s.inner.SlotInAccessList(addr, slot)
}

func (s *HookedStateDB) AddAddressToAccessList(addr common.Address) {
	s.inner.AddAddressToAccessList(addr)
}

func (s *HookedStateDB) AddSlotToAccessList(addr common.Address, slot common.Hash) {
	s.inner.AddSlotToAccessList(addr, slot)
}

func (s *HookedStateDB) Prepare(rules params.Rules, sender, coinbase common.Address, dest *common.Address,
	precompiles []common.Address, txAccesses ethtypes.AccessList,
) {
	s.inner.Prepare(rules, sender, coinbase, dest, precompiles, txAccesses)
}

func (s *HookedStateDB) RevertToSnapshot(i int) {
	s.inner.RevertToSnapshot(i)
}

func (s *HookedStateDB) Snapshot() int {
	return s.inner.Snapshot()
}

func (s *HookedStateDB) AddPreimage(hash common.Hash, bytes []byte) {
	s.inner.AddPreimage(hash, bytes)
}

func (s *HookedStateDB) Witness() *stateless.Witness {
	return s.inner.Witness()
}

func (s *HookedStateDB) AccessEvents() *ethstate.AccessEvents {
	return s.inner.AccessEvents()
}

func (s *HookedStateDB) SubBalance(addr common.Address, amount *uint256.Int, reason tracing.BalanceChangeReason) uint256.Int {
	prev := s.inner.SubBalance(addr, amount, reason)
	if s.hooks.OnBalanceChange != nil && !amount.IsZero() {
		newBalance := new(uint256.Int).Sub(&prev, amount)
		s.hooks.OnBalanceChange(addr, prev.ToBig(), newBalance.ToBig(), reason)
	}
	return prev
}

func (s *HookedStateDB) AddBalance(addr common.Address, amount *uint256.Int, reason tracing.BalanceChangeReason) uint256.Int {
	prev := s.inner.AddBalance(addr, amount, reason)
	if s.hooks.OnBalanceChange != nil && !amount.IsZero() {
		newBalance := new(uint256.Int).Add(&prev, amount)
		s.hooks.OnBalanceChange(addr, prev.ToBig(), newBalance.ToBig(), reason)
	}
	return prev
}

func (s *HookedStateDB) SetNonce(address common.Address, nonce uint64, reason tracing.NonceChangeReason) {
	prev := s.inner.GetNonce(address)
	s.inner.SetNonce(address, nonce, reason)
	if s.hooks.OnNonceChangeV2 != nil {
		s.hooks.OnNonceChangeV2(address, prev, nonce, reason)
	} else if s.hooks.OnNonceChange != nil {
		s.hooks.OnNonceChange(address, prev, nonce)
	}
}

func (s *HookedStateDB) SetCode(address common.Address, code []byte, reason tracing.CodeChangeReason) []byte {
	prev := s.inner.SetCode(address, code, reason)
	if s.hooks.OnCodeChange != nil || s.hooks.OnCodeChangeV2 != nil {
		prevHash := ethtypes.EmptyCodeHash
		if len(prev) != 0 {
			prevHash = crypto.Keccak256Hash(prev)
		}
		codeHash := ethtypes.EmptyCodeHash
		if len(code) != 0 {
			codeHash = crypto.Keccak256Hash(code)
		}
		if prevHash != codeHash {
			if s.hooks.OnCodeChangeV2 != nil {
				s.hooks.OnCodeChangeV2(address, prevHash, prev, codeHash, code, reason)
			} else {
				s.hooks.OnCodeChange(address, prevHash, prev, codeHash, code)
			}
		}
	}
	return prev
}

func (s *HookedStateDB) SetState(address common.Address, key common.Hash, value common.Hash) common.Hash {
	prev := s.inner.SetState(address, key, value)
	if s.hooks.OnStorageChange != nil && prev != value {
		s.hooks.OnStorageChange(address, key, prev, value)
	}
	return prev
}

// SelfDestruct emits no balance change: the interpreter moves the balance out
// first. Nonce and code resets are reported by SelfDestructHooks.
func (s *HookedStateDB) SelfDestruct(address common.Address) {
	s.inner.SelfDestruct(address)
}

func (s *HookedStateDB) IsNewContract(addr common.Address) bool {
	return s.inner.IsNewContract(addr)
}

func (s *HookedStateDB) Touch(addr common.Address) {
	s.inner.Touch(addr)
}

func (s *HookedStateDB) SetTxContext(thash common.Hash, ti int, blockAccessIndex uint32) {
	s.inner.SetTxContext(thash, ti, blockAccessIndex)
}

func (s *HookedStateDB) AddLog(log *ethtypes.Log) {
	// The inner will modify the log (add fields), so invoke that first
	s.inner.AddLog(log)
	if s.hooks.OnLog != nil {
		s.hooks.OnLog(log)
	}
}

//nolint:misspell
func (s *HookedStateDB) Finalise(rules params.Rules) *bal.ConstructionBlockAccessList {
	s.SelfDestructHooks()()
	//nolint:misspell
	return s.inner.Finalise(rules)
}

type selfDestructed struct {
	addr     common.Address
	balance  *uint256.Int
	nonce    uint64
	codeHash common.Hash
	code     []byte
}

// SelfDestructHooks snapshots the end-of-tx hooks geth emits for accounts
// self-destructed in the current tx, and returns a func that emits them.
func (s *HookedStateDB) SelfDestructHooks() func() {
	if s.hooks.OnBalanceChange == nil && s.hooks.OnNonceChangeV2 == nil && s.hooks.OnNonceChange == nil &&
		s.hooks.OnCodeChangeV2 == nil && s.hooks.OnCodeChange == nil {
		return func() {}
	}
	var addrs []common.Address
	for addr := range s.inner.journal.dirties {
		if obj := s.inner.stateObjects[addr]; obj != nil && obj.selfDestructed {
			addrs = append(addrs, addr)
		}
	}
	if len(addrs) == 0 {
		return func() {}
	}
	slices.SortFunc(addrs, func(a, b common.Address) int { return bytes.Compare(a[:], b[:]) })

	events := make([]selfDestructed, 0, len(addrs))
	for _, addr := range addrs {
		obj := s.inner.stateObjects[addr]
		ev := selfDestructed{
			addr:     addr,
			nonce:    obj.Nonce(),
			codeHash: common.BytesToHash(obj.CodeHash()),
			code:     obj.Code(),
		}
		// The interpreter moves the balance out before SelfDestruct, so the bank
		// balance only holds funds sent afterwards, which are burned before Amsterdam.
		if s.hooks.OnBalanceChange != nil && !s.inner.isAmsterdam {
			ev.balance = s.inner.GetBalance(addr)
		}
		events = append(events, ev)
	}

	return func() {
		for _, ev := range events {
			if ev.balance != nil && ev.balance.Sign() != 0 {
				s.hooks.OnBalanceChange(ev.addr, ev.balance.ToBig(), new(big.Int), tracing.BalanceDecreaseSelfdestructBurn)
			}

			if s.hooks.OnNonceChangeV2 != nil {
				s.hooks.OnNonceChangeV2(ev.addr, ev.nonce, 0, tracing.NonceChangeSelfdestruct)
			} else if s.hooks.OnNonceChange != nil {
				s.hooks.OnNonceChange(ev.addr, ev.nonce, 0)
			}

			// If an initcode invokes selfdestruct, do not emit a code change.
			if ev.codeHash == ethtypes.EmptyCodeHash {
				continue
			}
			if s.hooks.OnCodeChangeV2 != nil {
				s.hooks.OnCodeChangeV2(ev.addr, ev.codeHash, ev.code, ethtypes.EmptyCodeHash, nil, tracing.CodeChangeSelfDestruct)
			} else if s.hooks.OnCodeChange != nil {
				s.hooks.OnCodeChange(ev.addr, ev.codeHash, ev.code, ethtypes.EmptyCodeHash, nil)
			}
		}
	}
}

func (s *HookedStateDB) Error() error {
	return s.inner.Error()
}

// Impl ExtStateDB interface

// ExecuteNativeAction executes native action in isolate,
// the writes will be revert when either the native action itself fail
// or the wrapping message call reverted.
func (s *HookedStateDB) ExecuteNativeAction(contract common.Address, converter EventConverter, action func(ctx sdk.Context) error) error {
	return s.inner.ExecuteNativeAction(contract, converter, action)
}

// Context returns the current context for query native state in precompiles.
func (s *HookedStateDB) Context() sdk.Context {
	return s.inner.Context()
}

// Transfer transfers amount from sender to recipient.
func (s *HookedStateDB) Transfer(sender, recipient common.Address, amount *uint256.Int) {
	s.inner.Transfer(sender, recipient, amount)
}

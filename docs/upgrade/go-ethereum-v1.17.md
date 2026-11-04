# Go-ethereum v1.17

Ethermint now builds on go-ethereum v1.17.10, through the crypto-org-chain fork
that carries the Cronos stateful precompiles. The chain still runs Osaka rules.

## What's new

- **Amsterdam fork support (disabled)**: a new `amsterdam_time` chain config
  param and best-effort support for the Amsterdam EIPs:
    - EIP-7708: ETH transfers emit logs.
    - EIP-8024: `SWAPN`, `DUPN` and `EXCHANGE` opcodes.
    - EIP-8246: SELFDESTRUCT no longer burns the balance.
    - EIP-8038: state-access gas repricing.
    - EIP-7976 and EIP-7981: calldata floor cost and access-list cost.
    - EIP-7954: larger maximum contract and init code size.
    - EIP-8037: state creation gas, charged from a single gas pool.

  It cannot be activated yet, see [Not implemented](#not-implemented-todo).
- **Multi-dimensional gas**: the EVM tracks gas as a `vm.GasBudget` (execution
  gas plus an EIP-8037 state-gas reservoir). Before Amsterdam the reservoir
  stays empty and gas behaves as before.
- **Faster EVM execution**: upstream interpreter work is included, such as the
  stack arena, in-place stack operations, the JUMPDEST bitmap cache, faster
  opcode and DUP/SWAP dispatch, and `uint256` amounts in `core.Message`.
- **Tracing**: new `OnGasChangeV2`, `OnNonceChangeV2` and `OnCodeChangeV2`
  hooks; self-destructed accounts now also report their nonce reset;
  `callTracer` reports the index of each log within its transaction.
- **EIP-7825 gas cap in `eth_estimateGas`**: under Osaka, estimates never exceed
  `MaxTxGas` (2^24).
- **Binary trie replaces Verkle**: the Verkle tree code (`go-verkle`, `go-ipa`)
  is gone upstream.
- **Security**: the upstream v1.17.x releases fix several CVEs, including a
  timing-based key extraction issue in `crypto/ecies`.

## Breaking changes

### State machine

- **Static-context precompile calls**: a stateful precompile reached by a plain
  `CALL` from inside a static frame now gets `readonly = true` (before: `false`).
- **SELFDESTRUCT**: the interpreter moves the balance itself before calling
  `StateDB.SelfDestruct`. Final balances are unchanged, but the bank events are
  emitted in a different order.
- **Fork rules always assume the merge**, as the interpreter does, so the
  active forks no longer depend on `merge_netsplit_block`. On chains that leave
  it unset, tracing blocks executed before the upgrade can report different gas
  than the original execution.
- **EIP-7825 per-tx gas cap**: enforced in the ante handler and in
  `ApplyTransaction`. `eth_call`, `eth_simulateV1` and messages that modules
  apply directly through `ApplyMessage`/`ApplyMessageWithConfig` are not capped.

### Stateful precompiles

- `Contract.Gas` is a `vm.GasBudget`, and `Contract.UseGas`/`RefundGas` are
  removed. Charge gas with `contract.Gas.ChargeExecution(cost)` and return
  `vm.ErrOutOfGas` when it returns `false`.
- `vm.ValidatePrecompiles` rejects a precompile whose `Address()` differs from
  the address it is registered at.
- Do not implement `vm.CacheablePrecompile`: geth may serve cacheable
  precompiles from a shared result cache keyed only on the input.

### Go API

- `core.Message`: `Value`, `GasPrice`, `GasFeeCap` and `GasTipCap` are
  `*uint256.Int`. The skip flags are unexported; set them with
  `SkipNonceCheck()` and `SkipEOACheck()`.
- `MsgEthereumTx.AsMessage` returns `(*core.Message, error)`. `AsMessage`,
  `TransactionArgs.ToMessage` and `ToSimMessage` reject negative values and
  values above 256 bits.
- `TransactionArgs.ToSimMessage(baseFee)` drops the `skipNonce` argument.
- `GetEthIntrinsicGas(msg, rules)` drops `isContractCreation` and derives it
  from `msg.To`.
- `EVM.Call` and `EVM.Create` take and return a `vm.GasBudget`.
- `statedb.Transfer` takes the fork rules (`*params.Rules`).
- `core.ErrMaxInitCodeSizeExceeded` moved to `vm.ErrMaxInitCodeSizeExceeded`.
- `vm.StateDB`: `SelfDestruct` returns nothing, `SelfDestruct6780` and
  `PointCache` are removed, `IsNewContract`, `Touch` and `SetTxContext` are
  added, and `Finalise(rules)` replaces `Finalise(bool)` and returns a nil block
  access list.
- `HookedStateDB.SelfDestructHooks` snapshots the end-of-tx self-destruct
  tracing hooks and returns a func that emits them: call it before `Commit` and
  run the func after. `HookedStateDB.SelfDestruct` only reports a balance that
  is still on the account.

### JSON-RPC

- `debug_trace*` struct logs follow the new upstream format: memory words and
  storage keys/values are `0x`-prefixed, and `error` is omitted when empty.
- `eth_simulateV1` returns error code `-38012` (base fee too low) instead of
  `-32602` for `ErrFeeCapTooLow`, matching geth.
- `eth_simulateV1` skips the EIP-7002/EIP-7251 system calls when those contracts
  have no code, since v1.17 errors on empty system contracts.

## Not implemented (TODO)

### Amsterdam

Genesis validation and `MsgUpdateParams` reject `amsterdam_time`
(`ChainConfig.ValidateAmsterdamDisabled`) until the first item is done.

- [ ] **Runtime state charges** (EIP-2780/EIP-8037): top-level contract
  creation, value transfers to empty accounts, EIP-7702 authorizations and the
  top-frame halt drain are underpriced. Port them, with their refunds, into
  `ApplyMessageWithConfig` and `eth_simulateV1`.
- [ ] **Two-dimensional gas**: state-gas reservoir, `MaxTxTotalGas` and
  `max(execution, state)` block accounting. Ethermint keeps one gas pool, so
  transactions above `MaxTxGas` are rejected, and tracers see one gas dimension.
- [ ] **SLOTNUM** (EIP-7843): returns `0`; needs a value such as the block
  height.
- [ ] **Block-level access lists** (EIP-7928).
- [ ] **CREATE2 factory predeploy** (EIP-7997): needs an upgrade handler.

### JSON-RPC

- [ ] `eth_capabilities`
- [ ] `debug_clearTxpool`
- [ ] `debug_replayBadBlock`

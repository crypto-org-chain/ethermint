# go-ethereum v1.17

Ethermint runs go-ethereum v1.17.10 through the `crypto-org-chain/go-ethereum`
fork, which carries the stateful precompile patch. The EVM runs Osaka rules.
Amsterdam is implemented on a best-effort basis but cannot be activated yet.

## What's new

- **Stateful precompiles**: `Contract.Gas` is a `vm.GasBudget`, so precompiles
  charge gas with `contract.Gas.ChargeExecution` instead of `contract.UseGas`.
  A call from a static context now gets `readonly = true` (state machine
  breaking), and `ValidatePrecompiles` rejects a precompile registered at an
  address other than its `Address()`.
- **SELFDESTRUCT**: the interpreter moves the balance itself before calling
  `StateDB.SelfDestruct`. Resulting balances are unchanged, but the order of the
  emitted bank events differs.
- **Fork rules**: the block config always assumes the merge, as the interpreter
  does, so the active forks no longer depend on `merge_netsplit_block`. On chains
  that leave it unset, tracing blocks executed before the upgrade can report
  different gas than the original execution.
- **EIP-7825 gas cap**: enforced in the ante handler and `ApplyTransaction`, and
  `eth_estimateGas` never returns more than the cap. `eth_call`, `eth_simulateV1`
  and messages applied directly by modules are not capped.
- **API changes**:
    - `core.Message` amounts are `*uint256.Int`; `AsMessage`, `ToMessage` and
      `ToSimMessage` return an error for values above 256 bits.
    - `GetEthIntrinsicGas(msg, rules)` derives contract creation from `msg.To`.
    - `statedb.Transfer` takes the fork rules.
    - `vm.StateDB`: `SelfDestruct` returns nothing, `SelfDestruct6780` and
      `PointCache` are removed, `IsNewContract`, `Touch` and `SetTxContext` are
      added, and `Finalise(rules)` returns a nil block access list.
    - `TransactionArgs.ToSimMessage(baseFee)` drops the `skipNonce` argument.
    - `HookedStateDB.SelfDestructHooks` snapshots the end-of-tx self-destruct
      tracing hooks and returns a func that emits them: call it before `Commit`
      and run the func after.
- **Amsterdam (disabled)**: a new `amsterdam_time` chain config param, with
  EIP-7708 transfer logs, EIP-8246 (SELFDESTRUCT keeps the balance), the
  EIP-8024 opcodes, EIP-8038 repricing, the EIP-7976/EIP-7981 calldata and
  access-list costs, the larger code size limits, and EIP-8037 state gas charged
  from a single gas pool.

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

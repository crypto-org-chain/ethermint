# go-ethereum v1.17.10 upgrade

This note records what changed when Ethermint moved from go-ethereum v1.16.9 to
v1.17.10, for reviewers and for downstream chains (e.g. Cronos).

| | Before | After |
|---|---|---|
| Upstream go-ethereum | v1.16.9 | v1.17.10 |
| `replace` target | `crypto-org-chain/go-ethereum@663dca6c618e` | `crypto-org-chain/go-ethereum@43022f1730ed` |
| Fork patch | "feat: support cronos precompiles" on v1.16.9 | "core/vm: support cronos precompiles" on v1.17.10 |

The EVM keeps running Osaka rules: Ethermint's `ChainConfig` has no
`amsterdam_time`, so none of the v1.17 Amsterdam features are active (see
[New EVM features](#new-evm-features-not-enabled)).

## Fork patch changes

The crypto-org-chain fork keeps the stateful precompile interface
(`Address()`, `Run(evm, contract, readonly)`, `DefaultPrecompiles`,
`SetPrecompiles`, `ValidatePrecompiles`), ported onto v1.17.10:

- **`Contract.Gas` is a `vm.GasBudget`.** `Contract.UseGas` and `RefundGas` are
  gone; precompiles that charge gas inside `Run` must use
  `contract.Gas.ChargeExecution`. This breaks Cronos precompiles that call
  `contract.UseGas`.
- **`readonly` follows the static context.** A plain `CALL` to a stateful
  precompile from inside a static frame now gets `readonly = true`; before, it
  always got `false`. This is state machine breaking for any transaction that
  relied on mutating state that way.
- **`ValidatePrecompiles` checks addresses.** It now rejects a precompile whose
  `Address()` differs from the address it is registered at.
- **Precompile result cache.** Upstream added an opt-in cache for pure
  precompiles (`vm.CacheablePrecompile`). Ethermint does not attach a cache, and
  stateful precompiles must not implement `CacheablePrecompile`.

## API breaking changes in Ethermint

go-ethereum v1.17 changed several types Ethermint exposes or implements:

- **`core.Message`**: `Value`, `GasPrice`, `GasFeeCap`, `GasTipCap` and
  `BlobGasFeeCap` are `*uint256.Int`. `SkipNonceChecks` and
  `SkipTransactionChecks` are unexported and set through
  `SkipNonceCheck()`, `SkipEOACheck()` and `SkipGasLimitCapCheck()`.
- **`keeper.EVMConfig.SkipTransactionChecks`** (new) replaces
  `core.Message.SkipTransactionChecks`, which can no longer be read. `EthCall`,
  `EstimateGas`, `TraceCall` and `CreateAccessList` set it, so they keep
  skipping the EIP-7825 per-transaction gas cap. `eth_simulateV1` always skips
  that cap, as before.
- **`keeper.GetEthIntrinsicGas(msg, rules)`**: the `isContractCreation`
  argument is removed; contract creation is derived from `msg.To == nil`.
- **`types.TransactionArgs.ToMessage` / `ToSimMessage`**: return an error when
  `value`, `gasPrice`, `maxFeePerGas` or `maxPriorityFeePerGas` exceeds 256
  bits.
- **`statedb.Transfer`** takes a `*params.Rules` argument (new
  `vm.TransferFunc`). After Amsterdam it would also emit the EIP-7708 transfer
  log.
- **`statedb.StateDB` / `statedb.HookedStateDB`** (`vm.StateDB`):
  - `SelfDestruct(addr)` no longer returns the previous balance.
  - `SelfDestruct6780` and `PointCache` are removed (the `trie/utils` package no
    longer exists).
  - `IsNewContract`, `Touch` and `SetTxContext` are added. `SetTxContext` is a
    no-op so that go-ethereum system calls cannot overwrite the keeper-owned
    `TxConfig`.
  - `Finalise(rules params.Rules)` replaces `Finalise(deleteEmptyObjects bool)`
    and always returns a nil block access list. Empty objects are deleted when
    `rules.IsEIP158` is set; state overrides pass empty rules, as go-ethereum
    does.
- **`core.ErrMaxInitCodeSizeExceeded`** moved to `vm.ErrMaxInitCodeSizeExceeded`.
- **`vm.EVM.Call` / `Create`** take and return a `vm.GasBudget`. The keeper
  passes `vm.NewGasBudget(gasLimit, 0)` and reads back
  `ExecutionGas + StateGas`.

## Behaviour changes with Osaka rules

- **Gas.** `core.IntrinsicGas` and `core.FloorDataGas` now take the fork rules
  and the sender, recipient and value. Before Amsterdam they compute the same
  values as v1.16. The EIP-8037 state-gas reservoir is always empty before
  Amsterdam, so the gas left after a top-level call is unchanged.
- **SELFDESTRUCT (EIP-6780).** The interpreter now moves the balance itself
  (through `SubBalance`/`AddBalance`) and checks `IsNewContract` before calling
  `SelfDestruct`. The resulting balances are the same, but the sequence of
  native bank operations, and therefore the emitted bank events, differs:
  - existing contract, beneficiary is itself: no balance operations (before: a
    `SubBalance` + `AddBalance` pair);
  - new contract, other beneficiary: `AddBalance` then `SubBalance` (before:
    the reverse);
  - new contract, beneficiary is itself: a single `SubBalance` burn (before:
    `SubBalance`, `AddBalance`, then a burn inside `SelfDestruct`).
- **`eth_simulateV1` requests (Prague).** `core.ProcessWithdrawalQueue` and
  `core.ProcessConsolidationQueue` now fail when the EIP-7002/EIP-7251 system
  contract has no code. Ethermint chains do not deploy them, so the simulator
  skips those calls when the contract is empty, which yields no requests as
  before.
- **Tracing.** `HookedStateDB.SelfDestruct` only reports a balance change when
  the account still holds a balance; the interpreter's own balance operations
  now emit the hooks.

These changes are expected to keep the app hash unchanged for existing blocks,
but the precompile `readonly` fix above is state machine breaking. Roll the
upgrade out as a coordinated chain upgrade, and replay historical blocks to
compare app hashes and receipts before release.

## New EVM features (not enabled)

v1.17.10 adds the **Amsterdam** fork. It only activates when `AmsterdamTime` is
set, which Ethermint's `ChainConfig` does not expose yet:

| EIP | Feature | Notes for Ethermint |
|---|---|---|
| EIP-8024 | `DUPN`, `SWAPN`, `EXCHANGE` opcodes | Works as is |
| EIP-7843 | `SLOTNUM` opcode | No beacon slot on Cosmos; needs a decision on `BlockContext.SlotNum` |
| EIP-8037 / EIP-8038 | Two-dimensional gas (state-gas reservoir), state-access repricing, `MaxTxTotalGas` | Needs rework of refunds, the per-tx gas cap and estimation |
| EIP-2780 | Lower intrinsic transaction gas | Handled by `core.IntrinsicGas` |
| EIP-7976 / EIP-7981 | Calldata floor 64/64, access-list data cost | Handled by `core.FloorDataGas` / `core.IntrinsicGas` |
| EIP-7708 | ETH transfer and burn logs | Emitted by `statedb.Transfer` and the interpreter |
| EIP-8246 | No SELFDESTRUCT burn | Needs a matching change in `StateDB.SelfDestruct` |
| EIP-7928 | Block-level access lists | Not supported (`Finalise` returns nil) |
| EIP-7997 | Deterministic CREATE2 factory predeploy | Needs an upgrade handler |
| EIP-8282 | Builder execution requests | Not applicable |

Enabling Amsterdam requires a new `amsterdam_time` chain config field, an
upgrade handler and the work listed above.

## JSON-RPC

New in go-ethereum v1.17: `eth_capabilities`, `eth_getStorageValues`,
`eth_baseFee`, `debug_clearTxpool`, `debug_replayBadBlock`. Ethermint already
serves `eth_getStorageValues` and `eth_baseFee`; the missing methods are
tracked in a separate PR.

## Dependencies

Besides go-ethereum, minimum version selection bumps several indirect modules,
including `google.golang.org/grpc` v1.83.1, `google.golang.org/protobuf`
v1.36.12, `golang.org/x/crypto` v0.55.0, `golang.org/x/net` v0.58.0, the
OpenTelemetry modules (v1.46.0), `github.com/crate-crypto/go-eth-kzg` v1.5.0 and
`github.com/ethereum/c-kzg-4844/v2` v2.1.8. `github.com/ethereum/go-verkle` and
`github.com/crate-crypto/go-ipa` are dropped. `gomod2nix.toml` is regenerated.

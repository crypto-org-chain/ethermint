# Amsterdam support

Ethermint supports the Ethereum Amsterdam fork on a best-effort basis. We do not
commit to matching Ethereum's design exactly; this document lists what works,
where Ethermint deliberately or temporarily differs, and what is still missing.

Reference implementation: go-ethereum v1.17 (crypto-org-chain fork). At that
version no Ethereum network schedules Amsterdam and the EIPs are still subject
to change, so this document must be revisited on every go-ethereum bump.

## Enabling

Amsterdam is off by default. It is activated by setting
`params.chain_config.amsterdam_time` (a unix timestamp, `0` = from genesis)
through a governance `MsgUpdateParams`. No store migration is needed: the field
is new and defaults to `nil` (not activated).

> Do not enable it on a production network before the items marked
> **blocker** below are implemented.

## Status by EIP

| EIP | Description | Status |
| --- | --- | --- |
| EIP-7708 | ETH transfers emit a `Transfer` log | Done (`statedb.Transfer`, SELFDESTRUCT log comes from geth) |
| EIP-8246 | SELFDESTRUCT no longer burns the balance | Done (`StateDB.SelfDestruct`, `StateDB.Commit`, tracing hooks) |
| EIP-8024 | `SWAPN`, `DUPN`, `EXCHANGE` opcodes | Done (geth interpreter) |
| EIP-8038 | State-access gas repricing | Done (geth interpreter) |
| EIP-7976 / EIP-7981 | Calldata floor and access-list repricing | Done (`core.IntrinsicGas` / `core.FloorDataGas`, used by the ante handler and the keeper) |
| EIP-3860 (Amsterdam limits) | Larger max code size (64 KiB) and init code size (128 KiB) | Done (geth interpreter, `vm.CheckMaxInitCodeSize` in the keeper and `eth_simulateV1`) |
| EIP-8037 | State gas metering | Partial, see [State gas](#state-gas-eip-8037) |
| EIP-2780 | Intrinsic gas repricing with runtime state charges | Partial, see [Runtime state charges](#runtime-state-charges-eip-2780--eip-8037) |
| EIP-7843 | `SLOTNUM` opcode | Partial: opcode exists but always returns `0` |
| EIP-7928 | Block-level access lists | Not implemented |

## Differences with Ethereum

### State gas (EIP-8037)

Ethereum splits a transaction's gas into an execution budget, capped at
`MaxTxGas` (16,777,216), and a state-gas reservoir holding the rest. The tx gas
limit is capped by `MaxTxTotalGas` (2^32 - 1).

Ethermint keeps a **single gas pool**: the reservoir is always empty, and
geth's EVM charges state gas from the execution gas instead ("spill-over"),
refunding it the same way on revert. `CostPerStateByte` (1530) is set in the
EVM block context, so in-EVM state charges (SSTORE slot creation, CREATE/CALL
account creation, code deposit) are priced as on Ethereum.

Consequences:

- The per-tx gas cap stays at the EIP-7825 value `MaxTxGas` for the whole tx
  (`keeper.CheckMaxTxGas`, used by the ante handler and `ApplyMessageWithConfig`).
  Transactions needing more than 16,777,216 gas in total, which Ethereum accepts
  as long as the excess is state gas, are rejected. This is stricter than
  Ethereum, not a safety issue.
- Receipts, `eth_estimateGas` and tracers report one gas figure; there is no
  separate state-gas dimension.
- Ethermint's own gas rules still apply on top (`MinGasMultiplier` minimum gas
  used, refund cap).

### Runtime state charges (EIP-2780 / EIP-8037)

Under Amsterdam, `core.IntrinsicGas` charges less up-front (the EIP-2780 base
cost) because geth's state transition charges the state-dependent part at
runtime. Ethermint reimplements the state transition in
`Keeper.ApplyMessageWithConfig` and does **not** yet port those runtime
charges, so the following are **underpriced** (**blocker**):

- **Top-level contract creation**: geth charges
  `AccountCreationSize * CostPerStateByte` (183,600 gas) before running the init
  code and refunds it if creation fails. Example from
  `TestAmsterdamBestEffort`: deploying 32 bytes of code costs 59,439 gas before
  Amsterdam and 73,045 gas under Ethermint's Amsterdam, where Ethereum would
  charge about 183,600 more.
- **Value transfer to an empty account** at the top level
  (`chargeCallRecipientEIP2780`), refunded if the call fails.
- **EIP-7702 authorizations**: the per-authorization state costs charged at
  runtime (`applyAuthorization`), now that the intrinsic cost per authorization
  is the lower `ExecutionPerAuthBaseCost`.
- **Top-frame halt**: geth burns the leftover execution gas when the top frame
  halts (`DrainExecution`) after refunding state charges.

Nested CREATE/CALL frames are not affected: their state charges happen inside
geth's EVM.

### Block gas accounting

Ethereum checks the block gas limit against
`max(total execution gas, total state gas)` (`GasPool.ChargeGasAmsterdam`).
Ethermint keeps its one-dimensional block gas meter: the sum of tx gas limits
in the ante handler and the sum of gas used at execution. With the single gas
pool above this is stricter than Ethereum.

### SELFDESTRUCT (EIP-8246)

Behaviour matches Ethereum for the EVM denom: the balance is kept, and an
account left with a balance becomes a balance-only account (nonce 0, no code,
storage cleared) instead of being deleted; an account with no balance is still
deleted. Pre-Amsterdam, Ethermint still burns the balance as before.

Difference: Ethermint balances live in the bank module, and only the EVM denom
is considered. Other denoms held by a self-destructed address are left
untouched both before and after Amsterdam.

### Block-level access lists (EIP-7928)

Not implemented. `StateDB.Finalise` returns `nil` and no access list is built or
exposed. CometBFT block headers have no field for it, so this would only matter
for RPC compatibility.

### SLOTNUM (EIP-7843)

Ethermint does not set `vm.BlockContext.SlotNum`, so `SLOTNUM` returns `0`.
CometBFT has no beacon slot; a possible mapping is the block height.

### Tracing

Geth's state transition emits gas-change events for the two gas dimensions
(`GasChangeTxIntrinsicGas`, `GasChangeAccountCreation`,
`GasChangeRefundAccountCreation`, ...) at the top level. Ethermint emits its own
single-dimension events; in-EVM events come from geth unchanged.

## Remaining work

In suggested order:

1. **Runtime state charges** (blocker): port geth's Amsterdam top-level charges
   for contract creation, value transfers to empty accounts and EIP-7702
   authorizations, with their refunds on failure and the halt drain, into
   `Keeper.ApplyMessageWithConfig` and the `eth_simulateV1` path
   (`x/evm/keeper/simulate.go`).
2. **Reference tests**: run the integration tests with Amsterdam enabled against
   the go-ethereum reference node and compare gas used, receipts and state.
3. **SLOTNUM**: decide on a value (e.g. block height) and set `SlotNum` in the
   block context of `NewEVM` and the simulator.
4. **Two-dimensional gas** (optional): state-gas reservoir, `MaxTxTotalGas`
   cap, `max(execution, state)` block accounting, and the matching ante handler,
   RPC and tracer changes. Only needed if we want to accept the larger
   state-heavy transactions Ethereum allows.
5. **Block-level access lists** (optional): only for RPC compatibility.

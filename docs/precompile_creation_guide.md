# How to Create a New Precompile in Ethermint

This guide walks through adding a **custom precompile**: a native contract at a fixed address that runs Go code instead of EVM bytecode. Ethermint merges go-ethereum's default precompiles with your custom ones and gives them access to Cosmos `sdk.Context` when needed.

Patterns below are based on **Ethermint** and **Cronos** ([crypto-org-chain/cronos](https://github.com/crypto-org-chain/cronos) `x/cronos/keeper/precompiles`).

---

## High-level flow: Cronos / Ethermint → geth

How a call to a custom precompile moves from the Cosmos app down into go-ethereum:

1. **App (e.g. Cronos)** – Builds the EVM keeper with `evmkeeper.NewKeeper(..., customContractFns, ...)`. Each `CustomContractFn` is a factory that, given `(sdk.Context, ethparams.Rules)`, returns a `vm.PrecompiledContract`. When an Ethereum tx is delivered (via the EVM module handler or RPC), the keeper is used to run it.

2. **ApplyMessage** – For each tx, the keeper’s `ApplyMessage` / `ApplyMessageWithConfig` is invoked with a `core.Message` (sender, to, data, value, gas, etc.). It obtains the Ethermint **StateDB** (which implements Cosmos-backed storage and exposes `Context()` and `ExecuteNativeAction` for precompiles) and builds an EVM instance.

3. **NewEVM** – For that message, the keeper builds the precompile set:
   - Start with **geth default precompiles**: `vm.DefaultPrecompiles(cfg.Rules)` (ecrecover, sha256, etc.).
   - For each `customContractFns`, call `fn(ctx, cfg.Rules)` and add the returned contract to a map by `contract.Address()`.
   - Create the geth EVM with `vm.NewEVM(blockCtx, stateDB, chainConfig, vmConfig)`.
   - Call **`evm.SetPrecompiles(contracts)`** so this EVM instance uses both default and custom precompiles.

4. **StateDB.Prepare** – Before execution, the keeper calls `stateDB.Prepare(rules, msg.From, coinbase, msg.To, vm.ActivePrecompiles(rules), msg.AccessList)`. That adds default precompile addresses to the access list (EIP-2929) and resets transient storage.

5. **EVM execution (geth)** – The keeper calls either **`evm.Create(sender, data, gas, value)`** (contract creation) or **`evm.Call(sender, to, data, gas, value)`** (normal call). Execution runs inside go-ethereum’s EVM and interpreter.

6. **Precompile dispatch (geth)** – When the interpreter encounters a **CALL** (or **STATICCALL**) to an address that is in the precompiles map, geth:
   - Looks up the contract with that address.
   - Charges **`RequiredGas(input)`**.
   - Calls **`Run`** (with either `(input, contract)` or `(evm, contract, readonly)` depending on the fork).
   - Your precompile’s `Run` uses the same **StateDB** (cast to `ExtStateDB` in Cronos) to read Cosmos state via `Context()` or change it via `ExecuteNativeAction`.

So: **Cronos/Ethermint** owns keeper creation (with custom factories), StateDB, and each per-tx EVM construction; **geth** owns the interpreter and the moment it dispatches to a precompile’s `Run`. Custom precompiles are “injected” into that geth EVM via `SetPrecompiles` before every execution.

---

## 1. Implement `vm.PrecompiledContract`

Your type must satisfy the interface used by the EVM (from `github.com/ethereum/go-ethereum/core/vm`). The **exact `Run` signature depends on your go-ethereum (or fork)**:

| Environment | `Run` signature |
|-------------|-----------------|
| Vanilla go-ethereum | `Run(input []byte, contract *vm.Contract) ([]byte, error)` |
| Cronos / Ethermint fork (e.g. crypto-org-chain/go-ethereum) | `Run(evm *vm.EVM, contract *vm.Contract, readonly bool) ([]byte, error)` |

Always required:

- **`Address() common.Address`** – Contract address (used by Ethermint's keeper when registering).
- **`RequiredGas(input []byte) uint64`** – Gas cost for the given input (charged before `Run`).

**Example: minimal stateless precompile (vanilla `Run`)**

```go
package myprecompile

import (
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/vm"
)

// Use an address that does not conflict with default precompiles (1–9).
// Cronos uses 100 (0x64), 101 (0x65), 102 (0x66) for bank, relayer, ICA.
var myPrecompileAddress = common.BytesToAddress([]byte{100})

type MyPrecompile struct{}

func (MyPrecompile) Address() common.Address {
	return myPrecompileAddress
}

func (MyPrecompile) RequiredGas(input []byte) uint64 {
	return 1000
}

func (MyPrecompile) Run(input []byte, contract *vm.Contract) ([]byte, error) {
	return input, nil
}
```

**Example: extended `Run` (evm + readonly)** – use when your chain uses a fork that passes `evm` and `readonly`:

```go
func (p MyPrecompile) Run(evm *vm.EVM, contract *vm.Contract, readonly bool) ([]byte, error) {
	if readonly {
		return nil, errors.New("this method is not readonly")
	}
	// Use evm.StateDB, contract.Caller(), contract.Input, etc.
	return contract.Input, nil
}
```

Use an address that does **not** conflict with default precompiles (e.g. 1–9). Custom precompiles often use `common.BytesToAddress([]byte{100})`, `[]byte{101}`, etc., or the `0x0100` range.

---

## 2. Use Cosmos context and native state (stateful precompiles)

If your precompile reads or writes Cosmos state (keepers, module stores, events), it must use the StateDB implementation from Ethermint.

- **`Context() sdk.Context`** – Use for **read-only** Cosmos access (queries, block height, KVStore reads).
- **`ExecuteNativeAction(contract, converter, action)`** – Use for **state-changing** logic; state is reverted if the action errors or the EVM call reverts. Pass `statedb.EventConverter` or `nil`.

Define an extended interface in your precompile package (Cronos pattern) and cast in `Run`:

```go
type ExtStateDB interface {
	vm.StateDB
	ExecuteNativeAction(contract common.Address, converter statedb.EventConverter, action func(ctx sdk.Context) error) error
	Context() sdk.Context
}
```

In `Run`: use `Context()` for reads and `ExecuteNativeAction` for writes (see Cronos `x/cronos/keeper/precompiles/bank.go` for a full example).

**Example (extended `Run`):** cast StateDB to `ExtStateDB`; use `Context()` for reads and `ExecuteNativeAction` for writes. See Cronos [bank.go](https://github.com/crypto-org-chain/cronos/blob/main/x/cronos/keeper/precompiles/bank.go) for a full stateful precompile.

```go
func (p MyPrecompile) Run(evm *vm.EVM, contract *vm.Contract, readonly bool) ([]byte, error) {
	stateDB := evm.StateDB.(ExtStateDB)
	ctx := stateDB.Context()
	return contract.Input, nil
}
```

Use these APIs only when the EVM is backed by Ethermint's StateDB.

---

## 3. Create a `CustomContractFn` (factory)

Ethermint’s keeper expects a **slice of functions** that produce a precompile for each (context, rules) pair. Each function has the type:

```go
type CustomContractFn func(sdk.Context, ethparams.Rules) vm.PrecompiledContract
```

Create a factory that returns your precompile:

```go
package myprecompile

import (
	ethparams "github.com/ethereum/go-ethereum/params"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/ethereum/go-ethereum/core/vm"
	evmkeeper "github.com/evmos/ethermint/x/evm/keeper"
)

func NewMyPrecompileFn() evmkeeper.CustomContractFn {
	return func(ctx sdk.Context, rules ethparams.Rules) vm.PrecompiledContract {
		// You can use ctx/rules to enable/disable or configure the precompile per block.
		return MyPrecompile{}
	}
}
```

If you need to inject keepers or config, capture them in the closure:

```go
// types = e.g. github.com/evmos/ethermint/x/evm/types
func NewMyPrecompileFn(accountKeeper types.AccountKeeper) evmkeeper.CustomContractFn {
	return func(ctx sdk.Context, rules ethparams.Rules) vm.PrecompiledContract {
		return MyPrecompile{accountKeeper: accountKeeper}
	}
}
```

---

## 3a. ABI-based method dispatch and gas (recommended for multi-method precompiles)

For precompiles that expose multiple Solidity-style methods (e.g. `balanceOf`, `transfer`, `mint`), use **ABI method IDs** and a **gas table** (Cronos pattern):

1. **Load the ABI** – Embed `abi.json` or use generated bindings (e.g. `bank.BankModuleMetaData.ABI`).
2. **In `init()`** – Parse the ABI and build `map[[4]byte]uint64` from method ID to required gas.
3. **`RequiredGas(input []byte)`** – If `len(input) < 4` return a base cost (or 0). Otherwise copy `input[:4]` as method ID, lookup gas from the map, and add a base cost (e.g. `len(input) * kvGasConfig.WriteCostPerByte`).
4. **In `Run`** – Get method with `abi.MethodById(contract.Input[:4])`, unpack args with `method.Inputs.Unpack(contract.Input[4:])`, switch on `method.Name`, then pack output with `method.Outputs.Pack(...)`.

Example (Cronos-style):

```go
var myABI abi.ABI
var gasByMethod = map[[4]byte]uint64{}

func init() {
	_ = myABI.UnmarshalJSON([]byte(metadata.ABI))
	for name := range myABI.Methods {
		var id [4]byte
		copy(id[:], myABI.Methods[name].ID[:4])
		gasByMethod[id] = 10000 // or per-method values
	}
}

func (p *MyPrecompile) RequiredGas(input []byte) uint64 {
	if len(input) < 4 {
		return baseCost
	}
	var id [4]byte
	copy(id[:], input[:4])
	return gasByMethod[id] + baseCost
}

func (p *MyPrecompile) Run(evm *vm.EVM, contract *vm.Contract, readonly bool) ([]byte, error) {
	method, err := myABI.MethodById(contract.Input[:4])
	if err != nil { return nil, err }
	args, err := method.Inputs.Unpack(contract.Input[4:])
	if err != nil { return nil, err }
	switch method.Name {
	case "balanceOf":
		return method.Outputs.Pack(p.balanceOf(evm, args))
	default:
		return nil, errors.New("unknown method")
	}
}
```

For **read-only vs state-changing**: if your `Run` has a `readonly bool`, reject state-changing methods when `readonly` is true (e.g. `if readonly && isStateChanging(method) { return nil, vm.ErrWriteProtection }`).

---

## 3b. Caller authentication (Cosmos msg-style precompiles)

When the precompile input is a **serialized Cosmos message** (e.g. IBC relayer, ICA), the caller must match the message signer. Cronos uses a generic **Executor** that unmarshals the input as a protobuf message, gets the signer via `codec.GetMsgV1Signers(msg)`, and requires `contract.Caller() == common.BytesToAddress(signers[0])` before running the action inside `ExecuteNativeAction`. See Cronos `precompiles/utils.go` (`exec`) and `relayer.go` / `ica.go`.

---

## 4. Register the precompile in the app

When constructing the EVM keeper, pass your factory (or factories) as the **custom precompiles** slice. In `evmd/app.go` the keeper is created with `nil` for that slice; replace it with your list:

**Before (snippet from `evmd/app.go`):**

```go
app.EvmKeeper = evmkeeper.NewKeeper(
	appCodec,
	keys[evmtypes.StoreKey], okeys[evmtypes.ObjectStoreKey], authtypes.NewModuleAddress(govtypes.ModuleName),
	app.AccountKeeper, app.BankKeeper, app.StakingKeeper, app.FeeMarketKeeper,
	tracer,
	evmSs,
	nil,  // <-- custom precompiles
	cast.ToUint64(appOpts.Get(server.FlagQueryGasLimit)),
)
```

**After:**

```go
import (
	evmkeeper "github.com/evmos/ethermint/x/evm/keeper"
	myprecompile "github.com/evmos/ethermint/x/myprecompile" // or your module path
)

// ...

app.EvmKeeper = evmkeeper.NewKeeper(
	appCodec,
	keys[evmtypes.StoreKey], okeys[evmtypes.ObjectStoreKey], authtypes.NewModuleAddress(govtypes.ModuleName),
	app.AccountKeeper, app.BankKeeper, app.StakingKeeper, app.FeeMarketKeeper,
	tracer,
	evmSs,
	[]evmkeeper.CustomContractFn{
		myprecompile.NewMyPrecompileFn(),
		// add more custom precompiles here
	},
	cast.ToUint64(appOpts.Get(server.FlagQueryGasLimit)),
)
```

**Cronos example** – multiple precompiles with keepers and context:

```go
[]evmkeeper.CustomContractFn{
	func(_ sdk.Context, rules ethparams.Rules) vm.PrecompiledContract {
		return cronosprecompiles.NewRelayerContract(app.IBCKeeper, appCodec, rules, app.Logger())
	},
	func(ctx sdk.Context, rules ethparams.Rules) vm.PrecompiledContract {
		return cronosprecompiles.NewIcaContract(ctx, app.ICAControllerKeeper, &app.CronosKeeper, appCodec, gasConfig)
	},
},
```

After this, every new EVM instance will get both `vm.DefaultPrecompiles(cfg.Rules)` and your custom precompiles, and active precompile addresses are passed to `StateDB.Prepare` for access-list handling.

---

## Example: Cronos bank precompile

The Cronos **bank precompile** exposes an ERC-20-style interface for **native tokens** at a fixed address. Each EVM contract address that calls the precompile is treated as the “token” address: the Cosmos denom is `evm/<caller_hex>`. This section summarizes how it is implemented and how it would be integrated (Cronos currently registers Relayer and ICA in `customContractFns`; the bank precompile is implemented in the same package but can be added the same way).

### Purpose and methods

- **Address**: `0x64` (`common.BytesToAddress([]byte{100})`).
- **Methods** (Solidity ABI): `mint(recipient, amount)`, `burn(recipient, amount)`, `balanceOf(token, account)`, `transfer(sender, recipient, amount)`.
- **Denom**: `EVMDenom(contract.Caller())` = `"evm/" + caller.Hex()` — the caller of the precompile is the “token” (e.g. a deployed contract that mints/burns its own denom).

### ABI and gas table (`init`)

The ABI comes from generated bindings (`bank.BankModuleMetaData.ABI` in `x/cronos/events/bindings/cosmos/precompile/bank`). In `init()` the package parses this ABI and fills a map from 4-byte method ID to gas:

- `mint` / `burn`: 200_000
- `balanceOf`: 10_000
- `transfer`: 150_000

### Contract struct and factory

```go
type BankContract struct {
	bankKeeper  types.BankKeeper
	cdc         codec.Codec
	kvGasConfig storetypes.GasConfig
}

func NewBankContract(bankKeeper types.BankKeeper, cdc codec.Codec, kvGasConfig storetypes.GasConfig) vm.PrecompiledContract {
	return &BankContract{bankKeeper, cdc, kvGasConfig}
}
```

`Address()` returns the fixed address; `RequiredGas(input)` uses `input[:4]` as method ID and returns the mapped gas plus a base cost (`len(input) * kvGasConfig.WriteCostPerByte`).

### Run: ExtStateDB and method dispatch

1. **Cast StateDB** – `stateDB := evm.StateDB.(ExtStateDB)` (from `precompiles/interface.go`).
2. **Dispatch by method** – `method, _ := bankABI.MethodById(contract.Input[:4])`, then `method.Inputs.Unpack(contract.Input[4:])`, and a `switch method.Name`.
3. **Read-only (`balanceOf`)** – Uses `stateDB.Context()` only:  
   `bc.bankKeeper.GetBalance(stateDB.Context(), account, EVMDenom(token))`, then `method.Outputs.Pack(balance)`.
4. **State-changing (`mint`, `burn`, `transfer`)** – Returns an error if `readonly` is true. Then runs all Cosmos logic inside **`stateDB.ExecuteNativeAction(precompileAddr, nil, func(ctx sdk.Context) error { ... })`**:
   - **mint**: `IsSendEnabledCoins` → `MintCoins(module, ...)` → `SendCoinsFromModuleToAccount`.
   - **burn**: `SendCoinsFromAccountToModule` → `BurnCoins(module, ...)`.
   - **transfer**: `IsSendEnabledCoins` → `SendCoins(from, to, coins)`.
   - Uses `EVMDenom(contract.Caller())` as the coin denom and checks `bankKeeper.BlockedAddr` for recipients.
5. **Return** – `method.Outputs.Pack(true)` for mutating methods, or the balance for `balanceOf`.

So: reads go through `Context()`; writes go through `ExecuteNativeAction` so that any failure (or EVM revert) reverts Cosmos state as well.

### Integration in the app

To register the bank precompile, pass a factory in `customContractFns` that injects the bank keeper, codec, and gas config (e.g. from `storetypes.TransientGasConfig()` or a module param):

```go
gasConfig := storetypes.TransientGasConfig() // or your app's config

[]evmkeeper.CustomContractFn{
	// ... other precompiles (e.g. Relayer, ICA) ...
	func(_ sdk.Context, rules ethparams.Rules) vm.PrecompiledContract {
		return cronosprecompiles.NewBankContract(app.BankKeeper, appCodec, gasConfig)
	},
},
```

The EVM keeper then builds each EVM instance with default precompiles plus this bank precompile; calls to `0x64` with ABI-encoded `mint`, `burn`, `balanceOf`, or `transfer` are handled by the bank precompile’s `Run` and the Cosmos x/bank module.

---

## 5. Avoid address conflicts

- **Preinstalls**: If you use `AddPreinstalls` (contracts deployed at fixed addresses), the keeper checks that those addresses do **not** match any precompile address (including those from `customContractFns`). Pick precompile addresses that won’t clash with preinstalls.
- **Default precompiles**: Don’t reuse addresses 1–9 (and any others your go-ethereum fork defines). Use a distinct range (e.g. `0x0100`, `0x0101`, …) for custom precompiles.

---

## 6. Summary checklist

1. **Interface** – Implement `Address() common.Address`, `RequiredGas(input []byte) uint64`, and `Run` with the signature required by your go-ethereum (or fork): either `(input, contract)` or `(evm, contract, readonly)`.
2. **Address** – Choose a non-conflicting address (e.g. `common.BytesToAddress([]byte{100})`; avoid 1–9).
3. **Stateful** – Use `ExtStateDB`: `Context()` for reads, `ExecuteNativeAction` for writes in `Run`.
4. **Multi-method** – Use ABI method IDs and gas-by-method map; dispatch in `Run` with `MethodById`, `Unpack`, `Pack`.
5. **Factory and registration** – Add a `CustomContractFn` and pass it in `customContractFns` to `evmkeeper.NewKeeper` in `app.go`.

Contracts call the precompile via `CALL`/`STATICCALL` to your address with ABI-encoded calldata; the EVM dispatches to your `Run` and charges `RequiredGas`.

---

## References

- **Ethermint** – `x/evm/keeper/state_transition.go` (precompiles merged and set on EVM), `x/evm/statedb/statedb.go` (`ExecuteNativeAction`, `Context()`).
- **Cronos** – [x/cronos/keeper/precompiles](https://github.com/crypto-org-chain/cronos/tree/main/x/cronos/keeper/precompiles): `interface.go` (ExtStateDB), `bank.go` (stateful ABI precompile), `relayer.go` / `ica.go` (Cosmos msg + Executor), `utils.go` (caller auth).

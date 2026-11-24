# Upgrade Reference

This document covers upgrading an app built on Ethermint from v0.24.x to
v0.25.x: the changes to the evm and feemarket modules and to Ethermint's own
APIs. See the [CHANGELOG](CHANGELOG.md) for the full list of changes.

For the cosmos-sdk v0.55 changes (CometBFT, `x/params` and `SIGN_MODE_TEXTUAL`
removal, staking key rotation wiring, module migrations), follow the SDK fork's
[UPGRADING.md](https://github.com/crypto-org-chain/cosmos-sdk/blob/release/v0.55.x/UPGRADING.md).
For the EVM, precompile and JSON-RPC changes from go-ethereum v1.17, see
[`docs/upgrade/go-ethereum-v1.17.md`](docs/upgrade/go-ethereum-v1.17.md).

Neither module changes its store layout: evm stays at consensus version 8 and
feemarket at 4, so neither registers a new migration.

## Table of Contents

- [Breaking Changes](#breaking-changes)
    - [Dependencies](#dependencies)
    - [evm and feemarket Keeper Wiring](#evm-and-feemarket-keeper-wiring)
    - [Removed and Changed APIs](#removed-and-changed-apis)
    - [Staking Keeper Interface](#staking-keeper-interface)
- [Upgrade Handler](#upgrade-handler)
- [Behavior Changes Affecting Dapps and Operators](#behavior-changes-affecting-dapps-and-operators)

## Breaking Changes

### Dependencies

**Required action:** your app must use the same cosmos-sdk and CometBFT forks
as Ethermint. Go does not apply a dependency's `replace` lines, so add these to
your app's `go.mod`:

```go
replace (
	github.com/cosmos/cosmos-sdk => github.com/crypto-org-chain/cosmos-sdk v0.50.6-0.20261118030422-b495f1d247db
	github.com/cometbft/cometbft => github.com/crypto-org-chain/cometbft v0.0.0-20261105212524-34b0a74b4248
)
```

Ethermint v0.25.x also requires Go 1.26.11 or later.

### evm and feemarket Keeper Wiring

The SDK removed `x/params`, so the evm and feemarket keepers take a read-only,
decode-only `legacyparams.Subspace` instead of an SDK subspace.

**Required action:** remove the subspace argument of the evm and feemarket
`NewAppModule`, and pass a `legacyparams.Subspace` to their keepers. Pick the
wiring for your chain:

- If your chain ever ran evm before consensus version 5 or feemarket before
  consensus version 4, keep mounting the `params` store and pass real
  subspaces, so queries at those heights get that height's params. Evm version
  4 needs it too: for a false bool, an empty denom or empty extra EIPs it falls
  back to the `x/params` value.
- Otherwise pass `legacyparams.Subspace{}`, stop mounting the `params` store,
  and list `params` in the upgrade's `StoreUpgrades.Deleted` (see below).

Before:

```go
feeMarketSs := app.GetSubspace(feemarkettypes.ModuleName)
app.FeeMarketKeeper = feemarketkeeper.NewKeeper(appCodec, authority, keys[feemarkettypes.StoreKey], feeMarketSs)

evmSs := app.GetSubspace(evmtypes.ModuleName)
app.EvmKeeper = evmkeeper.NewKeeper(
	appCodec, keys[evmtypes.StoreKey], okeys[evmtypes.ObjectStoreKey], authority,
	app.AccountKeeper, app.BankKeeper, app.StakingKeeper, app.FeeMarketKeeper,
	tracer, evmSs, nil, queryGasLimit,
)

feemarket.NewAppModule(app.FeeMarketKeeper, feeMarketSs)
evm.NewAppModule(app.EvmKeeper, app.AccountKeeper, evmSs)
```

After, keeping the legacy params:

```go
// keep "params" in the store keys passed to storetypes.NewKVStoreKeys
paramsKey := keys["params"]

app.FeeMarketKeeper = feemarketkeeper.NewKeeper(
	appCodec, authority, keys[feemarkettypes.StoreKey],
	legacyparams.NewSubspace(paramsKey, feemarkettypes.ModuleName),
)

app.EvmKeeper = evmkeeper.NewKeeper(
	appCodec, keys[evmtypes.StoreKey], okeys[evmtypes.ObjectStoreKey], authority,
	app.AccountKeeper, app.BankKeeper, app.StakingKeeper, app.FeeMarketKeeper,
	tracer, legacyparams.NewSubspace(paramsKey, evmtypes.ModuleName), nil, queryGasLimit,
)

feemarket.NewAppModule(app.FeeMarketKeeper)
evm.NewAppModule(app.EvmKeeper, app.AccountKeeper)
```

When you drop the `params` store, the SDK store ignores the
`StoreUpgrades.Deleted` entry, but memiavl (cronos-store) needs it: without it
the node fails to start with `memiavl tree membership mismatch`.

```go
if upgradeInfo.Name == "sdk55" && !app.UpgradeKeeper.IsSkipHeight(upgradeInfo.Height) {
	app.SetStoreLoader(upgradetypes.UpgradeStoreLoader(upgradeInfo.Height, &storetypes.StoreUpgrades{
		Deleted: []string{"params"},
	}))
}
```

Also:

- An app that builds its interface registry without
  `encoding/codec.RegisterInterfaces` must call `legacyparams.RegisterInterfaces`,
  so gov queries and `export` still decode historical `ParameterChangeProposal`s.
- Remove any copy of the `cosmos.params.v1beta1` protos from your app.
  gogoproto keeps the first registration of a type name, so a second copy gets
  an empty type URL.

### Removed and Changed APIs

**Required action:** run evm and feemarket at consensus version 4 or later on
v0.24.x before upgrading. The 3→4 migrations (evm `migrations/v4.MigrateStore`
and the feemarket `migrations/v4` package) are gone.

Then replace these in your code:

| Before | After |
|---|---|
| evm `NewMigrator(keeper, subspace)` | `NewMigrator(keeper)` |
| feemarket `keeper.Migrator` | Delete it; feemarket has no migrations left. |
| evm and feemarket `types.Subspace` | `legacyparams.Subspace` |
| evm and feemarket `types.LegacyParams` | Delete it; read params with `Keeper.GetParams`. |
| evm `Keeper.GetLegacyParams` | `Keeper.GetParams`, which reads `x/params` at heights before the params moved. |
| evm `types.ValidateChainConfig(cfg)` | `cfg.Validate()` |
| evm `types.ValidateBool` | Delete the call; a typed `bool` needs no check. |
| evm `types.ValidateEVMDenom(interface{})` | `types.ValidateEVMDenom(string)` |
| evm `types.ValidateInt64Overflow(interface{})` | `types.ValidateInt64Overflow(uint64)` |
| feemarket `types.ParamKeyTable`, `Params.ParamSetPairs`, `ParamStoreKey*` | Delete them; set params with `MsgUpdateParams` or `Keeper.SetParams`. |
| evmd `GetSubspace`, `GetTKey` | Delete them; they only served `x/params`. |

### Staking Keeper Interface

**Required action:** if you pass your own `StakingKeeper` to the evm keeper,
implement `GetRotationLockedConsAddr`; the SDK staking keeper already has it.
The evm keeper uses it to resolve the coinbase of a proposer whose rotated
consensus key staking has not indexed yet.

## Upgrade Handler

See the `sdk55` handler in [`evmd/upgrades.go`](evmd/upgrades.go). Besides the
SDK steps, it does two things an Ethermint app needs:

- `sdk55.EnsureModuleAccount` creates the `key_rotation_fee_pool` module
  account, converting any plain account already at its address. Anyone can
  create that plain account by sending it funds, and it makes every key
  rotation fail.
- It calls the ibc-go send-sequence migration, which ibc-go v11.2.1 does not
  register itself. Any chain with IBC channels needs it:

  ```go
  import channelv11 "github.com/cosmos/ibc-go/v11/modules/core/04-channel/migrations/v11"

  err := channelv11.MigrateStore(
  	sdk.UnwrapSDKContext(ctx), runtime.NewKVStoreService(keys[ibcexported.StoreKey]), appCodec, app.IBCKeeper,
  )
  ```

  Without it, every send on an existing channel fails with
  `ErrSequenceSendNotFound` and `export` panics. Run it exactly once: a second
  run resets every send sequence to 0. It covers channels only; copy IBC v2
  send sequences (`nextSequenceSend/<client>` to `nextSequenceSend//<client>`)
  yourself if your chain has sent IBC v2 packets.

## Behavior Changes Affecting Dapps and Operators

- EVM queries at heights before evm consensus version 5 or feemarket consensus
  version 4 return wrong params with no error unless the legacy subspaces are
  wired: heights before version 4 get an empty chain config (Frontier rules) and
  no base fee, and evm version 4 heights get false, an empty denom or no extra
  EIPs where the chain used the `x/params` value.
- `eth_sign`, `eth_signTypedData` and `MsgEthereumTx.Sign` use
  `SIGN_MODE_DIRECT`, so Ledger keys fail with `ErrInvalidSignMode`.

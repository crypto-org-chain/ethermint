# Upgrade Reference

This document covers upgrading an app built on Ethermint from v0.24.x to
v0.25.x. Ethermint v0.25.x builds against cosmos-sdk v0.55 (fork
`release/v0.55.x`), CometBFT v0.40 (fork `v0.40.x`) and ibc-go v11.2.1-rc1.
See the [CHANGELOG](CHANGELOG.md) for the full list of changes.

The main changes are:

- `x/params` is gone. The evm and feemarket modules read legacy params through
  a read-only `legacyparams.Subspace`.
- `SIGN_MODE_TEXTUAL` is gone.
- The SDK adds consensus key rotation and ML-DSA-65 keys.
- go-ethereum v1.17: see [`docs/upgrade/go-ethereum-v1.17.md`](docs/upgrade/go-ethereum-v1.17.md)
  for the EVM, precompile and JSON-RPC changes.

## Table of Contents

- [Breaking Changes](#breaking-changes)
    - [Dependencies](#dependencies)
    - [x/params Removal](#xparams-removal)
    - [Staking Key Rotation](#staking-key-rotation)
    - [SIGN_MODE_TEXTUAL Removal](#sign_mode_textual-removal)
    - [Zero-Height Export](#zero-height-export)
    - [Upgrade Handler and Store Migrations](#upgrade-handler-and-store-migrations)
- [Upgrading from v0.24.x](#upgrading-from-v024x)
- [New Features and Non-Breaking Changes](#new-features-and-non-breaking-changes)
- [Behavior Changes Affecting Dapps and Operators](#behavior-changes-affecting-dapps-and-operators)

## Breaking Changes

### Dependencies

**Required action:** copy the cosmos-sdk and CometBFT `replace` directives from
Ethermint's `go.mod`. Go does not apply a dependency's `replace` lines.

```go
replace (
	github.com/cosmos/cosmos-sdk => github.com/crypto-org-chain/cosmos-sdk <rev from ethermint go.mod>
	github.com/cometbft/cometbft => github.com/crypto-org-chain/cometbft <rev from ethermint go.mod>
)
```

### x/params Removal

The SDK deleted `x/params`. Ethermint keeps a read-only, decode-only
replacement in `types/legacyparams`. Nothing writes to the `params` store any
more.

**Required action:** remove `ParamsKeeper`, `GetSubspace`, the params module and
its proposal handler, and the subspace argument of every `NewAppModule`. Then
pick the evm and feemarket keeper wiring for your chain.

If your chain ever ran evm before consensus version 5 or feemarket before
consensus version 4, keep mounting the `params` store and pass real subspaces,
so queries at those heights get that height's params. Evm version 4 needs it
too: for a false bool, an empty denom or empty extra EIPs it falls back to the
`x/params` value.

Before:

```go
app.ParamsKeeper = initParamsKeeper(appCodec, cdc, keys[paramstypes.StoreKey], tkeys[paramstypes.TStoreKey])

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

After:

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

Otherwise pass `legacyparams.Subspace{}` in the same places, stop mounting the
`params` store, and list `params` in the upgrade's `StoreUpgrades.Deleted`. The
SDK store ignores that entry for an unmounted store, but memiavl
(cronos-store) needs it: without it the node fails to start with
`memiavl tree membership mismatch`.

```go
if upgradeInfo.Name == "sdk55" && !app.UpgradeKeeper.IsSkipHeight(upgradeInfo.Height) {
	app.SetStoreLoader(upgradetypes.UpgradeStoreLoader(upgradeInfo.Height, &storetypes.StoreUpgrades{
		Deleted: []string{"params"},
	}))
}
```

Also:

- Drop the subspace argument from the evm `NewMigrator`. The feemarket
  `Migrator` is gone.
- `evmtypes.ValidateEVMDenom` now takes a `string` and
  `evmtypes.ValidateInt64Overflow` a `uint64`.
- These exports are gone:
    - evm: `types.ValidateBool`, `types.ValidateChainConfig` (call
      `ChainConfig.Validate`), `types.Subspace`, `types.LegacyParams`,
      `Keeper.GetLegacyParams` and `migrations/v4.MigrateStore`.
    - feemarket: `types.Subspace`, `types.LegacyParams`, `ParamKeyTable`,
      `Params.ParamSetPairs`, the `ParamStoreKey*` keys and the
      `migrations/v4` package.
    - evmd: `GetSubspace` and `GetTKey`.
- Run evm and feemarket at consensus version 4 or later before upgrading; the
  3→4 migrations are gone.
- An app that builds its interface registry without
  `encoding/codec.RegisterInterfaces` must call `legacyparams.RegisterInterfaces`,
  so gov queries and `export` still decode historical `ParameterChangeProposal`s.
- Remove any copy of the `cosmos.params.v1beta1` protos from your app.
  gogoproto keeps the first registration of a type name, so a second copy gets
  an empty type URL.

### Staking Key Rotation

**Required action:** add the key rotation fee pool to the module account
permissions. The staking keeper panics at construction without it.

```go
maccPerms = map[string][]string{
	// ...
	stakingtypes.KeyRotationFeePoolName: {authtypes.Burner},
}
```

If you pass your own `StakingKeeper` to the evm keeper, implement
`GetRotationLockedConsAddr`; the SDK staking keeper already has it.

Genesis tooling that changes the staking `bond_denom` must set the
`key_rotation_fee` denom to the same value. Staking genesis validation rejects
a mismatch.

### SIGN_MODE_TEXTUAL Removal

**Required action:** remove `SIGN_MODE_TEXTUAL` and `TextualCoinMetadataQueryFn`
from the tx config in the app and the root command.

Before:

```go
enabledSignModes := append(slices.Clone(authtx.DefaultSignModes), sigtypes.SignMode_SIGN_MODE_TEXTUAL)
txConfig, err := authtx.NewTxConfigWithOptions(appCodec, authtx.ConfigOptions{
	EnabledSignModes:           enabledSignModes,
	TextualCoinMetadataQueryFn: txmodule.NewBankKeeperCoinMetadataQueryFn(app.BankKeeper),
})
```

After: use the default tx config.

### Zero-Height Export

**Required action:** set the export context to height 0 after
`prepForZeroHeightGenesis`, and rebase pending key rotations.

```go
if forZeroHeight {
	height = 0
	if err := app.prepForZeroHeightGenesis(ctx, jailAllowedAddrs); err != nil {
		return servertypes.ExportedApp{}, err
	}
	ctx = ctx.WithBlockHeight(height)
}
```

In `prepForZeroHeightGenesis`, after `ApplyAndReturnValidatorSetUpdates`:

```go
if err := app.StakingKeeper.PrepareConsKeyRotationsForZeroHeightExport(ctx); err != nil {
	return err
}
```

Without the height reset, a pending key rotation makes the exported validator
set use the rotated key while `InitChain` expects the old one, and the genesis
fails to start. The rebase moves the pending rotations' apply heights onto the
restarted chain.

### Upgrade Handler and Store Migrations

#### Module Migrations

`RunMigrations` runs:

- staking 5→6: sets `key_rotation_fee` to 1000000 of the bond denom. That is
  close to free on an 18-decimal denom; set your own fee in the upgrade handler
  if it is too low.
- auth 6→7.

#### Reference Upgrade Handler

From `evmd/upgrades.go`:

```go
app.UpgradeKeeper.SetUpgradeHandler("sdk55",
	func(ctx context.Context, _ upgradetypes.Plan, fromVM module.VersionMap) (module.VersionMap, error) {
		sdk55.EnsureModuleAccount(sdk.UnwrapSDKContext(ctx), app.AccountKeeper, stakingtypes.KeyRotationFeePoolName)
		if err := app.StakingKeeper.PopulateQueuePendingSlots(ctx); err != nil {
			return nil, err
		}
		return app.ModuleManager.RunMigrations(ctx, app.configurator, fromVM)
	},
)
```

- `sdk55.EnsureModuleAccount` creates the `key_rotation_fee_pool` module
  account, converting any plain account already at its address. Anyone can
  create that plain account by sending it funds, and it makes every key
  rotation fail.
- `PopulateQueuePendingSlots` rebuilds the fork's staking queue index.
  Unbonding validators, unbondings and redelegations queued before the upgrade
  never mature without it. It does a full scan, so it is safe to run again.

#### IBC Send Sequences

ibc-go v11.2.1 reads send sequences from a new key and registers no migration
for it. `evmd` has no channels and skips this step. If your chain has IBC
channels, call the ibc-go migration in your handler:

```go
import channelv11 "github.com/cosmos/ibc-go/v11/modules/core/04-channel/migrations/v11"

err := channelv11.MigrateStore(
	sdk.UnwrapSDKContext(ctx), runtime.NewKVStoreService(keys[ibcexported.StoreKey]), appCodec, app.IBCKeeper,
)
```

- Without it, every send on an existing channel fails with
  `ErrSequenceSendNotFound` and `export` panics.
- Run it exactly once. A second run finds the old keys gone and resets every
  send sequence to 0.
- It panics if a channel's connection is missing, so test it on a copy of your
  chain's state first.
- It covers channels only. IBC v2 send sequences, keyed by client ID, also move
  (`nextSequenceSend/<client>` to `nextSequenceSend//<client>`); copy them
  yourself if your chain has sent IBC v2 packets.

## Upgrading from v0.24.x

1. Bump Ethermint and copy its `replace` directives.
2. Apply the wiring changes in [Breaking Changes](#breaking-changes).
3. Register an upgrade handler like the reference one, with the store upgrades
   for your params choice.
4. Raise the gRPC message size limits if you serve large `debug_trace*`
   responses (see below).

## New Features and Non-Breaking Changes

- Consensus key rotation (`MsgRotateConsPubKey`).
- ML-DSA-65 account keys.
- ML-DSA-65 consensus keys, once `ml_dsa_65` is in the consensus params'
  `validator.pub_key_types`. This build's IBC light client verifies headers
  signed with them. A counterparty decodes the keys only on CometBFT v0.38.26
  or later in the v0.38 line, or v0.40 or later; v0.39.x cannot. Otherwise its
  client of this chain stops updating once an active validator uses one.
  Relayers must decode them too: hermes needs crypto-org-chain/hermes#11 and
  the Go relayer needs crypto-org-chain/relayer#22. Allow the key type only
  after every counterparty and relayer supports it. Each ML-DSA-65 validator
  adds about 7KB to every `MsgUpdateClient`.

## Behavior Changes Affecting Dapps and Operators

- The SDK fork lowers the default gRPC `max-send-msg-size` from 2GB to 10MB. If
  large `debug_trace*` responses fail with `ResourceExhausted`, raise both
  `max-send-msg-size` and `max-recv-msg-size`; the JSON-RPC server's gRPC client
  also caps what it receives.
- The chain rejects txs signed with `SIGN_MODE_TEXTUAL`. `--sign-mode textual`
  is gone; the CLI signs with the default mode instead, without an error.
- New `ParameterChangeProposal`s are rejected: no gov handler serves the
  `params` route, and `ParameterChange` is not a registered v1beta1 proposal
  type.
- EVM queries at heights before evm consensus version 5 or feemarket consensus
  version 4 return wrong params with no error unless the legacy subspaces are
  wired: heights before version 4 get an empty chain config (Frontier rules) and
  no base fee, and evm version 4 heights get false, an empty denom or no extra
  EIPs where the chain used the `x/params` value.
- `eth_sign`, `eth_signTypedData` and `MsgEthereumTx.Sign` use
  `SIGN_MODE_DIRECT`, so Ledger keys fail with `ErrInvalidSignMode`.
- ibc-go v11.2.1-rc1 is a release candidate. `SendPacket` rejects a timeout
  height whose revision number is above the counterparty client's, and
  `MsgTransfer` can send IBC v2 packets over v1 channel IDs (`use_aliasing`).

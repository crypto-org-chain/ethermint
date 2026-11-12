# Cosmos SDK v0.55

Ethermint builds against cosmos-sdk v0.55 (fork `release/v0.55.x`), CometBFT
v0.40 (fork `v0.40.x`) and ibc-go v11.2.1-rc1.

## Steps for apps built on ethermint

- Run evm and feemarket at consensus version 4 or later before upgrading; the
  3→4 migrations are gone.
- Repeat the cosmos-sdk and CometBFT `replace` directives from ethermint's
  `go.mod`; Go does not apply a dependency's `replace` lines.
- Add `stakingtypes.KeyRotationFeePoolName: {authtypes.Burner}` to the module
  account permissions, or the staking keeper panics at construction.
- In the upgrade handler, create the `key_rotation_fee_pool` module account,
  converting any plain account already at its address (see
  `ensureModuleAccount` in `evmd/upgrades.go`). Anyone can create that plain
  account by sending it funds, and it makes every key rotation fail.
- In the upgrade handler, call `StakingKeeper.PopulateQueuePendingSlots`.
  Unbonding validators, unbondings and redelegations queued by a binary
  without the fork's staking queue index never mature without it. It rebuilds
  the index from a full scan, so it is safe even if the index already exists.
- Stop mounting the `params` store, and list `params` in the upgrade's
  `StoreUpgrades.Deleted`. The SDK store ignores that entry for an unmounted
  store, but memiavl (cronos-store) keeps the tree, and its app hash, without
  it.
- Drop the `x/params` subspace argument from the evm and feemarket
  `NewKeeper` and `NewAppModule` and from the evm `NewMigrator`, and remove
  `ParamsKeeper` and `GetSubspace` wiring. The feemarket `Migrator` is gone.
- Implement `GetRotationLockedConsAddr` on the `StakingKeeper` passed to the
  evm keeper; the SDK staking keeper already has it.
- Remove `SIGN_MODE_TEXTUAL` and `TextualCoinMetadataQueryFn` from the tx
  config in the app and the root command; the SDK no longer has them.
- In zero-height export, call
  `StakingKeeper.PrepareConsKeyRotationsForZeroHeightExport` after
  `ApplyAndReturnValidatorSetUpdates`. Otherwise a pending rotation leaves
  staking on the old key while CometBFT runs the new one.
- Set the staking `key_rotation_fee` denom to the bond denom in genesis
  tooling.

## Operator notes

- The SDK fork lowers the default gRPC `max-send-msg-size` from 2GB to 10MB.
  Raise it if large `debug_trace*` responses fail with `ResourceExhausted`.
- The `cosmos.params.v1beta1` types are gone. Apps holding historical
  `ParameterChangeProposal`s must register decode-only copies, or gov proposal
  queries and `export` fail.
- EVM queries at heights before evm/feemarket consensus version 4 read
  zero-value params. Serve them with an older binary.
- `eth_sign`, `eth_signTypedData` and `MsgEthereumTx.Sign` now use
  `SIGN_MODE_DIRECT`, so Ledger keys fail with `ErrInvalidSignMode`.
- `--sign-mode textual` is gone. The CLI signs with the default mode instead,
  without an error.

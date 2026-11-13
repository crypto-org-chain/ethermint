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
- In the upgrade handler, create the `key_rotation_fee_pool` module account
  with `upgrades/sdk55.EnsureModuleAccount`, which converts any plain account
  already at its address. Anyone can create that plain account by sending it
  funds, and it makes every key rotation fail. `evmd/upgrades.go` shows the
  full handler.
- In the upgrade handler, call `StakingKeeper.PopulateQueuePendingSlots`.
  Unbonding validators, unbondings and redelegations queued by a binary
  without the fork's staking queue index never mature without it. It rebuilds
  the index from a full scan, so it is safe even if the index already exists.
- In the upgrade handler, after `RunMigrations`, call
  `upgrades/sdk55.MigrateChannelSendSequences`. ibc-go v11.2.1 reads IBC v1
  channel send sequences from a new key and registers no migration for it, so
  without this step every transfer on an existing channel fails and `export`
  panics. Don't call ibc-go's `04-channel/migrations/v11.MigrateStore`
  directly: a second run of it sets every channel's send sequence to 0. The
  wrapper is safe to repeat, also after a registered copy of that migration
  runs, but cannot restore a sequence an earlier direct run already reset.
  ibc-go v11.2.1 also moves IBC v2 send sequences to a new key without a
  migration; check this if your app routes IBC v2 packets (evmd does not).
- Stop mounting the `params` store, and list `params` in the upgrade's
  `StoreUpgrades.Deleted`. The SDK store ignores that entry for an unmounted
  store, but memiavl (cronos-store) needs it: without it the node fails to
  start with `memiavl tree membership mismatch`, and older cronos-store
  versions keep the tree in the app hash.
- Drop the `x/params` subspace argument from the evm and feemarket
  `NewKeeper` and `NewAppModule` and from the evm `NewMigrator`, and remove
  `ParamsKeeper` and `GetSubspace` wiring. The feemarket `Migrator` is gone.
- Implement `GetRotationLockedConsAddr` on the `StakingKeeper` passed to the
  evm keeper; the SDK staking keeper already has it.
- Remove `SIGN_MODE_TEXTUAL` and `TextualCoinMetadataQueryFn` from the tx
  config in the app and the root command; the SDK no longer has them.
- In zero-height export, set the export context to height 0
  (`ctx = ctx.WithBlockHeight(height)` after `prepForZeroHeightGenesis`) before
  exporting genesis and calling `staking.WriteValidators`. Otherwise, with a
  key rotation pending, the exported validator set uses the rotated key while
  `InitChain` expects the old one, and the genesis fails to start. Like
  simapp, also call `StakingKeeper.PrepareConsKeyRotationsForZeroHeightExport`
  after `ApplyAndReturnValidatorSetUpdates`.
- Set the staking `key_rotation_fee` denom to the bond denom in genesis
  tooling.
- The upgrade's `RunMigrations` also runs the SDK's staking 5→6 and auth 6→7
  migrations. Staking 5→6 sets `key_rotation_fee` to 1000000 of the bond
  denom, close to free on an 18-decimal denom; set your own fee in the
  upgrade handler if that default is too low.

## Operator notes

- The SDK fork lowers the default gRPC `max-send-msg-size` from 2GB to 10MB.
  If large `debug_trace*` responses fail with `ResourceExhausted`, raise both
  `max-send-msg-size` and `max-recv-msg-size`; the JSON-RPC server's gRPC
  client also caps what it receives.
- After the upgrade the chain rejects txs signed with `SIGN_MODE_TEXTUAL`.
- ibc-go v11.2.1-rc1 is a release candidate. `SendPacket` now rejects a
  timeout height whose revision number is above the counterparty client's,
  and `MsgTransfer` can send IBC v2 packets over v1 channel IDs
  (`use_aliasing`).
- The upgrade enables consensus key rotation (`MsgRotateConsPubKey`) and
  ML-DSA-65 account keys.
- ML-DSA-65 consensus keys need `ml_dsa_65` in the consensus params'
  `validator.pub_key_types`. This build's IBC light client verifies headers
  signed with them, but counterparties on CometBFT before v0.40 (ibc-go v11.2.0
  and older) cannot decode the keys, so their clients of this chain stop
  updating once an active validator uses one. Allow them only after every
  counterparty runs CometBFT v0.40 or later. Each ML-DSA-65 validator also adds
  about 7KB to every `MsgUpdateClient`.
- The `cosmos.params.v1beta1` types are gone. Apps holding historical
  `ParameterChangeProposal`s must register decode-only copies, or gov proposal
  queries and `export` fail.
- EVM queries at heights before evm consensus version 5 or feemarket
  consensus version 4 read zero-value params; evm params moved to their
  current key in the 4→5 migration. Serve those heights with an older binary.
- `eth_sign`, `eth_signTypedData` and `MsgEthereumTx.Sign` now use
  `SIGN_MODE_DIRECT`, so Ledger keys fail with `ErrInvalidSignMode`.
- `--sign-mode textual` is gone. The CLI signs with the default mode instead,
  without an error.

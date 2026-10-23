<!--
order: 8
-->

# Parameters

The evm module contains the following parameters:

## Params

| Key            | Type        | Default Value   |
| -------------- | ----------- | --------------- |
| `EVMDenom`     | string      | `"aphoton"`     |
| `EnableCreate` | bool        | `true`          |
| `EnableCall`   | bool        | `true`          |
| `ExtraEIPs`    | []int       | TBD             |
| `ChainConfig`  | ChainConfig | See ChainConfig |
| `CompactStorage` | bool      | `true`          |
| `StorageSweepBatch` | uint64 | `0`             |

## EVM denom

The evm denomination parameter defines the token denomination used on the EVM state transitions and gas consumption for EVM messages.

For example, on Ethereum, the `evm_denom` would be `ETH`. In the case of Ethermint, the default denomination is the **[atto photon](notion://www.notion.so/docs/basics/photon.md)** (used on the Evmos testnets). In terms of precision, the `PHOTON` and `ETH` share the same value, *i.e* `1 PHOTON = 10^18 atto photon` and `1 ETH = 10^18 wei`.

::: tip
Note: SDK applications that want to import the EVM module as a dependency will need to set their own `evm_denom` (i.e not `"aphoton"`).
:::

## Enable Create

The enable create parameter toggles state transitions that use the `vm.Create` function. When the parameter is disabled, it will prevent all contract creation functionality.

## Enable Transfer

The enable transfer toggles state transitions that use the `vm.Call` function. When the parameter is disabled, it will prevent transfers between accounts and executing a smart contract call.

## Compact Storage

When on, contract storage values are stored without leading zero bytes and a slot set to zero is deleted. Reads
handle both formats. Params that predate the field leave it off; existing chains turn it on with the v9 store
migration, which also starts a sweep that deletes slots stored as zero bytes. A change takes effect from the next
block. Once on, `MsgUpdateParams` rejects turning it off. Only the v9 migration starts the sweep, so turning it on any
other way leaves existing zero slots in place.

A chain restarted from a pre-v9 export starts at module version 9, so the v9 migration never runs for it. Set
`compact_storage: true` in its genesis; `InitGenesis` then drops the exported zero slots
on import.

## Storage Sweep Batch

The number of storage entries the zero-slot sweep visits in each `BeginBlock` while a sweep is running; the maximum
is 20,000. The sweep sets it to `0` when it finishes, and genesis must carry `0`. `MsgUpdateParams`
rejects `0` while a sweep runs (lower the batch to slow it instead) and a non-zero batch when none runs.

`MsgUpdateParams` replaces every param, so a proposal must carry `compact_storage` and `storage_sweep_batch`. A
proposal built from pre-v9 params is rejected because it would turn `compact_storage` off.

## Extra EIPs

The extra EIPs parameter defines the set of activateable Ethereum Improvement Proposals (**[EIPs](https://ethereum.org/en/eips/)**)
on the Ethereum VM `Config` that apply custom jump tables.

::: tip
NOTE: some of these EIPs are already enabled by the chain configuration, depending on the hard fork number.
:::

The supported activateable EIPS are:

- **[EIP 1344](https://eips.ethereum.org/EIPS/eip-1344)**
- **[EIP 1884](https://eips.ethereum.org/EIPS/eip-1884)**
- **[EIP 2200](https://eips.ethereum.org/EIPS/eip-2200)**
- **[EIP 2315](https://eips.ethereum.org/EIPS/eip-2315)**
- **[EIP 2929](https://eips.ethereum.org/EIPS/eip-2929)**
- **[EIP 3198](https://eips.ethereum.org/EIPS/eip-3198)**
- **[EIP 3529](https://eips.ethereum.org/EIPS/eip-3529)**

## Chain Config

The `ChainConfig` is a protobuf wrapper type that contains the same fields as the go-ethereum `ChainConfig` parameters, but using `*sdk.Int` types instead of `*big.Int`.

By default, all block configuration fields but `ConstantinopleBlock`, are enabled at genesis (height 0).

### ChainConfig Defaults

| Name                | Default Value                                                        |
| ------------------- | -------------------------------------------------------------------- |
| HomesteadBlock      | 0                                                                    |
| DAOForkBlock        | 0                                                                    |
| DAOForkSupport      | `true`                                                               |
| EIP150Block         | 0                                                                    |
| EIP150Hash          | `0x0000000000000000000000000000000000000000000000000000000000000000` |
| EIP155Block         | 0                                                                    |
| EIP158Block         | 0                                                                    |
| ByzantiumBlock      | 0                                                                    |
| ConstantinopleBlock | 0                                                                    |
| PetersburgBlock     | 0                                                                    |
| IstanbulBlock       | 0                                                                    |
| MuirGlacierBlock    | 0                                                                    |
| BerlinBlock         | 0                                                                    |
| LondonBlock         | 0                                                                    |
| ArrowGlacierBlock   | 0                                                                    |
| GrayGlacierBlock    | 0                                                                    |
| MergeNetsplitBlock  | 0                                                                    |
| ShanghaiTime        | 0                                                                    |
| CancunTime          | 0                                                                    |
| PragueTime          | 0                                                                    |

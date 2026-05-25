# Ethermint JSON-RPC Methods vs Execution-APIs Spec

Spec source: `~/Github/execution-apis/src/`

Legend: ✅ in spec | ❌ missing from spec | ➕ extension (not in spec)

---

## `eth_*` — [rpc/namespaces/ethereum/eth/api.go](rpc/namespaces/ethereum/eth/api.go)

| JSON-RPC Method | Go Method | Line | Spec File | Status |
| --- | --- | --- | --- | --- |
| `eth_blockNumber` | `BlockNumber` | [160](rpc/namespaces/ethereum/eth/api.go#L160) | eth/client.yaml | ✅ |
| `eth_chainId` | `ChainId` | [364](rpc/namespaces/ethereum/eth/api.go#L364) | eth/client.yaml | ✅ |
| `eth_syncing` | `Syncing` | [404](rpc/namespaces/ethereum/eth/api.go#L404) | eth/client.yaml | ✅ |
| `eth_coinbase` | `Coinbase` | [410](rpc/namespaces/ethereum/eth/api.go#L410) | eth/client.yaml | ✅ |
| `eth_accounts` | `Accounts` | [255](rpc/namespaces/ethereum/eth/api.go#L255) | eth/client.yaml | ✅ |
| `eth_getBlockByHash` | `GetBlockByHash` | [172](rpc/namespaces/ethereum/eth/api.go#L172) | eth/block.yaml | ✅ |
| `eth_getBlockByNumber` | `GetBlockByNumber` | [166](rpc/namespaces/ethereum/eth/api.go#L166) | eth/block.yaml | ✅ |
| `eth_getBlockReceipts` | `GetBlockReceipts` | [229](rpc/namespaces/ethereum/eth/api.go#L229) | eth/block.yaml | ✅ |
| `eth_getBlockTransactionCountByHash` | `GetBlockTransactionCountByHash` | [205](rpc/namespaces/ethereum/eth/api.go#L205) | eth/block.yaml | ✅ |
| `eth_getBlockTransactionCountByNumber` | `GetBlockTransactionCountByNumber` | [211](rpc/namespaces/ethereum/eth/api.go#L211) | eth/block.yaml | ✅ |
| `eth_getTransactionByHash` | `GetTransactionByHash` | [182](rpc/namespaces/ethereum/eth/api.go#L182) | eth/transaction.yaml | ✅ |
| `eth_getTransactionByBlockHashAndIndex` | `GetTransactionByBlockHashAndIndex` | [217](rpc/namespaces/ethereum/eth/api.go#L217) | eth/transaction.yaml | ✅ |
| `eth_getTransactionByBlockNumberAndIndex` | `GetTransactionByBlockNumberAndIndex` | [223](rpc/namespaces/ethereum/eth/api.go#L223) | eth/transaction.yaml | ✅ |
| `eth_getTransactionReceipt` | `GetTransactionReceipt` | [198](rpc/namespaces/ethereum/eth/api.go#L198) | eth/transaction.yaml | ✅ |
| `eth_getBalance` | `GetBalance` | [261](rpc/namespaces/ethereum/eth/api.go#L261) | eth/state.yaml | ✅ |
| `eth_getCode` | `GetCode` | [273](rpc/namespaces/ethereum/eth/api.go#L273) | eth/state.yaml | ✅ |
| `eth_getStorageAt` | `GetStorageAt` | [267](rpc/namespaces/ethereum/eth/api.go#L267) | eth/state.yaml | ✅ |
| `eth_getTransactionCount` | `GetTransactionCount` | [188](rpc/namespaces/ethereum/eth/api.go#L188) | eth/state.yaml | ✅ |
| `eth_getProof` | `GetProof` | [279](rpc/namespaces/ethereum/eth/api.go#L279) | eth/state.yaml | ✅ |
| `eth_call` | `Call` | [292](rpc/namespaces/ethereum/eth/api.go#L292) | eth/execute.yaml | ✅ |
| `eth_estimateGas` | `EstimateGas` | [332](rpc/namespaces/ethereum/eth/api.go#L332) | eth/execute.yaml | ✅ |
| `eth_createAccessList` | `CreateAccessList` | [560](rpc/namespaces/ethereum/eth/api.go#L560) | eth/execute.yaml | ✅ |
| `eth_simulateV1` | `SimulateV1` | [509](rpc/namespaces/ethereum/eth/api.go#L509) | eth/execute.yaml | ✅ |
| `eth_gasPrice` | `GasPrice` | [326](rpc/namespaces/ethereum/eth/api.go#L326) | eth/fee_market.yaml | ✅ |
| `eth_maxPriorityFeePerGas` | `MaxPriorityFeePerGas` | [350](rpc/namespaces/ethereum/eth/api.go#L350) | eth/fee_market.yaml | ✅ |
| `eth_feeHistory` | `FeeHistory` | [341](rpc/namespaces/ethereum/eth/api.go#L341) | eth/fee_market.yaml | ✅ |
| `eth_sendRawTransaction` | `SendRawTransaction` | [239](rpc/namespaces/ethereum/eth/api.go#L239) | eth/submit.yaml | ✅ |
| `eth_sendTransaction` | `SendTransaction` | [245](rpc/namespaces/ethereum/eth/api.go#L245) | eth/submit.yaml | ✅ |
| `eth_sign` | `Sign` | [422](rpc/namespaces/ethereum/eth/api.go#L422) | eth/sign.yaml | ✅ |
| `eth_protocolVersion` | `ProtocolVersion` | [320](rpc/namespaces/ethereum/eth/api.go#L320) | — | ➕ extension |
| `eth_getUncleByBlockHashAndIndex` | `GetUncleByBlockHashAndIndex` | [374](rpc/namespaces/ethereum/eth/api.go#L374) | — | ➕ extension |
| `eth_getUncleByBlockNumberAndIndex` | `GetUncleByBlockNumberAndIndex` | [379](rpc/namespaces/ethereum/eth/api.go#L379) | — | ➕ extension |
| `eth_getUncleCountByBlockHash` | `GetUncleCountByBlockHash` | [384](rpc/namespaces/ethereum/eth/api.go#L384) | — | ➕ extension |
| `eth_getUncleCountByBlockNumber` | `GetUncleCountByBlockNumber` | [389](rpc/namespaces/ethereum/eth/api.go#L389) | — | ➕ extension |
| `eth_getTransactionLogs` | `GetTransactionLogs` | [428](rpc/namespaces/ethereum/eth/api.go#L428) | — | ➕ extension |
| `eth_signTypedData` | `SignTypedData` | [468](rpc/namespaces/ethereum/eth/api.go#L468) | — | ➕ extension |
| `eth_fillTransaction` | `FillTransaction` | [476](rpc/namespaces/ethereum/eth/api.go#L476) | — | ➕ extension |
| `eth_resend` | `Resend` | [499](rpc/namespaces/ethereum/eth/api.go#L499) | — | ➕ extension |
| `eth_getPendingTransactions` | `GetPendingTransactions` | [522](rpc/namespaces/ethereum/eth/api.go#L522) | — | ➕ extension |

## `eth_*` filters — [rpc/namespaces/ethereum/eth/filters/api.go](rpc/namespaces/ethereum/eth/filters/api.go)

| JSON-RPC Method | Go Method | Line | Spec File | Status |
| --- | --- | --- | --- | --- |
| `eth_newPendingTransactionFilter` | `NewPendingTransactionFilter` | [136](rpc/namespaces/ethereum/eth/filters/api.go#L136) | eth/filter.yaml | ✅ |
| `eth_newBlockFilter` | `NewBlockFilter` | [159](rpc/namespaces/ethereum/eth/filters/api.go#L159) | eth/filter.yaml | ✅ |
| `eth_newFilter` | `NewFilter` | [191](rpc/namespaces/ethereum/eth/filters/api.go#L191) | eth/filter.yaml | ✅ |
| `eth_getLogs` | `GetLogs` | [220](rpc/namespaces/ethereum/eth/filters/api.go#L220) | eth/filter.yaml | ✅ |
| `eth_uninstallFilter` | `UninstallFilter` | [251](rpc/namespaces/ethereum/eth/filters/api.go#L251) | eth/filter.yaml | ✅ |
| `eth_getFilterLogs` | `GetFilterLogs` | [266](rpc/namespaces/ethereum/eth/filters/api.go#L266) | eth/filter.yaml | ✅ |
| `eth_getFilterChanges` | `GetFilterChanges` | [328](rpc/namespaces/ethereum/eth/filters/api.go#L328) | eth/filter.yaml | ✅ |

## `debug_*` — [rpc/namespaces/ethereum/debug/api.go](rpc/namespaces/ethereum/debug/api.go)

The spec (`debug/getters.yaml`) defines 6 raw-data getters. Ethermint instead implements go-ethereum-style tracing and profiling — zero overlap.

| JSON-RPC Method | Go Method | Line | Spec File | Status |
| --- | --- | --- | --- | --- |
| `debug_traceTransaction` | `TraceTransaction` | [78](rpc/namespaces/ethereum/debug/api.go#L78) | — | ➕ extension |
| `debug_traceBlockByNumber` | `TraceBlockByNumber` | [85](rpc/namespaces/ethereum/debug/api.go#L85) | — | ➕ extension |
| `debug_traceBlockByHash` | `TraceBlockByHash` | [102](rpc/namespaces/ethereum/debug/api.go#L102) | — | ➕ extension |
| `debug_traceCall` | `TraceCall` | [121](rpc/namespaces/ethereum/debug/api.go#L121) | — | ➕ extension |
| `debug_blockProfile` | `BlockProfile` | [140](rpc/namespaces/ethereum/debug/api.go#L140) | — | ➕ extension |
| `debug_cpuProfile` | `CpuProfile` | [155](rpc/namespaces/ethereum/debug/api.go#L155) | — | ➕ extension |
| `debug_gcStats` | `GcStats` | [169](rpc/namespaces/ethereum/debug/api.go#L169) | — | ➕ extension |
| `debug_goTrace` | `GoTrace` | [178](rpc/namespaces/ethereum/debug/api.go#L178) | — | ➕ extension |
| `debug_memStats` | `MemStats` | [192](rpc/namespaces/ethereum/debug/api.go#L192) | — | ➕ extension |
| `debug_setBlockProfileRate` | `SetBlockProfileRate` | [201](rpc/namespaces/ethereum/debug/api.go#L201) | — | ➕ extension |
| `debug_stacks` | `Stacks` | [207](rpc/namespaces/ethereum/debug/api.go#L207) | — | ➕ extension |
| `debug_startCPUProfile` | `StartCPUProfile` | [218](rpc/namespaces/ethereum/debug/api.go#L218) | — | ➕ extension |
| `debug_stopCPUProfile` | `StopCPUProfile` | [258](rpc/namespaces/ethereum/debug/api.go#L258) | — | ➕ extension |
| `debug_writeBlockProfile` | `WriteBlockProfile` | [284](rpc/namespaces/ethereum/debug/api.go#L284) | — | ➕ extension |
| `debug_writeMemProfile` | `WriteMemProfile` | [292](rpc/namespaces/ethereum/debug/api.go#L292) | — | ➕ extension |
| `debug_mutexProfile` | `MutexProfile` | [300](rpc/namespaces/ethereum/debug/api.go#L300) | — | ➕ extension |
| `debug_setMutexProfileFraction` | `SetMutexProfileFraction` | [313](rpc/namespaces/ethereum/debug/api.go#L313) | — | ➕ extension |
| `debug_writeMutexProfile` | `WriteMutexProfile` | [319](rpc/namespaces/ethereum/debug/api.go#L319) | — | ➕ extension |
| `debug_freeOSMemory` | `FreeOSMemory` | [325](rpc/namespaces/ethereum/debug/api.go#L325) | — | ➕ extension |
| `debug_setGCPercent` | `SetGCPercent` | [332](rpc/namespaces/ethereum/debug/api.go#L332) | — | ➕ extension |
| `debug_getHeaderRlp` | `GetHeaderRlp` | [338](rpc/namespaces/ethereum/debug/api.go#L338) | — | ➕ extension |
| `debug_getBlockRlp` | `GetBlockRlp` | [352](rpc/namespaces/ethereum/debug/api.go#L352) | — | ➕ extension |
| `debug_printBlock` | `PrintBlock` | [366](rpc/namespaces/ethereum/debug/api.go#L366) | — | ➕ extension |
| `debug_intermediateRoots` | `IntermediateRoots` | [380](rpc/namespaces/ethereum/debug/api.go#L380) | — | ➕ extension |
| `debug_startGoTrace` | `StartGoTrace` | [32](rpc/namespaces/ethereum/debug/trace.go#L32) | — | ➕ extension |
| `debug_stopGoTrace` | `StopGoTrace` | [97](rpc/namespaces/ethereum/debug/trace.go#L97) | — | ➕ extension |

## `personal_*` — [rpc/namespaces/ethereum/personal/api.go](rpc/namespaces/ethereum/personal/api.go)

Not part of execution-apis spec (deprecated go-ethereum namespace).

| JSON-RPC Method | Go Method | Line | Status |
| --- | --- | --- | --- |
| `personal_importRawKey` | `ImportRawKey` | [74](rpc/namespaces/ethereum/personal/api.go#L74) | ➕ extension |
| `personal_listAccounts` | `ListAccounts` | [80](rpc/namespaces/ethereum/personal/api.go#L80) | ➕ extension |
| `personal_lockAccount` | `LockAccount` | [87](rpc/namespaces/ethereum/personal/api.go#L87) | ➕ extension |
| `personal_newAccount` | `NewAccount` | [95](rpc/namespaces/ethereum/personal/api.go#L95) | ➕ extension |
| `personal_unlockAccount` | `UnlockAccount` | [122](rpc/namespaces/ethereum/personal/api.go#L122) | ➕ extension |
| `personal_sendTransaction` | `SendTransaction` | [131](rpc/namespaces/ethereum/personal/api.go#L131) | ➕ extension |
| `personal_sign` | `Sign` | [145](rpc/namespaces/ethereum/personal/api.go#L145) | ➕ extension |
| `personal_ecRecover` | `EcRecover` | [160](rpc/namespaces/ethereum/personal/api.go#L160) | ➕ extension |
| `personal_unpair` | `Unpair` | [182](rpc/namespaces/ethereum/personal/api.go#L182) | ➕ extension |
| `personal_initializeWallet` | `InitializeWallet` | [190](rpc/namespaces/ethereum/personal/api.go#L190) | ➕ extension |
| `personal_listWallets` | `ListWallets` | [207](rpc/namespaces/ethereum/personal/api.go#L207) | ➕ extension |

## `net_*` — [rpc/namespaces/ethereum/net/api.go](rpc/namespaces/ethereum/net/api.go)

| JSON-RPC Method | Go Method | Line | Spec File | Status |
| --- | --- | --- | --- | --- |
| `net_version` | `Version` | [48](rpc/namespaces/ethereum/net/api.go#L48) | eth/client.yaml | ✅ |
| `net_listening` | `Listening` | [53](rpc/namespaces/ethereum/net/api.go#L53) | — | ➕ extension |
| `net_peerCount` | `PeerCount` | [63](rpc/namespaces/ethereum/net/api.go#L63) | — | ➕ extension |

## `web3_*` — [rpc/namespaces/ethereum/web3/api.go](rpc/namespaces/ethereum/web3/api.go)

Not part of execution-apis spec.

| JSON-RPC Method | Go Method | Line | Status |
| --- | --- | --- | --- |
| `web3_clientVersion` | `ClientVersion` | [34](rpc/namespaces/ethereum/web3/api.go#L34) | ➕ extension |
| `web3_sha3` | `Sha3` | [39](rpc/namespaces/ethereum/web3/api.go#L39) | ➕ extension |

## `txpool_*` — [rpc/namespaces/ethereum/txpool/api.go](rpc/namespaces/ethereum/txpool/api.go)

| JSON-RPC Method | Go Method | Line | Spec File | Status |
| --- | --- | --- | --- | --- |
| `txpool_content` | `Content` | [40](rpc/namespaces/ethereum/txpool/api.go#L40) | txpool/pool.yaml | ✅ |
| `txpool_status` | `Status` | [60](rpc/namespaces/ethereum/txpool/api.go#L60) | txpool/pool.yaml | ✅ |
| `txpool_inspect` | `Inspect` | [50](rpc/namespaces/ethereum/txpool/api.go#L50) | — | ➕ extension |

---

## Missing: In spec but NOT implemented

### `eth_*` (eth/sign.yaml, eth/state.yaml, eth/execute.yaml, eth/fee_market.yaml, eth/client.yaml)

| Spec Method | Spec File | Notes |
| --- | --- | --- |
| `eth_signTransaction` | eth/sign.yaml | Signs tx locally without broadcasting |
| `eth_getStorageValues` | eth/state.yaml | Batch storage slot reads |
| `eth_getBlockAccessList` | eth/block.yaml | Access list for a full block |
| `eth_blobBaseFee` | eth/fee_market.yaml | EIP-4844 blob base fee (no blob tx support) |
| `eth_config` | eth/client.yaml | Node configuration query |

### `debug_*` (debug/getters.yaml)

| Spec Method | Notes |
| --- | --- |
| `debug_getBadBlocks` | Known bad/invalid blocks |
| `debug_getRawBlock` | RLP-encoded block by number |
| `debug_getRawBlockAccessList` | Access list for a raw block |
| `debug_getRawHeader` | RLP-encoded block header |
| `debug_getRawReceipts` | Raw receipts for a block |
| `debug_getRawTransaction` | RLP-encoded transaction |

### `txpool_*` (txpool/pool.yaml)

| Spec Method | Notes |
| --- | --- |
| `txpool_contentFrom` | Filter pending/queued txs by sender address |

### `engine_*`

Not applicable — Ethermint uses Tendermint/CometBFT consensus, not the Ethereum Engine API.

---

## Summary

| Category | Count |
| --- | --- |
| Spec methods implemented | 39 |
| Extensions (not in spec) | 46 |
| **Total implemented** | **85** |
| Spec methods missing (`eth_*`) | 5 |
| Spec methods missing (`debug_*`) | 6 |
| Spec methods missing (`txpool_*`) | 1 |
| `engine_*` (N/A — Tendermint) | 25 |

---

## 结论

**Ethermint 实现了绝大部分核心规范，但有 12 个缺口，且整个 debug 命名空间走错了方向。**

### 缺失方法优先级

| 优先级 | 方法 | 影响 |
| --- | --- | --- |
| 高 | `eth_signTransaction` | 钱包/工具链常用，许多 dApp 会调用 |
| 高 | `eth_getStorageValues` | 批量存储读取，效率接口 |
| 高 | `txpool_contentFrom` | 按发送者过滤 mempool，DEX/MEV 工具依赖 |
| 中 | `debug_getRawBlock` / `debug_getRawHeader` / `debug_getRawReceipts` / `debug_getRawTransaction` / `debug_getRawBlockAccessList` / `debug_getBadBlocks` | 标准 debug 接口，区块链浏览器/索引器会用 |
| 中 | `eth_getBlockAccessList` | 较新接口，使用较少 |
| 低 | `eth_blobBaseFee` | EIP-4844 blob tx，Ethermint 本身不支持 blob，可暂不实现 |
| 低 | `eth_config` | 较新、使用少 |

### debug 命名空间走偏了

规范定义的 `debug_*` 是**原始数据获取**（RLP 块、收据、header），而 Ethermint 实现的是 go-ethereum 的 **tracing/profiling** 接口。两者完全不重叠。这两类都有价值，但当前 Ethermint 没有实现任何一个规范要求的 `debug_*`。

### 总体评价

- 核心 `eth_*` 覆盖率高（29/36 核心方法 + 全部 7 个 filter 方法）
- 扩展丰富（46 个 go-ethereum 扩展），兼容性好
- `engine_*` 不适用（Tendermint 共识）
- 最值得补的是：`eth_signTransaction`、`txpool_contentFrom`、6 个 `debug_getRaw*`

---

## 错误码合规分析

### 规范定义的错误码

| 来源 | 错误码 | 含义 |
| --- | --- | --- |
| JSONRPCStandardErrors | -32700 / -32600 / -32601 / -32602 / -32603 | 解析错误、无效请求、方法不存在、无效参数、内部错误 |
| JSONRPCNonStandardErrors | -32000 ~ -32006 | 无效输入、资源未找到、资源不可用、交易被拒绝等 |
| ExecutionErrors | 1 (nonce too low) / 2 (nonce too high) / 3 (execution reverted) / 4 (invalid opcode) | EVM 执行错误 |
| GasErrors | 800~809 | 内在 gas 不足、gas 用尽、gas 价格过低、余额不足等 |
| TxPoolErrors | 1000 (already known) / 1001 (invalid sender) | mempool 错误 |
| PrunedHistory | 4444 | 历史数据已被裁剪 |
| eth_simulateV1 | -38010 ~ -38026 | 模拟专用错误（nonce、gas、EOA 检查等） |

**根本机制**：go-ethereum 的 JSON-RPC 服务器只识别实现了 `ErrorCode() int` 接口的错误类型。所有 `fmt.Errorf`、`errors.New`、`errorsmod.ABCIError`、`status.Error(codes.Internal,...)` 均**不实现该接口**，一律变成 `-32603 Internal`。

---

### ✅ 符合规范的错误处理（3 处）

| 方法 | 场景 | 实现 | 规范要求 |
| --- | --- | --- | --- |
| `eth_call` / `eth_estimateGas` | EVM revert | `RevertError.ErrorCode() = 3` + `ErrorData()` hex revert bytes ([errors.go:149](x/evm/types/errors.go#L149)) | code `3` + data ✅ |
| `eth_simulateV1` | nonce / gas / EOA 等 | `simulate_errors.go` 完整定义 -38010~-38026 | -38xxx ✅ |
| 块/交易查询 | 资源不存在 | 大多数路径返回 `nil, nil` → JSON `null` | `null` ✅ |

---

### ❌ 不符合规范的错误处理

#### 问题一：10 个方法缺少 error code 4444（历史被裁剪）

规范要求这些方法在访问已裁剪的历史数据时返回 code `4444`，Ethermint 全部没有实现，统一变成 `-32603`。

| 方法 | 规范文件 | 涉及代码位置 |
| --- | --- | --- |
| `eth_getBlockByHash` | block.yaml | [blocks.go:132](rpc/backend/blocks.go#L132) |
| `eth_getBlockByNumber` | block.yaml | [blocks.go:68](rpc/backend/blocks.go#L68) |
| `eth_getBlockTransactionCountByHash` | block.yaml | [blocks.go:160](rpc/backend/blocks.go#L160) |
| `eth_getBlockTransactionCountByNumber` | block.yaml | [blocks.go:181](rpc/backend/blocks.go#L181) |
| `eth_getBlockReceipts` | block.yaml | [blocks.go:95](rpc/backend/blocks.go#L95) |
| `eth_getTransactionByBlockHashAndIndex` | transaction.yaml | [tx_info.go:489](rpc/backend/tx_info.go#L489) |
| `eth_getTransactionByBlockNumberAndIndex` | transaction.yaml | [tx_info.go:512](rpc/backend/tx_info.go#L512) |
| `eth_getTransactionReceipt` | transaction.yaml | [tx_info.go:163](rpc/backend/tx_info.go#L163) |
| `eth_getLogs` | filter.yaml | [filters/api.go:220](rpc/namespaces/ethereum/eth/filters/api.go#L220) |
| `eth_getFilterLogs` | filter.yaml | [filters/api.go:266](rpc/namespaces/ethereum/eth/filters/api.go#L266) |

**修复方向**：新增 `PrunedHistoryError` 类型实现 `ErrorCode() = 4444`，在检测到 pruned 状态时使用。

---

#### 问题二：`eth_sendRawTransaction` / `eth_sendTransaction` — 广播错误码全部丢失

交易广播失败时使用 `errorsmod.ABCIError(rsp.Codespace, rsp.Code, rsp.RawLog)` 包装（[call_tx.go:169](rpc/backend/call_tx.go#L169)、[sign_tx.go:119](rpc/backend/sign_tx.go#L119)），该类型没有实现 `ErrorCode()`，所有失败场景统一变成 `-32603`。

| 失败场景 | 实际返回 | 规范要求 |
| --- | --- | --- |
| gas 不足 / 余额不足 | `-32603` | GasErrors 800~809 |
| nonce too low / too high | `-32603` | ExecutionErrors 1 / 2 |
| execution reverted (via broadcast) | `-32603` | ExecutionErrors 3 |
| 交易被拒绝 | `-32603` | -32003 |
| 已知交易 | `-32603` | TxPoolErrors 1000 |
| invalid sender | `-32603` | TxPoolErrors 1001 |

**修复方向**：解析 ABCI codespace/code，映射到实现了 `ErrorCode()` 的包装类型。

---

#### 问题三：`eth_call` / `eth_estimateGas` — 非 revert VM 错误

`vmError != "execution reverted"` 时，`handleRevertError` 返回 `status.Error(codes.Internal, vmError)`（[call_tx.go:317](rpc/backend/call_tx.go#L317)），变成 `-32603`。

| 失败场景 | 实际返回 | 规范要求 |
| --- | --- | --- |
| INVALID_OPCODE | `-32603` | ExecutionErrors 4 |
| 其他 VM 错误 | `-32603` | 视具体错误 |

---

#### 问题四：`eth_newPendingTransactionFilter` / `eth_newBlockFilter` — 错误以 ID 字符串返回

Filter 数量超限时，这两个方法把错误消息直接编码为 filter ID 字符串返回（[filters/api.go:141](rpc/namespaces/ethereum/eth/filters/api.go#L141)、[filters/api.go:164](rpc/namespaces/ethereum/eth/filters/api.go#L164)），而不是抛出 JSON-RPC error。客户端拿到的是一个"假 ID"，后续用这个 ID 调用任何 filter 方法都会静默失败。

```
return rpc.ID("error creating pending tx filter: max limit reached")
```

**修复方向**：返回类型改为 `(rpc.ID, error)`，或通过 context 传递错误。

---

#### 问题五：`eth_getLogs` / `eth_feeHistory` — 超限返回 -32603 而非 -32005

| 方法 | 超限场景 | 实际返回 | 规范要求 | 代码位置 |
| --- | --- | --- | --- | --- |
| `eth_getLogs` | 查询块范围超过 `RPCBlockRangeCap` | `-32603` | `-32005` | [filters.go:155](rpc/namespaces/ethereum/eth/filters/filters.go#L155) |
| `eth_getLogs` | 返回日志数超过 `RPCLogsCap` | `-32603` | `-32005` | [filters.go:186](rpc/namespaces/ethereum/eth/filters/filters.go#L186) |
| `eth_feeHistory` | 请求块数超过 `FeeHistoryCap` | `-32603` | `-32005` | [chain_info.go:217](rpc/backend/chain_info.go#L217) |

Ethermint 代码库中没有定义 `-32005` 错误类型，`simulate_errors.go` 里虽有 `ErrCodeClientLimitExceeded = -38026`，但那是 `eth_simulateV1` 专用。

**修复方向**：新增 `LimitExceededError` 类型实现 `ErrorCode() = -32005`。

---

### 错误合规汇总（按方法）

| 方法 | 场景 | 实际返回码 | 规范要求码 | 合规 |
| --- | --- | --- | --- | --- |
| `eth_call` / `eth_estimateGas` | EVM revert | `3` + data | `3` + data | ✅ |
| `eth_call` / `eth_estimateGas` | 非 revert VM 错误 | `-32603` | `4` | ❌ |
| `eth_sendRawTransaction` | gas / nonce / 余额 / pool 等所有失败 | `-32603` | 800~809 / 1~2 / -32003 / 1000 / 1001 | ❌ |
| `eth_sendTransaction` | 同上 | `-32603` | 同上 | ❌ |
| `eth_simulateV1` | nonce / gas / EOA 等 | `-38010~-38026` | `-38010~-38026` | ✅ |
| `eth_getBlockByHash` | 历史被裁剪 | `-32603` | `4444` | ❌ |
| `eth_getBlockByNumber` | 历史被裁剪 | `-32603` | `4444` | ❌ |
| `eth_getBlockTransactionCountByHash` | 历史被裁剪 | `-32603` | `4444` | ❌ |
| `eth_getBlockTransactionCountByNumber` | 历史被裁剪 | `-32603` | `4444` | ❌ |
| `eth_getBlockReceipts` | 历史被裁剪 | `-32603` | `4444` | ❌ |
| `eth_getTransactionByBlockHashAndIndex` | 历史被裁剪 | `-32603` | `4444` | ❌ |
| `eth_getTransactionByBlockNumberAndIndex` | 历史被裁剪 | `-32603` | `4444` | ❌ |
| `eth_getTransactionReceipt` | 历史被裁剪 | `-32603` | `4444` | ❌ |
| `eth_getLogs` | 历史被裁剪 | `-32603` | `4444` | ❌ |
| `eth_getFilterLogs` | 历史被裁剪 | `-32603` | `4444` | ❌ |
| `eth_newPendingTransactionFilter` | 超过 filter 数量上限 | 假 ID 字符串 | JSON-RPC error | ❌ |
| `eth_newBlockFilter` | 超过 filter 数量上限 | 假 ID 字符串 | JSON-RPC error | ❌ |
| `eth_getLogs` | 块范围 / 日志数超限 | `-32603` | `-32005` | ❌ |
| `eth_feeHistory` | 请求块数超限 | `-32603` | `-32005` | ❌ |
| `eth_getBalance` / `eth_getCode` / `eth_getStorageAt` / `eth_getTransactionCount` | 各类错误 | `-32603` | 规范未定义特定码 | ✅ |
| `eth_getTransactionByHash` | 不存在 | `null` | `null` | ✅ |
| 块/交易查询 | 资源不存在 | `null` | `null` | ✅ |

---

## 返回值字段完整性

### `eth_getTransactionReceipt` — 缺少 EIP-4844 字段

实现位置：[tx_info.go](rpc/backend/tx_info.go)

| 字段 | 规范要求 | 实现状态 |
| --- | --- | --- |
| `blockHash` | ✅ | ✅ |
| `blockNumber` | ✅ | ✅ |
| `from` | ✅ | ✅ |
| `cumulativeGasUsed` | ✅ | ✅ |
| `gasUsed` | ✅ | ✅ |
| `effectiveGasPrice` | ✅ | ✅ |
| `logs` / `logsBloom` | ✅ | ✅ |
| `status` / `type` | ✅ | ✅ |
| `contractAddress` | ✅ | ✅ |
| `blobGasUsed` | EIP-4844 必需 | ❌ 缺失 |
| `blobGasPrice` | EIP-4844 必需 | ❌ 缺失 |

---

### `eth_getBlockByHash` / `eth_getBlockByNumber` — 缺少后 EIP-4895/4844/4788 字段

实现位置：[rpc/types/utils.go](rpc/types/utils.go)

| 字段 | 引入版本 | 实现状态 |
| --- | --- | --- |
| `baseFeePerGas` | EIP-1559 | ✅ |
| `withdrawals` | EIP-4895 (Shanghai) | ❌ 缺失 |
| `withdrawalsRoot` | EIP-4895 (Shanghai) | ❌ 缺失 |
| `blobGasUsed` | EIP-4844 (Cancun) | ❌ 缺失 |
| `excessBlobGas` | EIP-4844 (Cancun) | ❌ 缺失 |
| `parentBeaconBlockRoot` | EIP-4788 (Cancun) | ❌ 缺失 |

---

### `eth_getTransactionByHash` — 缺少 EIP-4844 字段

实现位置：[rpc/types/types.go](rpc/types/types.go)

| 字段 | 引入版本 | 实现状态 |
| --- | --- | --- |
| `accessList` | EIP-2930 | ✅ |
| `maxFeePerGas` / `maxPriorityFeePerGas` | EIP-1559 | ✅ |
| `type` / `chainId` / `yParity` | EIP-2718 | ✅ |
| `authorizationList` | EIP-7702 | ✅ |
| `blobVersionedHashes` | EIP-4844 | ❌ 缺失（架构差距） |
| `maxFeePerBlobGas` | EIP-4844 | ❌ 缺失（架构差距） |

---

### `eth_createAccessList` — 响应缺少 `error` 字段

实现位置：[x/evm/types/access_list.go:72](x/evm/types/access_list.go#L72)

规范要求返回 `{ accessList, error, gasUsed }` 三个字段，其中 `error` 是一个字符串，用于在 access list 计算遇到 EVM 错误时（如 revert）将错误信息内联到响应体中（而不是抛出 JSON-RPC error）。Ethermint 的 `AccessListResult` 只有两个字段：

```go
type AccessListResult struct {
    Accesslist ethtypes.AccessList `json:"accessList"`
    GasUsed    uint64              `json:"gasUsed"`
    // 缺少：Error string `json:"error,omitempty"`
}
```

客户端无法得知 access list 计算过程中发生的 EVM 错误。

---

### `eth_getProof` — 响应缺少 `address` 字段

实现位置：[rpc/types/types.go:37](rpc/types/types.go#L37)

规范的 `AccountProof` 对象要求包含 `address` 字段，Ethermint 的 `AccountResult` 结构体没有该字段：

```go
type AccountResult struct {
    // 缺少：Address common.Address `json:"address"`
    AccountProof []string        `json:"accountProof"`
    Balance      *hexutil.Big    `json:"balance"`
    CodeHash     common.Hash     `json:"codeHash"`
    Nonce        hexutil.Uint64  `json:"nonce"`
    StorageHash  common.Hash     `json:"storageHash"`
    StorageProof []StorageResult `json:"storageProof"`
}
```

---

### 字段缺失说明

- **架构差距（不可修复）**：EIP-4844 blob 字段（`blobVersionedHashes`、`maxFeePerBlobGas`、`blobGasUsed`、`blobGasPrice`）及 EIP-4895 withdrawals 字段，Ethermint 基于 Tendermint 共识，不支持这些特性，客户端不应期望这些字段。
- **可修复 bug**：`eth_createAccessList` 缺少 `error` 字段、`eth_getProof` 缺少 `address` 字段，与架构无关，应补齐。

---

## 问题方法汇总（共 20 个）

| # | 方法 | 问题类型 | 可修复 |
| --- | --- | --- | --- |
| 1 | `eth_sendRawTransaction` | 广播错误码全部丢失（→ -32603） | ✅ |
| 2 | `eth_sendTransaction` | 广播错误码全部丢失（→ -32603） | ✅ |
| 3 | `eth_call` | 非 revert VM 错误返回 -32603，应为 4 | ✅ |
| 4 | `eth_estimateGas` | 非 revert VM 错误返回 -32603，应为 4 | ✅ |
| 5 | `eth_getBlockByHash` | 4444 缺失 + 5 个字段缺失 | 部分 |
| 6 | `eth_getBlockByNumber` | 4444 缺失 + 5 个字段缺失 | 部分 |
| 7 | `eth_getBlockTransactionCountByHash` | 4444 缺失 | ✅ |
| 8 | `eth_getBlockTransactionCountByNumber` | 4444 缺失 | ✅ |
| 9 | `eth_getBlockReceipts` | 4444 缺失 | ✅ |
| 10 | `eth_getTransactionByBlockHashAndIndex` | 4444 缺失 | ✅ |
| 11 | `eth_getTransactionByBlockNumberAndIndex` | 4444 缺失 | ✅ |
| 12 | `eth_getTransactionReceipt` | 4444 缺失 + 2 个字段缺失 | 部分 |
| 13 | `eth_getLogs` | 4444 缺失 + -32005 缺失 | ✅ |
| 14 | `eth_getFilterLogs` | 4444 缺失 | ✅ |
| 15 | `eth_newPendingTransactionFilter` | 错误塞进 ID 字符串 | ✅ |
| 16 | `eth_newBlockFilter` | 错误塞进 ID 字符串 | ✅ |
| 17 | `eth_feeHistory` | -32005 缺失 | ✅ |
| 18 | `eth_getTransactionByHash` | 2 个字段缺失（EIP-4844 架构差距） | ❌ |
| 19 | `eth_createAccessList` | 响应缺少 `error` 字段 | ✅ |
| 20 | `eth_getProof` | 响应缺少 `address` 字段 | ✅ |

---

## 返回数据格式合规分析

以下检查每个字段的**编码格式**是否符合规范（数值是否为 hex 字符串、长度是否正确、类型是否匹配等）。

### ✅ 格式正确

| 方法 | 字段 | 规范类型 | 实现 |
| --- | --- | --- | --- |
| `eth_getBalance` | 返回值 | uint256（hex） | `*hexutil.Big` ✅ |
| `eth_getTransactionCount` | 返回值 | uint64（hex） | `hexutil.Uint64` ✅ |
| `eth_feeHistory` | baseFeePerGas | uint[]（hex） | `[]*hexutil.Big` ✅ |
| `eth_feeHistory` | gasUsedRatio | float[] | `[]float64` ✅ |
| `eth_feeHistory` | reward | uint[][]（hex） | `[][]*hexutil.Big` ✅ |
| `eth_getTransactionReceipt` | status | uint（"0x0"/"0x1"） | `hexutil.Uint` ✅ |
| `eth_getTransactionReceipt` | contractAddress | address or null | nil / address ✅ |
| `eth_syncing` | 不同步时 | false（bool） | `false` ✅ |
| `eth_getFilterChanges` | block filter | hash32[] | `[]common.Hash` ✅ |

---

### ❌ 格式错误

#### 1. `eth_getBlockByHash` / `eth_getBlockByNumber` — `nonce` 字段返回空字节

**问题**：块 `nonce` 字段使用 `ethtypes.BlockNonce{}`（空结构体），序列化为 `"0x"`（0 字节），而规范要求 `bytes8`（8 字节），PoS 链应固定为 `"0x0000000000000000"`。

实现位置：[rpc/types/utils.go:163](rpc/types/utils.go#L163)

---

#### 2. 所有 EIP-1559/2930 交易 — `yParity` 计算错误（严重）

**问题**：`yParity` 使用 `hexutil.Uint64(v.Sign())` 计算（[rpc/types/utils.go:226](rpc/types/utils.go#L226)），`v.Sign()` 返回的是 big.Int 的正负符号（-1/0/1），**不是签名的 y 轴奇偶位**。正确做法是从签名中提取实际的 parity bit（0 或 1）。

**影响**：所有 EIP-1559、EIP-2930、EIP-7702 交易的 `yParity` 字段值错误，客户端用此字段恢复公钥时会得到错误结果。

涉及交易类型：`DynamicFeeTxType`（type 2）、`AccessListTxType`（type 1）、`SetCodeTxType`（type 4）

实现位置：[rpc/types/utils.go:226](rpc/types/utils.go#L226)

---

#### 3. `eth_getTransactionReceipt` — `effectiveGasPrice` 仅对 EIP-1559 交易设置

**问题**：`effectiveGasPrice` 只在 `DynamicFeeTxType` 时写入（[rpc/backend/tx_info.go:475](rpc/backend/tx_info.go#L475)），legacy 交易和 EIP-2930 交易的 receipt 中缺少该字段。规范要求所有类型的交易都必须包含此字段。

---

#### 4. `eth_syncing` — 同步中时缺少 `highestBlock` 字段

**问题**：节点同步中时，响应体只包含 `startingBlock` 和 `currentBlock`，缺少规范要求的 `highestBlock`（[rpc/backend/node_info.go:89](rpc/backend/node_info.go#L89)）。客户端无法计算同步进度百分比。

---

#### 5. Log 对象 — 缺少 `blockTimestamp` 字段

**问题**：规范的 Log schema 要求包含 `blockTimestamp`（uint hex），Ethermint 使用 go-ethereum 的原生 `ethtypes.Log` 结构，该结构没有 `blockTimestamp` 字段，导致所有 log 响应（`eth_getLogs`、`eth_getFilterChanges`、receipt 中的 logs）都缺少此字段。

---

### 返回格式问题汇总

| # | 方法 | 字段 | 实际返回 | 规范要求 | 严重程度 |
| --- | --- | --- | --- | --- | --- |
| 1 | `eth_getBlockByHash` / `eth_getBlockByNumber` | `nonce` | `"0x"` (0字节) | `"0x0000000000000000"` (8字节) | 中 |
| 2 | EIP-1559/2930/7702 交易查询 | `yParity` | `v.Sign()` 的值（错误） | 签名 y 轴 parity bit（0或1） | **高** |
| 3 | `eth_getTransactionReceipt` | `effectiveGasPrice` | legacy/2930 交易缺失 | 所有类型必须有 | 高 |
| 4 | `eth_syncing` | `highestBlock` | 缺失 | uint（hex） | 高 |
| 5 | `eth_getLogs` / receipts 中的 logs | `blockTimestamp` | 缺失 | uint（hex） | 中 |

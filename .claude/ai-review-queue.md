# AI Code Review — Daily Module Queue

**Why:** Large repo, can't review in one pass. One module per day (weekdays), findings posted to one shared GitHub issue.

## Ranking method

Primary sort: git modification count (descending). Tiebreaker: criticality — modules that are security-sensitive or correctness-critical are promoted above their raw churn rank. This is applied consistently: `app/ante` (security gate for all txs), `x/evm/statedb` (EVM state correctness), and `ethereum/eip712` (signing security) are all promoted regardless of commit count.

## GitHub Issue

- **issue_number**: (not created yet)
- **issue_url**: (not created yet)

## Current Position

- **current_index**: 0
- **last_reviewed_date**: (not started)

## Module Queue

| # | Module | Path | Commits | Priority |
| --- | --- | --- | --- | --- |
| 0 | x/evm/types | x/evm/types | 1121 | critical |
| 1 | x/evm/keeper | x/evm/keeper | 1000 | critical |
| 2 | x/evm/statedb | x/evm/statedb | 117 | critical (promoted: EVM state correctness) |
| 3 | app/ante | app/ante | 557 | critical (promoted: security gate for all txs) |
| 4 | ethereum/eip712 | ethereum/eip712 | 52 | critical (promoted: signing security) |
| 5 | rpc/backend | rpc/backend | 349 | high |
| 6 | x/evm (root files only) | x/evm (root-level .go files only, exclude keeper/, types/, statedb/ subdirectories) | 272 | high |
| 7 | ethereum/rpc | ethereum/rpc | 243 | high |
| 8 | types | types | 239 | medium |
| 9 | rpc/namespaces | rpc/namespaces | 170 | high |
| 10 | x/feemarket/keeper | x/feemarket/keeper | 154 | high |
| 11 | server | server | 140 | medium |
| 12 | crypto | crypto | 130 | high (security, stable) |
| 13 | cmd/ethermintd | cmd/ethermintd | 109 | medium |
| 14 | x/feemarket/types | x/feemarket/types | 94 | medium |
| 15 | rpc/types | rpc/types | 86 | medium |
| 16 | client | client | 81 | low |
| 17 | testutil | testutil | 51 | low |
| 18 | encoding | encoding | 31 | medium |
| 19 | indexer | indexer | 16 | medium |

## Completed Reviews

(none yet)

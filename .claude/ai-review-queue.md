# AI Code Review — Daily Module Queue

**Why:** Large repo, can't review in one pass. 4 modules per day (09:07, 14:07, 19:07, 22:07 on weekdays), all 20 modules completed in one week. Findings posted to one shared GitHub issue.

## Ranking method

Primary sort: `git log --oneline -- <path> | wc -l` per-path commit count (descending). Tiebreaker: criticality — modules that are security-sensitive or correctness-critical are promoted above their raw churn rank. Promotions are applied consistently and documented in the Priority column.

## GitHub Issue

- **issue_number**: not created yet
- **issue_url**: not created yet

## Current Position

- **current_index**: 0
- **last_reviewed_date**: (not started)

## Module Queue

| # | Module | Path | Commits | Priority |
| --- | --- | --- | --- | --- |
| 0 | x/evm/keeper | x/evm/keeper | 331 | critical |
| 1 | x/evm/types | x/evm/types | 323 | critical |
| 2 | evmd/ante | evmd/ante | 10 | critical (promoted: security gate for all txs) |
| 3 | x/evm/statedb | x/evm/statedb | 47 | critical (promoted: EVM state correctness) |
| 4 | ethereum/eip712 | ethereum/eip712 | 26 | critical (promoted: signing security) |
| 5 | x/evm (root files only) | x/evm (root-level .go files only, exclude keeper/, types/, statedb/ subdirectories) | 124 | high |
| 6 | server | server | 110 | medium |
| 7 | types | types | 107 | medium |
| 8 | rpc/backend | rpc/backend | 99 | high |
| 9 | rpc/namespaces/ethereum | rpc/namespaces/ethereum | 54 | high |
| 10 | client | client | 98 | low |
| 11 | cmd/ethermintd | cmd/ethermintd | 66 | medium |
| 12 | rpc/namespaces | rpc/namespaces | 65 | high |
| 13 | x/feemarket/keeper | x/feemarket/keeper | 60 | high |
| 14 | crypto | crypto | 50 | high (security, stable) |
| 15 | testutil | testutil | 47 | low |
| 16 | x/feemarket/types | x/feemarket/types | 44 | medium |
| 17 | rpc/types | rpc/types | 43 | medium |
| 18 | encoding | encoding | 20 | medium |
| 19 | indexer | indexer | 12 | medium |

## Completed Reviews

(none yet)

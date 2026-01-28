# ADR 003: Batch EVM Transaction Limitations and Nonce Management

## Changelog

- 2026-01-27: Initial draft

## Status

PROPOSED, Implemented

## Abstract

This ADR documents the architectural decision to align Ethermint's EVM transaction processing with go-ethereum's behavior, particularly regarding nonce management and contract address derivation. This alignment results in unexpected and potentially dangerous behavior when processing batched EVM transactions—a Cosmos SDK-specific feature not present in Ethereum. We formally discourage the use of batched EVM transactions to prevent contract address collisions and maintain compatibility with Ethereum's execution model.

## Context

Ethermint operates at the intersection of two blockchain paradigms: Cosmos SDK's application-specific blockchain framework and Ethereum's EVM execution environment. This dual nature introduces unique capabilities and challenges.

### Cosmos SDK Batch Transactions

The Cosmos SDK allows multiple messages (transactions) to be included in a single transaction envelope. This feature enables atomic execution of multiple operations and can reduce transaction overhead. In Ethermint, this means multiple `MsgEthereumTx` messages can be batched together in a single Cosmos transaction.

### Ethereum Nonce Model

In Ethereum, each account maintains a nonce that:

1. Prevents replay attacks by ensuring transaction ordering
2. Determines contract addresses for CREATE operations via `keccak256(rlp([sender, nonce]))`
3. Increments sequentially: one increment per transaction, plus additional increments for nested contract creations

### The Ante Handler Problem

In Cosmos SDK, the ante handler processes transactions before execution. Ethermint's ante handler increments account nonces for **all messages in a batch upfront** before EVM execution begins. For a batch with 3 messages from the same sender:

- **Before ante handler:** Account nonce = N
- **After ante handler:** Account nonce = N+3

### EIP-7702 Self-Delegation Complexity

EIP-7702 allows Externally Owned Accounts (EOAs) to delegate their code execution to smart contracts. When an EOA self-delegates (authorizes delegation to itself in the same transaction), the authorization processing increments the nonce before the EVM call executes. This adds another layer of nonce management complexity:

- Authorization processing: +1 nonce per authorization
- EVM transaction execution: +1 nonce
- Nested contract creations: +N nonces (one per CREATE opcode)

In batched transactions with self-delegation, calculating the correct nonce for contract address derivation becomes increasingly complex and error-prone.

### Incompatibility with Ethereum's Model

Critically, Ethereum and go-ethereum **do not support batch transactions**. The concept of processing multiple EVM transactions atomically in a single block does not exist in the Ethereum execution model. Therefore, any behavior specific to batched EVM transactions:

1. Cannot be tested against Ethereum for correctness
2. May violate developer expectations based on Ethereum tooling
3. Could lead to contract address derivation that differs from what tools like Hardhat, Foundry, or web3.js predict

## Decision

We will align Ethermint's EVM transaction processing with go-ethereum's behavior and **strongly discourage the use of batched EVM transactions**. Specifically:

1. **Nonce Management Alignment**: The EVM state transition layer will implement nonce reset-and-reconcile logic that matches go-ethereum's behavior for single transactions.

2. **Documentation**: We will explicitly document that batched EVM transactions are not recommended and may produce unexpected results, particularly regarding contract address derivation.

3. **CREATE Transaction Handling**: For CREATE transactions, we will:
   - Reset the stateDB nonce to `msg.Nonce` before calling `evm.Create()`
   - Allow `evm.Create()` to increment the nonce internally (as go-ethereum does)

4. **CALL Transaction Handling**: For CALL transactions (including EIP-7702 delegated calls):
   - Process authorizations first (which may increment nonces)
   - Allow `evm.Call()` to increment the nonce internally (as go-ethereum does)

5. **No Batch Transaction Optimizations**: We will not add special handling or optimizations for batched EVM transactions, as such patterns do not exist in Ethereum and cannot be validated against the reference implementation.

## Consequences

### Backwards Compatibility

This change introduces a breaking change in nonce management behavior:

- **Single EVM transactions**: Behavior now matches go-ethereum exactly. Previously deployed contracts and existing single-transaction workflows are unaffected.

- **Batched EVM transactions**: Existing batched EVM transaction patterns may result in unexpected contract addresses created, particularly those involving:
    - Multiple contract deployments in a single batch (EVM Create)
    - Nested contract creations across batched messages with self-authorization (EVM Call)

### Positive

1. **Ethereum Compatibility**: Complete alignment with go-ethereum's nonce management ensures that developer tools, contract address calculations, and execution behavior match Ethereum mainnet.

2. **Predictable Contract Addresses**: Contract addresses can be reliably predicted using standard Ethereum tools (ethers.js, web3.py, etc.) without needing to understand Cosmos-specific batch transaction semantics.

3. **Reduced Complexity**: Removing support for edge cases in batched transactions simplifies the state transition logic and reduces the surface area for bugs.

4. **Testing Confidence**: All EVM behavior can be validated against go-ethereum and Geth, ensuring correctness through battle-tested reference implementations.

5. **Developer Experience**: Developers familiar with Ethereum can use Ethermint without learning Cosmos-specific batch transaction patterns.

### Negative

1. **Cosmos Feature Underutilization**: The Cosmos SDK's native support for multi-message transactions cannot be fully leveraged for EVM operations.

## Examples of Batch Transaction Issues

The following examples illustrate why batched EVM transactions produce unexpected behavior:

### Example 1: Batch EVM CREATE Transactions with Nested Creates

When multiple CREATE transactions are batched, nonce resets can cause address collisions:

| Transaction | Initial Nonce | Contracts Created | Nonces Used | Issue |
|-------------|---------------|-------------------|-------------|-------|
| tx0 (msg.Nonce=0) | 0 | 2 (1 parent + 1 nested) | 0, 1 | Reset to 0, creates at nonce 0 and 1 |
| tx1 (msg.Nonce=1) | 1 (after tx0) | 1 | 1 | Reset to 1, creates at nonce 1 (collision!) |

**Result**: Both transactions attempt to create a contract at an address derived from nonce 1, causing a conflict.

### Example 2: Batch EIP-7702 Self-Delegated Nested CREATE Transactions

Assuming the sender has delegated to a factory contract and submits a batch of 3 transactions:

| Transaction | msg.Nonce | Post-Ante Nonce | Auth Nonce | CREATE Nonce | Expected |
|-------------|-----------|-----------------|------------|--------------|----------|
| tx0 | 0 | 3 | N/A (pre-delegated) | 3 | Address from nonce 3 |
| tx1 | 1 | 3 | N/A (pre-delegated) | 4 | Address from nonce 4 |
| tx2 | 2 | 3 | N/A (pre-delegated) | 5 | Address from nonce 5 |

**Result**: All three transactions see the same post-ante-handler nonce (3) initially, requiring complex reconciliation logic that may not match Ethereum semantics.

### Example 3: Self EIP-7702 Authorization in Batch

When self-authorizations are included in batched transactions, the authorization nonce calculation becomes ambiguous:

| Transaction | msg.Nonce | Post-Ante Nonce | Expected Auth Nonce | Actual Behavior |
|-------------|-----------|-----------------|---------------------|-----------------|
| tx0 | 0 | 3 | 3 (after all ante increments) | Depends on implementation |
| tx1 | 1 | 3 | 4 | Depends on implementation |
| tx2 | 2 | 3 | 5 | Depends on implementation |

**Result**: The correct authorization nonce is unclear and cannot be validated against Ethereum, which has no concept of batched authorizations.

## Further Discussions

### Future Considerations

1. **Batch Transaction Implementation**: How should we implement batch EVM transaction logic, or should we allow them at all?

## References

- [EIP-7702: Set EOA account code](https://eips.ethereum.org/EIPS/eip-7702)
- [go-ethereum state transition](https://github.com/ethereum/go-ethereum/blob/master/core/state_transition.go)

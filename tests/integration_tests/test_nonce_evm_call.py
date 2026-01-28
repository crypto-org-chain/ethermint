"""
Integration test for EIP-7702 nonce management in nested CREATE operations.

This test verifies the fix for incorrect contract address derivation when CREATE
opcodes are executed within the Call branch of ApplyMessageWithConfig. The bug
occurs because AnteHandler pre-increments nonces for all messages before execution,
but the Call branch needs to reset the nonce before executing CREATE opcodes.

Background:
- AnteHandler increments nonces for all messages in a batch (N -> N+3)
- Each message should use its original nonce (msg.Nonce) for contract deployments
- Without the fix: All CREATEs use wrong address (post-batch nonce)
- With the fix: Each CREATE uses correct address (msg.Nonce)

Test Coverage:
1. Single transaction with CREATE
2. Batched transactions with CREATEs
3. Single transaction with self-authorization + CREATE
4. Batched transactions with self-authorizations + CREATEs
"""

from pathlib import Path

import pytest
from hexbytes import HexBytes
from web3 import Web3

from .eip7702 import (
    address_to_delegation,
    generate_signed_auth,
    setup_eip7702_delegation,
)
from .network import setup_custom_ethermint
from .utils import (
    CONTRACTS,
    build_batch_tx,
    contract_address,
    deploy_contract,
    derive_new_account,
    fund_acc,
    w3_wait_for_new_blocks,
)


@pytest.fixture(scope="module")
def custom_ethermint(tmp_path_factory):
    path = tmp_path_factory.mktemp("nonce_evm_call")
    yield from setup_custom_ethermint(
        path, 26600, Path(__file__).parent / "configs/default.jsonnet"
    )


@pytest.fixture(scope="module", params=["ethermint", "geth"])
def cluster(request, custom_ethermint, geth):
    """Run tests on both ethermint and geth."""
    provider = request.param
    if provider == "ethermint":
        yield custom_ethermint
    elif provider == "geth":
        yield geth
    else:
        raise NotImplementedError


def setup_contract_and_eip7702(w3, deployer_n, delegator_n):
    """
    Common setup for EIP-7702 tests.

    Steps:
    1. Deploy DelegationTarget contract
    2. Create and fund delegator EOA
    3. Set up EIP-7702 delegation
    4. Verify delegation is correct

    Returns:
        tuple: (delegation_target, delegator, initial_nonce, post_del_nonce)
    """
    # Deploy the DelegationTarget contract
    deployer = derive_new_account(n=deployer_n)
    fund_acc(w3, deployer)

    delegation_target, _ = deploy_contract(
        w3, CONTRACTS["DelegationTarget"], key=deployer.key
    )
    delegate_addr = delegation_target.address
    print(f"✓ DelegationTarget deployed at: {delegate_addr}")

    # Create an EOA that will be delegated to DelegationTarget
    delegator = derive_new_account(n=delegator_n)
    fund_acc(w3, delegator)

    w3_wait_for_new_blocks(w3, 1)

    initial_nonce = w3.eth.get_transaction_count(delegator.address)
    print(f"✓ Delegator initial nonce: {initial_nonce}")

    # Set up EIP-7702 delegation
    print("Setting up EIP-7702 delegation...")
    setup_eip7702_delegation(w3, delegator, delegate_addr)

    w3_wait_for_new_blocks(w3, 1)

    # Verify delegation was set correctly
    delegator_code = w3.eth.get_code(delegator.address, "latest")
    expected_delegation = address_to_delegation(delegate_addr)
    print(f"✓ Delegator code: {Web3.to_hex(delegator_code)}")
    print(f"✓ Expected delegation: {expected_delegation}")

    assert delegator_code == HexBytes(expected_delegation), (
        f"Delegation not set correctly!\n"
        f"  Got:      {Web3.to_hex(delegator_code)}\n"
        f"  Expected: {expected_delegation}"
    )
    print("✓ Delegation verified successfully!")

    # Verify nonce was incremented by setcode tx (nonce+1 for tx, +1 for auth)
    post_delegation_nonce = w3.eth.get_transaction_count(delegator.address)
    expected_post_delegation = initial_nonce + 2  # +1 tx, +1 auth
    print(
        f"✓ Nonce after delegation: {post_delegation_nonce} "
        f"(expected: {expected_post_delegation})"
    )
    assert post_delegation_nonce == expected_post_delegation, (
        f"Nonce after delegation should be {expected_post_delegation}, "
        f"got {post_delegation_nonce}"
    )

    w3_wait_for_new_blocks(w3, 1)

    return delegation_target, delegator, initial_nonce, post_delegation_nonce


def test_single_tx_create(cluster):
    """
    Test 1: Single transaction with CREATE operation.

    Verifies that a single contract deployment through EIP-7702 delegated code
    uses the correct msg.Nonce for address derivation.

    Step 1: Verify delegation is set up correctly (via common setup)
    Step 2: Verify CREATE operation uses correct nonce
    """
    w3: Web3 = cluster.w3

    # STEP 1: Set up delegation (common setup)
    delegation_target, delegator, initial_nonce, post_delegation_nonce = (
        setup_contract_and_eip7702(w3, deployer_n=100, delegator_n=101)
    )

    # STEP 2: Now test CREATE operation
    # evm.Call increments nonce by 1 before execution, so any contract
    # deployment embedded within the call will use the nonce+1
    deploy_nonce = w3.eth.get_transaction_count(delegator.address)
    print(
        f"\n✓ Starting evm.Call (with contract creation) test "
        f"with nonce: {deploy_nonce}"
    )

    # Pre-calculate expected child contract address
    expected_child_addr = contract_address(delegator.address, deploy_nonce + 1)
    print(
        f"✓ Expected child contract at: {expected_child_addr} "
        f"(nonce={deploy_nonce + 1})"
    )

    # Use MinimalContract bytecode - simple contract that has runtime code
    # This is the creation code that returns 1 byte of runtime (STOP opcode)
    minimal_bytecode = "0x6001600C60003960016000F300"  # Returns 1 byte: 0x00 (STOP)

    deploy_data = delegation_target.functions.deploy(
        HexBytes(minimal_bytecode)
    ).build_transaction({"gas": 0})["data"]

    tx = {
        "chainId": w3.eth.chain_id,
        "from": delegator.address,
        "to": delegator.address,  # Self-call to trigger delegated code
        "value": 0,
        "gas": 500000,
        "maxFeePerGas": 1000000000000,
        "maxPriorityFeePerGas": 10000,
        "nonce": deploy_nonce,
        "data": deploy_data,
    }

    print("✓ Sending deployment transaction...")
    signed_tx = delegator.sign_transaction(tx)
    tx_hash = w3.eth.send_raw_transaction(signed_tx.raw_transaction)
    receipt = w3.eth.wait_for_transaction_receipt(tx_hash, timeout=30)

    assert receipt.status == 1, f"Deployment transaction failed: {receipt}"
    print(f"✓ Deployment tx succeeded, gas used: {receipt.gasUsed}")

    w3_wait_for_new_blocks(w3, 1)

    # CRITICAL VERIFICATION: Check that child was deployed at correct address
    child_code = w3.eth.get_code(expected_child_addr, "latest")
    has_code = len(child_code) > 0
    print(
        f"✓ Child at {expected_child_addr}: hasCode={has_code} "
        f"(codeLen={len(child_code)})"
    )

    assert has_code, (
        f"Child should be deployed at {expected_child_addr} (nonce={deploy_nonce}), "
        f"but no code found."
    )

    # Verify delegator's final nonce
    final_nonce = w3.eth.get_transaction_count(delegator.address)
    expected_final_nonce = deploy_nonce + 2  # +1 for tx, +1 for create embedded in call
    print(f"✓ Delegator final nonce: {final_nonce} (expected: {expected_final_nonce})")

    assert (
        final_nonce == expected_final_nonce
    ), f"Delegator nonce should be {expected_final_nonce}, got {final_nonce}"

    print("\n✅ All checks passed! Child deployed at correct address")


def test_batched_tx_create(cluster):
    """
    Test 2: Batched transactions with CREATE operations.

    Verifies that multiple contract deployments in a batch transaction each use
    their respective msg.Nonce (not the post-batch nonce) for address derivation.

    Step 1: Verify delegation is set up correctly (via common setup)
    Step 2: Verify batched CREATE operations use correct nonces

    NOTE: This test only runs on Ethermint (batch transactions are Cosmos-specific).
    """
    # Skip this test for geth (batch transactions are Cosmos-specific)
    if not hasattr(cluster, "cosmos_cli"):
        pytest.skip("Batch transactions only supported on Ethermint")

    w3: Web3 = cluster.w3
    cli = cluster.cosmos_cli()

    # STEP 1: Set up delegation (common setup)
    delegation_target, delegator, initial_nonce, post_delegation_nonce = (
        setup_contract_and_eip7702(w3, deployer_n=110, delegator_n=111)
    )

    # STEP 2: Now test batched CREATE operations
    # Get nonce before batch
    batch_start_nonce = w3.eth.get_transaction_count(delegator.address)
    print(f"\n✓ Starting batched CREATE test with nonce: {batch_start_nonce}")

    # Pre-calculate expected child contract addresses

    # AnteHandler increments nonces for all messages upfront
    num_messages = 3
    expected_child_addresses = []
    for i in range(num_messages):
        msg_nonce = batch_start_nonce + i
        create_nonce = batch_start_nonce + num_messages + i
        child_addr = contract_address(delegator.address, create_nonce)
        expected_child_addresses.append(child_addr)
        print(
            f"✓ Expected child {i} at create_nonce {create_nonce} "
            f"(msg_nonce={msg_nonce}): {child_addr}"
        )

    # Build batch transaction: each deploys a contract
    # Use MinimalContract bytecode - simple contract that has runtime code
    minimal_bytecode = "0x6001600C60003960016000F300"  # Returns 1 byte: 0x00 (STOP)
    deploy_data = delegation_target.functions.deploy(
        HexBytes(minimal_bytecode)
    ).build_transaction({"gas": 0})["data"]

    batch_txs = []
    for i in range(num_messages):
        msg_nonce = batch_start_nonce + i  # Sequential nonces for batch
        tx = {
            "chainId": w3.eth.chain_id,
            "from": delegator.address,
            "to": delegator.address,  # Self-call to trigger delegated code
            "value": 0,
            "gas": 500000,
            "maxFeePerGas": 1000000000000,
            "maxPriorityFeePerGas": 10000,
            "nonce": msg_nonce,
            "data": deploy_data,
        }
        batch_txs.append(tx)
        print(f"✓ Batch tx {i}: nonce={msg_nonce}")

    # Build and send batch Cosmos transaction
    batch_cosmos_tx, eth_tx_hashes = build_batch_tx(
        w3, cli, batch_txs, key=delegator.key
    )

    # Broadcast the batch transaction
    rsp = cli.broadcast_tx_json(batch_cosmos_tx)
    assert rsp["code"] == 0, f"Batch broadcast failed: {rsp.get('raw_log', rsp)}"
    print("Batch transaction broadcast successful")

    # Wait for transactions to be included and get receipts
    receipts = [
        w3.eth.wait_for_transaction_receipt(h, timeout=30) for h in eth_tx_hashes
    ]

    # Verify all transactions succeeded
    for i, receipt in enumerate(receipts):
        assert receipt.status == 1, f"Transaction {i} failed: {receipt}"
        print(f"Transaction {i} succeeded, gas used: {receipt.gasUsed}")

    w3_wait_for_new_blocks(w3, 1)

    # CRITICAL VERIFICATION: Check that children were deployed at correct addresses
    print("\n✓ Verifying contract deployments:")
    for i, expected_addr in enumerate(expected_child_addresses):
        code = w3.eth.get_code(expected_addr, "latest")
        has_code = len(code) > 0
        print(
            f"  ✓ Child {i} at {expected_addr}: "
            f"hasCode={has_code} (codeLen={len(code)})"
        )

        msg_nonce = batch_start_nonce + i
        create_nonce = msg_nonce + 1
        assert has_code, (
            f"Child {i} should be deployed at {expected_addr} "
            f"(msg_nonce={msg_nonce}, create_nonce={create_nonce}), "
            f"but no code found."
        )

    # Verify delegator's final nonce
    # AnteHandler increments nonce by num_messages (for the batch)
    # Each CREATE increments nonce by 1 more during execution
    # Total: batch_start_nonce + num_messages (ante) + nested_creates (CREATEs)
    final_nonce = w3.eth.get_transaction_count(delegator.address)
    nested_creates = num_messages
    expected_final_nonce = batch_start_nonce + num_messages + nested_creates
    print(
        f"\n✓ Delegator final nonce: {final_nonce} (expected: {expected_final_nonce})"
    )

    assert (
        final_nonce == expected_final_nonce
    ), f"Delegator nonce should be {expected_final_nonce}, got {final_nonce}"

    print("\n✅ All checks passed! All children deployed at correct addresses")


def test_single_tx_with_self_authorization_create(cluster):
    """
    Test 3: Single transaction with self-authorization and CREATE.

    Similar to test_single_tx_create_nonce but includes SetCodeAuthorization
    in the same transaction. Verifies that the CREATE operation still uses the
    correct msg.Nonce even when authorization is attached.

    The authorization happens in the same transaction (no pre-setup needed).

    Step 1: Deploy DelegationTarget contract
    Step 2: Create and fund delegator EOA
    Step 3: Send transaction with authorization AND CREATE operation
    Step 4: Verify CREATE uses correct nonce
    """
    w3: Web3 = cluster.w3

    # STEP 1: Deploy the DelegationTarget contract
    deployer = derive_new_account(n=120)
    fund_acc(w3, deployer)

    delegation_target, _ = deploy_contract(
        w3, CONTRACTS["DelegationTarget"], key=deployer.key
    )
    print(f"✓ DelegationTarget deployed at: {delegation_target.address}")

    # STEP 2: Create and fund delegator EOA (no pre-delegation setup)
    delegator = derive_new_account(n=121)
    fund_acc(w3, delegator)

    w3_wait_for_new_blocks(w3, 1)

    initial_nonce = w3.eth.get_transaction_count(delegator.address)
    print(f"✓ Delegator initial nonce: {initial_nonce}")

    w3_wait_for_new_blocks(w3, 1)

    # STEP 3: Now test CREATE with self-authorization attached
    deploy_nonce = w3.eth.get_transaction_count(delegator.address)
    print(f"\n✓ Starting CREATE with self-authorization test, nonce: {deploy_nonce}")

    # Pre-calculate expected child contract address
    # Authorizations are processed first: +1 nonce increment
    # Then tx execution (evm call): +1 nonce increment
    # So CREATE uses: deploy_nonce + 1 + 1 = deploy_nonce + 2
    expected_child_addr = contract_address(delegator.address, deploy_nonce + 2)
    print(
        f"✓ Expected child contract at: {expected_child_addr} "
        f"(nonce={deploy_nonce + 2})"
    )

    # Use MinimalContract bytecode
    minimal_bytecode = "0x6001600C60003960016000F300"  # Returns 1 byte: 0x00 (STOP)

    # Build transaction data for CREATE via delegated call
    deploy_data = delegation_target.functions.deploy(
        HexBytes(minimal_bytecode)
    ).build_transaction({"gas": 0})["data"]

    # Generate signed authorizations using helper
    chain_id = w3.eth.chain_id

    # First auth: to delegation_target (to enable delegation for CREATE)
    auth1_nonce = deploy_nonce + 1
    signed_auth1 = generate_signed_auth(
        w3, delegator, delegation_target.address, auth1_nonce
    )
    print(
        f"✓ First authorization to {delegation_target.address} with nonce {auth1_nonce}"
    )

    # Build EIP-7702 transaction with authorizations and CREATE
    setcode_tx = {
        "chainId": chain_id,
        "type": 4,
        "to": delegator.address,  # Self-call to trigger delegated code
        "value": 0,
        "gas": 500000,
        "maxFeePerGas": 1000000000000,
        "maxPriorityFeePerGas": 10000,
        "nonce": deploy_nonce,
        "data": deploy_data,
        "authorizationList": [signed_auth1],
    }

    print("✓ Sending transaction with self-authorization...")
    signed_tx = delegator.sign_transaction(setcode_tx)
    tx_hash = w3.eth.send_raw_transaction(signed_tx.raw_transaction)
    receipt = w3.eth.wait_for_transaction_receipt(tx_hash, timeout=30)

    assert receipt.status == 1, f"Transaction failed: {receipt}"
    print(f"✓ Transaction succeeded, gas used: {receipt.gasUsed}")

    w3_wait_for_new_blocks(w3, 1)

    # STEP 5: Verify delegation was set to delegation_target
    delegator_code = w3.eth.get_code(delegator.address, "latest")
    expected_delegation = address_to_delegation(delegation_target.address)
    assert delegator_code == HexBytes(
        expected_delegation
    ), f"Delegation should be set to {delegation_target.address}"
    print("✓ Delegation set to delegation_target")

    w3_wait_for_new_blocks(w3, 1)

    # CRITICAL VERIFICATION: Check that child was deployed at correct address
    child_code = w3.eth.get_code(expected_child_addr, "latest")
    has_code = len(child_code) > 0
    print(
        f"✓ Child at {expected_child_addr}: hasCode={has_code} "
        f"(codeLen={len(child_code)})"
    )

    assert has_code, (
        f"Child should be deployed at {expected_child_addr} "
        f"(nonce={deploy_nonce + 2}), "
        "but no code found."
    )

    # Verify delegator's final nonce
    # +1 for 1 auth, +1 for tx, +1 for CREATE = +3 total
    final_nonce = w3.eth.get_transaction_count(delegator.address)
    expected_final_nonce = deploy_nonce + 3
    print(f"✓ Delegator final nonce: {final_nonce} (expected: {expected_final_nonce})")

    assert (
        final_nonce == expected_final_nonce
    ), f"Delegator nonce should be {expected_final_nonce}, got {final_nonce}"

    print(
        "\n✅ All checks passed! "
        "Child deployed at correct address with self-authorization"
    )


def test_batched_tx_with_self_authorizations_create(cluster):
    """
    Test 4: Batched transactions with self-authorizations and CREATEs.

    Similar to test_single_tx_with_self_authorization_create but batched
    with 3 transactions. Each transaction includes SetCodeAuthorization
    to delegation_target and performs a CREATE.

    The authorizations happen in the same transactions (no pre-setup needed).

    Step 1: Deploy DelegationTarget contract
    Step 2: Create and fund delegator EOA
    Step 3: Send batch of 3 txs, each with authorization AND CREATE operation
    Step 4: Verify each CREATE uses correct nonce

    NOTE: This test only runs on Ethermint (batch transactions are Cosmos-specific).
    """
    # Skip this test for geth (batch transactions are Cosmos-specific)
    if not hasattr(cluster, "cosmos_cli"):
        pytest.skip("Batch transactions only supported on Ethermint")

    w3: Web3 = cluster.w3
    cli = cluster.cosmos_cli()

    # STEP 1: Deploy the DelegationTarget contract
    deployer = derive_new_account(n=130)
    fund_acc(w3, deployer)

    delegation_target, _ = deploy_contract(
        w3, CONTRACTS["DelegationTarget"], key=deployer.key
    )
    print(f"✓ DelegationTarget deployed at: {delegation_target.address}")

    # STEP 2: Create and fund delegator EOA (no pre-delegation setup)
    delegator = derive_new_account(n=131)
    fund_acc(w3, delegator)

    w3_wait_for_new_blocks(w3, 1)

    initial_nonce = w3.eth.get_transaction_count(delegator.address)
    print(f"✓ Delegator initial nonce: {initial_nonce}")

    w3_wait_for_new_blocks(w3, 1)

    # STEP 3: Now test batched CREATEs with self-authorizations
    batch_start_nonce = w3.eth.get_transaction_count(delegator.address)
    print(
        f"\n✓ Starting batched CREATE with self-authorizations, "
        f"nonce: {batch_start_nonce}"
    )

    # Pre-calculate expected child contract addresses
    num_messages = 3
    expected_child_addresses = []
    for i in range(num_messages):
        msg_nonce = batch_start_nonce + i
        auth_nonce = batch_start_nonce + num_messages + (i * 2)
        create_nonce = auth_nonce + 1
        child_addr = contract_address(delegator.address, create_nonce)
        expected_child_addresses.append(child_addr)
        print(
            f"✓ Expected child {i} at create_nonce {create_nonce} "
            f"(msg_nonce={msg_nonce}): {child_addr}"
        )

    # Use MinimalContract bytecode
    minimal_bytecode = "0x6001600C60003960016000F300"  # Returns 1 byte: 0x00 (STOP)
    chain_id = w3.eth.chain_id

    # Build deploy data
    deploy_data = delegation_target.functions.deploy(
        HexBytes(minimal_bytecode)
    ).build_transaction({"gas": 0})["data"]

    # Build batch transactions
    batch_txs = []
    for i in range(num_messages):
        msg_nonce = batch_start_nonce + i

        # Calculate authorization nonce:
        # After all msgs: batch_start_nonce + num_messages
        # Each previous tx consumed 2 nonces (1 auth + 1 CREATE)
        auth_nonce = batch_start_nonce + num_messages + (i * 2)

        # Generate signed authorization to delegation_target
        signed_auth = generate_signed_auth(
            w3, delegator, delegation_target.address, auth_nonce
        )
        print(
            f"✓ Batch tx {i}: msg_nonce={msg_nonce}, "
            f"auth_nonce={auth_nonce} -> {delegation_target.address}"
        )

        tx = {
            "chainId": chain_id,
            "type": 4,
            "from": delegator.address,
            "to": delegator.address,  # Self-call to trigger delegated code
            "value": 0,
            "gas": 500000,
            "maxFeePerGas": 1000000000000,
            "maxPriorityFeePerGas": 10000,
            "nonce": msg_nonce,
            "data": deploy_data,
            "authorizationList": [signed_auth],
        }
        batch_txs.append(tx)

    # Build and send batch Cosmos transaction
    print(f"✓ Building batch transaction with {num_messages} messages...")
    batch_cosmos_tx, eth_tx_hashes = build_batch_tx(
        w3, cli, batch_txs, key=delegator.key
    )

    # Broadcast the batch transaction
    rsp = cli.broadcast_tx_json(batch_cosmos_tx)
    assert rsp["code"] == 0, f"Batch broadcast failed: {rsp.get('raw_log', rsp)}"
    print("✓ Batch transaction broadcast successful")

    # Wait for transactions to be included and get receipts
    receipts = [
        w3.eth.wait_for_transaction_receipt(h, timeout=30) for h in eth_tx_hashes
    ]

    # Verify all transactions succeeded
    for i, receipt in enumerate(receipts):
        assert receipt.status == 1, f"Transaction {i} failed: {receipt}"
        print(f"✓ Transaction {i} succeeded, gas used: {receipt.gasUsed}")

    w3_wait_for_new_blocks(w3, 1)

    # STEP 4: Verify delegation was set to delegation_target
    delegator_code = w3.eth.get_code(delegator.address, "latest")
    expected_delegation = address_to_delegation(delegation_target.address)
    assert delegator_code == HexBytes(
        expected_delegation
    ), f"Delegation should be set to {delegation_target.address}"
    print("✓ Delegation set to delegation_target")

    # CRITICAL VERIFICATION: Check that children were deployed at correct addresses
    print("\n✓ Verifying contract deployments:")
    for i, expected_addr in enumerate(expected_child_addresses):
        code = w3.eth.get_code(expected_addr, "latest")
        has_code = len(code) > 0
        msg_nonce = batch_start_nonce + i
        create_nonce = msg_nonce + 1
        print(
            f"  ✓ Child {i} at {expected_addr}: hasCode={has_code} "
            f"(codeLen={len(code)})"
        )

        assert has_code, (
            f"Child {i} should be deployed at {expected_addr} "
            f"(msg_nonce={msg_nonce}, create_nonce={create_nonce}), "
            "but no code found."
        )

    # Verify delegator's final nonce
    # AnteHandler: +num_messages (for batch txs)
    # CREATEs: +num_messages (one per tx)
    # Auths: +num_messages (one per tx)
    # Total: batch_start_nonce + num_messages + num_messages + num_messages
    final_nonce = w3.eth.get_transaction_count(delegator.address)
    nested_creates = num_messages
    auths = num_messages  # 1 auth per message
    expected_final_nonce = batch_start_nonce + num_messages + nested_creates + auths
    print(
        f"\n✓ Delegator final nonce: {final_nonce} (expected: {expected_final_nonce})"
    )

    assert (
        final_nonce == expected_final_nonce
    ), f"Delegator nonce should be {expected_final_nonce}, got {final_nonce}"

    print(
        "\n✅ All checks passed! "
        "All children deployed at correct addresses with self-authorizations"
    )

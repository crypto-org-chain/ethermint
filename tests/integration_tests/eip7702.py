"""
Shared utilities for EIP-7702 (Set Code Transaction) tests.

This module contains common helper functions used across EIP-7702 test files
to avoid code duplication.
"""

from hexbytes import HexBytes
from web3 import Web3

DELEGATION_PREFIX = "0xef0100"


def address_to_delegation(address: str) -> str:
    """
    Convert an Ethereum address to EIP-7702 delegation bytecode format.

    Args:
        address: Ethereum address (0x-prefixed hex string)

    Returns:
        Delegation bytecode string with format: 0xef0100{address_without_0x}
    """
    return DELEGATION_PREFIX + address[2:]


def generate_signed_auth(w3, acc, delegate_addr, nonce):
    """
    Generate a signed EIP-7702 authorization.

    Args:
        w3: Web3 instance
        acc: Account object with sign_authorization method
        delegate_addr: Address to delegate to
        nonce: Nonce for the authorization

    Returns:
        Signed authorization object
    """
    chain_id = w3.eth.chain_id
    auth = {
        "chainId": chain_id,
        "address": delegate_addr,
        "nonce": nonce,
    }
    return acc.sign_authorization(auth)


def send_setcode_tx(w3, sender_acc, to, signed_auth, gas=100000, data=None):
    """
    Send an EIP-7702 setcode transaction.

    Args:
        w3: Web3 instance
        sender_acc: Sender account
        to: Target address
        signed_auth: Signed authorization
        gas: Gas limit (default: 100000)
        data: Optional calldata for the transaction

    Returns:
        Transaction receipt
    """
    setcode_tx = {
        "chainId": w3.eth.chain_id,
        "type": 4,
        "to": to,
        "value": 0,
        "gas": gas,
        "maxFeePerGas": 1000000000000,
        "maxPriorityFeePerGas": 10000,
        "nonce": w3.eth.get_transaction_count(sender_acc.address),
        "authorizationList": [signed_auth],
    }

    if data is not None:
        setcode_tx["data"] = data

    signed_tx = sender_acc.sign_transaction(setcode_tx)
    tx_hash = w3.eth.send_raw_transaction(signed_tx.raw_transaction)
    receipt = w3.eth.wait_for_transaction_receipt(tx_hash, timeout=30)
    return receipt


def setup_eip7702_delegation(w3, delegator, delegate_addr, gas=500000):
    """
    Set up EIP-7702 delegation for an account.

    This is a convenience function that creates and sends a setcode transaction
    to delegate an EOA to a contract address. Uses generate_signed_auth and
    send_setcode_tx internally.

    Args:
        w3: Web3 instance
        delegator: Account to be delegated (sender == auth account)
        delegate_addr: Address of contract to delegate to
        gas: Gas limit for the transaction (default: 500000)

    Returns:
        Transaction receipt

    Raises:
        AssertionError: If delegation setup fails or verification fails
    """
    nonce = w3.eth.get_transaction_count(delegator.address)

    # When sender == auth account, auth nonce must be current + 1
    signed_auth = generate_signed_auth(w3, delegator, delegate_addr, nonce + 1)

    # Send the setcode transaction with higher gas limit
    receipt = send_setcode_tx(w3, delegator, delegator.address, signed_auth, gas=gas)

    assert receipt.status == 1, f"SetCode transaction failed: {receipt}"

    # Verify delegation was set correctly
    delegator_code = w3.eth.get_code(delegator.address, receipt.blockNumber)
    expected_delegation = address_to_delegation(delegate_addr)
    assert delegator_code == HexBytes(expected_delegation), (
        f"Delegation not set: got {Web3.to_hex(delegator_code)}, "
        f"want {expected_delegation}"
    )

    return receipt

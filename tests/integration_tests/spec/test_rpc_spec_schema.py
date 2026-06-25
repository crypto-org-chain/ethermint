import json
import sys
from pathlib import Path

import pytest
from _schema_constants import (
    BLOB_VERSIONED_HASH,
    EXCLUDED_SCHEMA_SPEC_CASES,
    LEGACY_CREATE_BYTECODE,
    REPORT_FILENAME,
    UNIMPLEMENTED_RPC_METHODS,
)
from _schema_normalize import _attach_details, _classify_schema
from _schema_report import (
    RpcSpecSchemaSummary,
    _expected_failure_drift,
    _schema_mismatches,
)
from _schema_rewrite import _format_case_context, _prepare_schema_request
from eth_account import Account
from test_rpc_spec import SPEC_FILES, _is_not_implemented, _send_rpc
from web3 import Web3


def _parse_spec_interactions(spec_name):
    filepath = Path(__file__).parent / f"{spec_name}.io"
    request_line = None
    comments = []
    interactions = []
    with filepath.open() as f:
        for line in f:
            line = line.strip()
            if line.startswith("//"):
                comments.append(line[2:].strip())
            elif line.startswith(">> "):
                request_line = line[3:]
            elif line.startswith("<< "):
                assert request_line, f"response without request in {spec_name}.io"
                interactions.append((request_line, line[3:]))
                request_line = None

    return interactions, comments


def _eip1559_fees(w3):
    latest = w3.eth.get_block("latest")
    base_fee = int(latest.get("baseFeePerGas") or w3.eth.gas_price)
    max_priority_fee = 10000
    max_fee = max(base_fee * 2 + max_priority_fee, int(w3.eth.gas_price) * 2)
    return max_fee, max_priority_fee


def _tx_hash(receipt):
    return Web3.to_hex(receipt.transactionHash)


def _assert_successful_receipt(receipt):
    assert receipt.status == 1, f"transaction failed: {receipt}"
    return receipt


def _send_dynamic_fee_transaction(w3, to, key, *, access_list=None):
    account = Account.from_key(key)
    max_fee, max_priority_fee = _eip1559_fees(w3)
    return _assert_successful_receipt(
        w3.eth.wait_for_transaction_receipt(
            w3.eth.send_raw_transaction(
                account.sign_transaction(
                    {
                        "chainId": w3.eth.chain_id,
                        "type": 2,
                        "to": to,
                        "value": 1,
                        "gas": 100000,
                        "maxFeePerGas": max_fee,
                        "maxPriorityFeePerGas": max_priority_fee,
                        "nonce": w3.eth.get_transaction_count(account.address),
                        "data": "0x1ee8f6de",
                        "accessList": access_list or [],
                    }
                ).raw_transaction
            ),
            timeout=30,
        )
    )


def _send_blob_transaction(w3, to, key, *, access_list=None):
    account = Account.from_key(key)
    max_fee, max_priority_fee = _eip1559_fees(w3)
    return _assert_successful_receipt(
        w3.eth.wait_for_transaction_receipt(
            w3.eth.send_raw_transaction(
                account.sign_transaction(
                    {
                        "chainId": w3.eth.chain_id,
                        "type": 3,
                        "to": to,
                        "value": 1,
                        "gas": 100000,
                        "maxFeePerGas": max_fee,
                        "maxPriorityFeePerGas": max_priority_fee,
                        "maxFeePerBlobGas": 1,
                        "blobVersionedHashes": [BLOB_VERSIONED_HASH],
                        "nonce": w3.eth.get_transaction_count(account.address),
                        "data": "0x29db6825",
                        "accessList": access_list or [],
                    }
                ).raw_transaction
            ),
            timeout=30,
        )
    )


def _send_setcode_transaction(w3, sender, delegate):
    nonce = w3.eth.get_transaction_count(sender.address)
    max_fee, max_priority_fee = _eip1559_fees(w3)
    signed_auth = sender.sign_authorization(
        {
            "chainId": w3.eth.chain_id,
            "address": delegate.address,
            "nonce": nonce + 1,
        }
    )
    signed_tx = sender.sign_transaction(
        {
            "chainId": w3.eth.chain_id,
            "type": 4,
            "to": sender.address,
            "value": 0,
            "gas": 100000,
            "maxFeePerGas": max_fee,
            "maxPriorityFeePerGas": max_priority_fee,
            "nonce": nonce,
            "accessList": [],
            "authorizationList": [signed_auth],
        }
    )
    return _assert_successful_receipt(
        w3.eth.wait_for_transaction_receipt(
            w3.eth.send_raw_transaction(signed_tx.raw_transaction),
            timeout=30,
        )
    )


def _sign_raw_transaction(w3, account, tx):
    signed = account.sign_transaction(
        {
            **tx,
            "chainId": w3.eth.chain_id,
            "nonce": w3.eth.get_transaction_count(account.address),
        }
    )
    return Web3.to_hex(signed.raw_transaction)


def _build_send_raw_transactions(w3, to, accounts, access_list):
    max_fee, max_priority_fee = _eip1559_fees(w3)
    return {
        "access_list": _sign_raw_transaction(
            w3,
            accounts["access_list"],
            {
                "type": 1,
                "to": to,
                "value": 1,
                "gas": 100000,
                "gasPrice": w3.eth.gas_price,
                "accessList": access_list,
            },
        ),
        "blob": _sign_raw_transaction(
            w3,
            accounts["blob"],
            {
                "type": 3,
                "to": to,
                "value": 1,
                "gas": 100000,
                "maxFeePerGas": max_fee,
                "maxPriorityFeePerGas": max_priority_fee,
                "maxFeePerBlobGas": 1,
                "blobVersionedHashes": [BLOB_VERSIONED_HASH],
                "data": "0x29db6825",
                "accessList": access_list,
            },
        ),
        "dynamic_fee_access_list": _sign_raw_transaction(
            w3,
            accounts["dynamic_fee_access_list"],
            {
                "type": 2,
                "to": to,
                "value": 1,
                "gas": 100000,
                "maxFeePerGas": max_fee,
                "maxPriorityFeePerGas": max_priority_fee,
                "data": "0x1ee8f6de",
                "accessList": access_list,
            },
        ),
        "dynamic_fee_create": _sign_raw_transaction(
            w3,
            accounts["dynamic_fee_create"],
            {
                "type": 2,
                "value": 0,
                "gas": 100000,
                "maxFeePerGas": max_fee,
                "maxPriorityFeePerGas": max_priority_fee,
                "data": LEGACY_CREATE_BYTECODE,
            },
        ),
        "legacy": _sign_raw_transaction(
            w3,
            accounts["legacy"],
            {
                "to": to,
                "value": 1,
                "gas": 100000,
                "gasPrice": w3.eth.gas_price,
            },
        ),
    }


@pytest.fixture(scope="module")
def rpc_context(rpc_endpoint, ethermint):
    sys.path.append(str(Path(__file__).parents[1]))
    from utils import ADDRS, KEYS, derive_new_account, fund_acc, send_transaction

    w3 = Web3(Web3.HTTPProvider(rpc_endpoint))
    access_list = [
        {
            "address": ADDRS["community"],
            "storageKeys": [
                "0x0000000000000000000000000000000000000000000000000000000000000000"
            ],
        }
    ]
    legacy_receipt = _assert_successful_receipt(
        send_transaction(
            w3,
            {"to": ADDRS["community"], "value": 1, "gasPrice": w3.eth.gas_price},
            KEYS["validator"],
        )
    )
    legacy_create_receipt = _assert_successful_receipt(
        send_transaction(
            w3,
            {
                "value": 0,
                "gas": 100000,
                "gasPrice": w3.eth.gas_price,
                "data": LEGACY_CREATE_BYTECODE,
            },
            KEYS["validator"],
        )
    )
    access_list_receipt = _assert_successful_receipt(
        send_transaction(
            w3,
            {
                "to": ADDRS["community"],
                "value": 1,
                "gas": 100000,
                "gasPrice": w3.eth.gas_price,
                "accessList": access_list,
            },
            KEYS["validator"],
        )
    )
    dynamic_fee_receipt = _send_dynamic_fee_transaction(
        w3, ADDRS["community"], KEYS["validator"], access_list=access_list
    )
    blob_receipt = _send_blob_transaction(
        w3, ADDRS["community"], KEYS["validator"], access_list=access_list
    )

    setcode_sender = derive_new_account(n=7702)
    setcode_delegate = derive_new_account(n=7703)
    fund_acc(w3, setcode_sender)
    setcode_receipt = _send_setcode_transaction(w3, setcode_sender, setcode_delegate)

    send_raw_accounts = {
        "access_list": derive_new_account(n=7800),
        "blob": derive_new_account(n=7801),
        "dynamic_fee_access_list": derive_new_account(n=7802),
        "dynamic_fee_create": derive_new_account(n=7803),
        "legacy": derive_new_account(n=7804),
    }
    for account in send_raw_accounts.values():
        fund_acc(w3, account)
    send_raw_txs = _build_send_raw_transactions(
        w3, ADDRS["community"], send_raw_accounts, access_list
    )

    receipt = legacy_receipt
    tx_hashes = {
        "access_list": _tx_hash(access_list_receipt),
        "blob": _tx_hash(blob_receipt),
        "dynamic_fee": _tx_hash(dynamic_fee_receipt),
        "legacy": _tx_hash(legacy_receipt),
        "legacy_create": _tx_hash(legacy_create_receipt),
        "setcode": _tx_hash(setcode_receipt),
    }
    block_one = w3.eth.get_block(1)
    block_four = w3.eth.get_block(4)
    return {
        "w3": w3,
        "endpoint": rpc_endpoint,
        "report_path": ethermint.base_dir.parent / REPORT_FILENAME,
        "block_hash": Web3.to_hex(receipt.blockHash),
        "block_number": hex(receipt.blockNumber),
        "fixture_block_hashes": {
            "0x1": Web3.to_hex(block_one.hash),
            "0x4": Web3.to_hex(block_four.hash),
        },
        "future_block_number": hex(receipt.blockNumber + 1000),
        "tx_hash": _tx_hash(receipt),
        "tx_hashes": tx_hashes,
        "send_raw_txs": send_raw_txs,
    }


def _skip_followups_after_unimplemented_first_request(
    rpc_context, spec_name, interactions, comments
):
    if len(interactions) < 2:
        return None

    first_request = json.loads(interactions[0][0])
    if first_request.get("method") not in UNIMPLEMENTED_RPC_METHODS:
        return None

    request, expected, runtime_rewrite_note, rewritten = _prepare_schema_request(
        spec_name, interactions[0][0], interactions[0][1], rpc_context
    )
    actual = _send_rpc(rpc_context["endpoint"], json.dumps(request))
    if not _is_not_implemented(actual):
        return None

    result = _classify_schema(spec_name, request, expected, actual)
    context = _format_case_context(
        comments,
        runtime_rewrite_note,
        rewritten,
        "skipped dependent requests because first request was not implemented",
    )
    return _attach_details(result, request, expected, actual, context)


def _run_spec_case(rpc_context, spec_name):
    interactions, comments = _parse_spec_interactions(spec_name)
    assert interactions, f"no request/response pair in {spec_name}.io"

    skipped_result = _skip_followups_after_unimplemented_first_request(
        rpc_context, spec_name, interactions, comments
    )
    if skipped_result is not None:
        return skipped_result

    request, expected, runtime_rewrite_note, rewritten = _prepare_schema_request(
        spec_name, interactions[-1][0], interactions[-1][1], rpc_context
    )
    actual = _send_rpc(rpc_context["endpoint"], json.dumps(request))

    result = _classify_schema(spec_name, request, expected, actual)
    context = _format_case_context(comments, runtime_rewrite_note, rewritten)
    return _attach_details(result, request, expected, actual, context)


def test_ethermint_rpc_matches_execution_api_schema(rpc_context):
    summary = RpcSpecSchemaSummary()
    for spec_name in SPEC_FILES:
        if spec_name in EXCLUDED_SCHEMA_SPEC_CASES:
            continue
        summary.add(_run_spec_case(rpc_context, spec_name))

    report = summary.report()
    report_path = rpc_context["report_path"]
    report_path.write_text(report)

    print("")
    print(summary.format())
    print("")
    print(f"wrote schema report: {report_path}")

    schema_mismatches = _schema_mismatches(summary)
    assert not schema_mismatches, (
        "RPC schema mismatches detected:\n"
        + "\n".join(f"- {line}" for line in schema_mismatches)
        + f"\nSee detailed report: {report_path}"
    )

    drift = _expected_failure_drift(summary)
    assert not drift, (
        "RPC schema expected-failure whitelist drifted:\n"
        + "\n".join(f"- {line}" for line in drift)
        + f"\nSee detailed report: {report_path}"
    )

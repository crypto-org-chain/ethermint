import json
from copy import deepcopy
from collections import defaultdict
from pathlib import Path

from eth_account import Account
import pytest
from web3 import Web3

from test_rpc_spec import (
    SPEC_FILES,
    CATEGORY_TITLES,
    RpcSpecResult,
    _first_schema_mismatch,
    _is_not_implemented,
    _is_request_schema_error,
    _markdown_json,
    _rewrite_request_for_ethermint_runtime_fixture,
    _response_kind,
    _same_schema,
    _send_rpc,
    rpc_endpoint,
)

REPORT_FILENAME = "rpc_schema_report.md"
SCHEMA_CATEGORY_TITLES = {
    **CATEGORY_TITLES,
    "schema_correct": "correct implemented by schema",
}
UNIMPLEMENTED_RPC_METHODS = {
    "debug_getRawBlock",
    "debug_getRawHeader",
    "debug_getRawReceipts",
    "debug_getRawTransaction",
    "eth_blobBaseFee",
    "eth_config",
    "eth_getStorageValues",
    "testing_buildBlockV1",
    "txpool_content",
    "txpool_contentFrom",
}
INCOMPLETE_UNIMPLEMENTED_RPC_METHODS = {
    # Ethermint exposes this method, but currently returns an incomplete txpool
    # shape compared with the execution-apis fixture. Track it with the other
    # unimplemented RPC gaps until the full response schema is supported.
    "txpool_content",
}
SCHEMA_MISMATCH_WHITELIST = {
    "request_schema_wrong": set(),
    "response_schema_wrong": set(),
    "mixed_wrong": set(),
}
ETHERMINT_MODERN_BLOCK_FIELD_SCHEMA_EXCEPTIONS = {
    # Genesis is a pre-fork block in the copied Geth fixture. Ethermint currently
    # returns Prague-era block fields for it, so ignore only those extra fields
    # until the block formatter becomes fork-aware for historical heights.
    "eth_getBlockByNumber/get-genesis",
    # The copied geth block-hash fixture is also from an old fork era, but this
    # test rewrites the hash to a local Ethermint block before comparing schema.
    "eth_getBlockByHash/get-block-by-hash",
}
ETHERMINT_LOCAL_TX_SCHEMA_EXCEPTIONS = {
    "eth_getBlockByHash/get-block-by-hash",
}
RELAXED_BLOCK_TRANSACTION_SCHEMA_EXCEPTIONS = {
    # These block tags resolve against the local Ethermint chain, not the copied
    # Geth fixture chain. Keep checking the block response envelope, but do not
    # require per-transaction object schemas to line up when the actual
    # transactions are from a different chain and can be different tx types.
    "eth_getBlockByNumber/get-finalized",
    "eth_getBlockByNumber/get-latest",
    "eth_getBlockByNumber/get-safe",
}
MODERN_BLOCK_FIELDS = {
    "baseFeePerGas",
    "blobGasUsed",
    "excessBlobGas",
    "parentBeaconBlockRoot",
    "requestsHash",
    "withdrawals",
    "withdrawalsRoot",
}
ETH_SIMULATE_BLOCK_HARDFORK_FIELDS = MODERN_BLOCK_FIELDS
ETH_SIMULATE_TRANSACTION_HARDFORK_FIELDS = {
    "accessList",
    "authorizationList",
    "blobVersionedHashes",
    "chainId",
    "maxFeePerBlobGas",
    "maxFeePerGas",
    "maxPriorityFeePerGas",
    "yParity",
}
ETH_SIMULATE_TIMESTAMP_HEADROOM = 120
LOCAL_FIXTURE_TX_FIELDS = {
    "chainId",
}
LOCAL_TRANSACTION_SCHEMA_EXCEPTIONS = {
    "eth_getTransactionByBlockHashAndIndex/get-block-n",
    "eth_getTransactionByBlockNumberAndIndex/get-block-n",
    "eth_getTransactionByHash/get-legacy-create",
    "eth_getTransactionByHash/get-legacy-input",
    "eth_getTransactionByHash/get-legacy-tx",
}
LOCAL_RECEIPT_SCHEMA_EXCEPTIONS = {
    "eth_getBlockReceipts/get-block-receipts-by-hash",
    "eth_getBlockReceipts/get-block-receipts-latest",
}
LEGACY_RECEIPT_ROOT_STATUS_SCHEMA_EXCEPTIONS = {
    "eth_getTransactionReceipt/get-legacy-contract",
    "eth_getTransactionReceipt/get-legacy-input",
    "eth_getTransactionReceipt/get-legacy-receipt",
}
LOCAL_RECEIPT_FUTURE_NULL_RESULT_ERROR_EXCEPTIONS = {
    "eth_getBlockReceipts/get-block-receipts-future",
}
LOCAL_LOG_SCHEMA_EXCEPTIONS = {
    "eth_getLogs/filter-with-blockHash",
    "eth_getLogs/filter-with-blockHash-and-topics",
}
LOCAL_LOG_FUTURE_BLOCK_RANGE_EXCEPTIONS = {
    "eth_getLogs/filter-error-future-block-range",
}
LOCAL_PROOF_SCHEMA_EXCEPTIONS = {
    "eth_getProof/get-account-proof-blockhash",
}
LOCAL_FIXTURE_RECEIPT_ADDRESS_FIELDS = {
    "contractAddress",
    "to",
}
EXCLUDED_SCHEMA_SPEC_CASES = {
    # Ethermint currently emits Prague-era block fields for historical blocks.
    # Exclude these explicitly fork-scoped Geth fixtures instead of treating
    # their expected older response shape as a current Ethermint schema failure.
    "eth_getBlockByNumber/get-block-london-fork",
    "eth_getBlockByNumber/get-block-merge-fork",
    "eth_getBlockByNumber/get-block-shanghai-fork",
    "eth_getBlockByNumber/get-block-cancun-fork",
}
TRANSACTION_BY_HASH_LOCAL_TX_KEYS = {
    "eth_getTransactionByHash/get-access-list": "access_list",
    "eth_getTransactionByHash/get-blob-tx": "blob",
    "eth_getTransactionByHash/get-dynamic-fee": "dynamic_fee",
    "eth_getTransactionByHash/get-legacy-create": "legacy_create",
    "eth_getTransactionByHash/get-legacy-input": "legacy_create",
    "eth_getTransactionByHash/get-legacy-tx": "legacy",
    "eth_getTransactionByHash/get-setcode-tx": "setcode",
}
TRANSACTION_RECEIPT_LOCAL_TX_KEYS = {
    "eth_getTransactionReceipt/get-access-list": "access_list",
    "eth_getTransactionReceipt/get-blob-tx": "blob",
    "eth_getTransactionReceipt/get-dynamic-fee": "dynamic_fee",
    "eth_getTransactionReceipt/get-legacy-contract": "legacy_create",
    "eth_getTransactionReceipt/get-legacy-input": "legacy_create",
    "eth_getTransactionReceipt/get-legacy-receipt": "legacy",
    "eth_getTransactionReceipt/get-setcode-tx": "setcode",
}
SEND_RAW_TRANSACTION_LOCAL_TX_KEYS = {
    "eth_sendRawTransaction/send-access-list-transaction": "access_list",
    "eth_sendRawTransaction/send-blob-tx": "blob",
    "eth_sendRawTransaction/send-dynamic-fee-access-list-transaction": (
        "dynamic_fee_access_list"
    ),
    "eth_sendRawTransaction/send-dynamic-fee-transaction": "dynamic_fee_create",
    "eth_sendRawTransaction/send-legacy-transaction": "legacy",
}
LEGACY_CREATE_BYTECODE = (
    "0x600d380380600d6000396000f360004381526020014681526020014181526020014"
    "881526020014481526020013281526020013481526020016000f3"
)
BLOB_VERSIONED_HASH = (
    "0x0100000000000000000000000000000000000000000000000000000000000000"
)


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


def _json_type_name(value):
    if value is None:
        return "null"
    if isinstance(value, bool):
        return "boolean"
    if type(value) in (int, float):
        return "number"
    if isinstance(value, str):
        return "string"
    if isinstance(value, list):
        return "array"
    if isinstance(value, dict):
        return "object"
    return type(value).__name__


def _schema_differences(expected, actual, path="$"):
    if isinstance(expected, dict) and isinstance(actual, dict):
        differences = []
        expected_keys = set(expected)
        actual_keys = set(actual)
        missing = sorted(expected_keys - actual_keys)
        extra = sorted(actual_keys - expected_keys)
        if missing:
            differences.append(f"{path}: expected-only keys {missing}")
        if extra:
            differences.append(f"{path}: actual-only keys {extra}")
        for key in sorted(expected_keys & actual_keys):
            differences.extend(
                _schema_differences(expected[key], actual[key], f"{path}.{key}")
            )
        return differences

    if isinstance(expected, list) and isinstance(actual, list):
        if not expected or not actual:
            return []
        differences = []
        for index, (exp, act) in enumerate(zip(expected, actual)):
            differences.extend(_schema_differences(exp, act, f"{path}[{index}]"))
        return differences

    if type(expected) in (int, float) and type(actual) in (int, float):
        return []

    if type(expected) is not type(actual):
        return [
            f"{path}: expected {_json_type_name(expected)}, "
            f"got {_json_type_name(actual)}"
        ]

    return []


def _format_schema_differences(expected, actual):
    differences = _schema_differences(expected, actual)
    return "\n".join(differences) if differences else "-"


def _normalize_historical_block_schema_exception(spec_name, expected, actual):
    if (
        spec_name not in ETHERMINT_MODERN_BLOCK_FIELD_SCHEMA_EXCEPTIONS
        and spec_name not in ETHERMINT_LOCAL_TX_SCHEMA_EXCEPTIONS
        and spec_name not in RELAXED_BLOCK_TRANSACTION_SCHEMA_EXCEPTIONS
    ):
        return expected, actual

    normalized_expected = deepcopy(expected)
    normalized_actual = deepcopy(actual)
    expected_result = normalized_expected.get("result")
    actual_result = normalized_actual.get("result")
    if not isinstance(expected_result, dict) or not isinstance(actual_result, dict):
        return normalized_expected, normalized_actual

    if spec_name in ETHERMINT_MODERN_BLOCK_FIELD_SCHEMA_EXCEPTIONS:
        # Geth formats historical blocks according to the fork active at that block.
        # Ethermint currently derives Ethereum RPC headers from CometBFT blocks and
        # populates modern fork fields even for historical blocks. Keep these old
        # fork fixtures useful for the schema test by ignoring only those known
        # extra fields until Ethermint's RPC block formatter is fork-aware.
        for field in MODERN_BLOCK_FIELDS:
            actual_result.pop(field, None)

    if spec_name in RELAXED_BLOCK_TRANSACTION_SCHEMA_EXCEPTIONS:
        expected_txs = expected_result.get("transactions")
        actual_txs = actual_result.get("transactions")
        if isinstance(expected_txs, list) and isinstance(actual_txs, list):
            expected_result["transactions"] = []
            actual_result["transactions"] = []

    if spec_name not in ETHERMINT_LOCAL_TX_SCHEMA_EXCEPTIONS:
        return normalized_expected, normalized_actual

    # This test rewrites the original Geth block hash to a local Ethermint block.
    # The local transfer transaction can include modern transaction-only fields
    # and a non-null `to`, while the copied fixture's first transactions are
    # contract creations. Normalize those local fixture artifacts separately from
    # the block-header fork fields above.
    expected_txs = expected_result.get("transactions", [])
    actual_txs = actual_result.get("transactions", [])
    if isinstance(expected_txs, list) and isinstance(actual_txs, list):
        for expected_tx, actual_tx in zip(expected_txs, actual_txs):
            if not isinstance(expected_tx, dict) or not isinstance(actual_tx, dict):
                continue
            for field in LOCAL_FIXTURE_TX_FIELDS:
                actual_tx.pop(field, None)
            if expected_tx.get("to") is None and isinstance(actual_tx.get("to"), str):
                actual_tx["to"] = None

    return normalized_expected, normalized_actual


def _normalize_local_receipt_schema_exception(spec_name, expected, actual):
    if spec_name not in LOCAL_RECEIPT_SCHEMA_EXCEPTIONS:
        return expected, actual

    normalized_expected = deepcopy(expected)
    normalized_actual = deepcopy(actual)
    expected_result = normalized_expected.get("result")
    actual_result = normalized_actual.get("result")
    if not isinstance(expected_result, list) or not isinstance(actual_result, list):
        return normalized_expected, normalized_actual

    for expected_receipt, actual_receipt in zip(expected_result, actual_result):
        if not isinstance(expected_receipt, dict) or not isinstance(
            actual_receipt, dict
        ):
            continue

        for field in LOCAL_FIXTURE_RECEIPT_ADDRESS_FIELDS:
            if field not in expected_receipt or field not in actual_receipt:
                continue

            expected_value = expected_receipt[field]
            actual_value = actual_receipt[field]
            if (expected_value is None and isinstance(actual_value, str)) or (
                actual_value is None and isinstance(expected_value, str)
            ):
                expected_receipt[field] = None
                actual_receipt[field] = None

    return normalized_expected, normalized_actual


def _normalize_legacy_receipt_schema_exception(spec_name, expected, actual):
    if spec_name not in LEGACY_RECEIPT_ROOT_STATUS_SCHEMA_EXCEPTIONS:
        return expected, actual

    normalized_expected = deepcopy(expected)
    normalized_actual = deepcopy(actual)
    expected_result = normalized_expected.get("result")
    actual_result = normalized_actual.get("result")
    if not isinstance(expected_result, dict) or not isinstance(actual_result, dict):
        return normalized_expected, normalized_actual

    # The copied Geth legacy receipt fixtures use the pre-Byzantium `root`
    # field, while local Ethermint receipts expose the modern `status` field.
    # Keep the rest of the receipt schema comparison strict.
    if (
        "root" in expected_result
        and "status" not in expected_result
        and "status" in actual_result
        and "root" not in actual_result
    ):
        expected_result.pop("root")
        actual_result.pop("status")

    return normalized_expected, normalized_actual


def _normalize_local_transaction_schema_exception(spec_name, expected, actual):
    if spec_name not in LOCAL_TRANSACTION_SCHEMA_EXCEPTIONS:
        return expected, actual

    normalized_expected = deepcopy(expected)
    normalized_actual = deepcopy(actual)
    expected_result = normalized_expected.get("result")
    actual_result = normalized_actual.get("result")
    if not isinstance(expected_result, dict) or not isinstance(actual_result, dict):
        return normalized_expected, normalized_actual

    # These copied Geth fixtures query historical unprotected transactions whose
    # transaction response omits `chainId`. Ethermint currently formats historical
    # transaction responses with the latest field set, so ignore that local-only
    # field when the request is rewritten to an Ethermint block/transaction.
    if "chainId" not in expected_result:
        actual_result.pop("chainId", None)

    # The copied Geth transaction is contract creation (`to: null`), but the
    # local Ethermint transaction at the rewritten block/index can be a normal
    # transfer. Keep the comparison focused on RPC schema fields, not tx kind.
    expected_to = expected_result.get("to")
    actual_to = actual_result.get("to")
    if (expected_to is None and isinstance(actual_to, str)) or (
        actual_to is None and isinstance(expected_to, str)
    ):
        expected_result["to"] = None
        actual_result["to"] = None

    return normalized_expected, normalized_actual


def _drop_asymmetric_fields(expected, actual, fields):
    for field in fields:
        if field in expected and field in actual:
            continue
        expected.pop(field, None)
        actual.pop(field, None)


def _normalize_eth_simulate_schema_exception(spec_name, expected, actual):
    if not spec_name.startswith("eth_simulateV1/"):
        return expected, actual

    normalized_expected = deepcopy(expected)
    normalized_actual = deepcopy(actual)
    expected_result = normalized_expected.get("result")
    actual_result = normalized_actual.get("result")
    if not isinstance(expected_result, list) or not isinstance(actual_result, list):
        return normalized_expected, normalized_actual

    for expected_block, actual_block in zip(expected_result, actual_result):
        if not isinstance(expected_block, dict) or not isinstance(actual_block, dict):
            continue

        # Ethermint may format simulated blocks/transactions with the latest
        # hardfork field set even when the copied Geth fixture was generated for
        # an earlier fork. Keep non-hardfork schema comparison strict.
        _drop_asymmetric_fields(
            expected_block, actual_block, ETH_SIMULATE_BLOCK_HARDFORK_FIELDS
        )

        expected_txs = expected_block.get("transactions")
        actual_txs = actual_block.get("transactions")
        if not isinstance(expected_txs, list) or not isinstance(actual_txs, list):
            continue

        for expected_tx, actual_tx in zip(expected_txs, actual_txs):
            if not isinstance(expected_tx, dict) or not isinstance(actual_tx, dict):
                continue
            _drop_asymmetric_fields(
                expected_tx, actual_tx, ETH_SIMULATE_TRANSACTION_HARDFORK_FIELDS
            )

    return normalized_expected, normalized_actual


def _is_local_receipt_future_null_result_error(spec_name, expected, actual):
    if spec_name not in LOCAL_RECEIPT_FUTURE_NULL_RESULT_ERROR_EXCEPTIONS:
        return False
    if expected.get("result") is not None:
        return False

    error = actual.get("error") or {}
    message = str(error.get("message", "")).lower()
    return (
        error.get("code") == -32000
        and "must be less than or equal to the current blockchain height" in message
    )


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
    import sys

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


class RpcSpecSchemaSummary:
    def __init__(self):
        self.schema_correct = []
        self.not_implemented = []
        self.request_schema_wrong = []
        self.response_schema_wrong = []

    def add(self, result):
        getattr(self, result.category).append(result)

    def format(self):
        lines = ["ethermint RPC spec schema case summary"]
        for attr in [
            "schema_correct",
            "not_implemented",
            "request_schema_wrong",
            "response_schema_wrong",
        ]:
            results = getattr(self, attr)
            lines.append(f"- {SCHEMA_CATEGORY_TITLES[attr]}: {len(results)}")
            by_method = defaultdict(int)
            examples = []
            for result in results:
                by_method[result.method] += 1
                if len(examples) < 8:
                    examples.append(result)
            if by_method:
                method_counts = ", ".join(
                    f"{method}={count}" for method, count in sorted(by_method.items())
                )
                lines.append(f"  methods: {method_counts}")
            for result in examples:
                lines.append(f"  example: {result.spec_name}: {result.reason}")
            if len(results) > len(examples):
                lines.append(f"  ... {len(results) - len(examples)} more")
        lines.extend(self._format_method_summary())
        return "\n".join(lines)

    def report(self):
        lines = [
            "# Ethermint RPC Schema Spec Report",
            "",
            "Generated by `spec/test_rpc_spec_schema.py`.",
            "",
            "This report compares Ethermint JSON-RPC responses with the copied "
            "execution-apis `.io` fixtures by schema only. Values are ignored, "
            "but response kind, object keys, and JSON value types must match.",
            "",
            "## Excluded Cases",
            "",
            "These fork-specific block-number fixtures are skipped because "
            "Ethermint currently returns Prague-era block fields for historical "
            "blocks, while Geth formats each block response according to the "
            "fork active at the queried block.",
            "",
        ]
        lines.extend(
            f"- `{spec_name}`" for spec_name in sorted(EXCLUDED_SCHEMA_SPEC_CASES)
        )
        lines.extend(
            [
                "",
                "## Case Summary",
                "",
            ]
        )
        lines.extend(self._markdown_case_summary())
        lines.extend(["", "## Method Summary", ""])
        lines.extend(self._markdown_method_summary())
        lines.extend(["", "## Null Result Summary", ""])
        lines.extend(self._markdown_null_result_summary())
        lines.extend(["", "## Expected Failure Whitelist", ""])
        lines.extend(self._markdown_expected_failure_whitelist())
        lines.extend(["", "## Detailed Mismatches", ""])
        lines.extend(self._markdown_details())
        return "\n".join(lines).rstrip() + "\n"

    def _markdown_case_summary(self):
        lines = ["| Category | Cases | Methods |", "| --- | ---: | --- |"]
        for attr in [
            "schema_correct",
            "not_implemented",
            "request_schema_wrong",
            "response_schema_wrong",
        ]:
            results = getattr(self, attr)
            methods = sorted({result.method for result in results})
            lines.append(
                "| {} | {} | {} |".format(
                    SCHEMA_CATEGORY_TITLES[attr],
                    len(results),
                    ", ".join(methods) if methods else "-",
                )
            )
        return lines

    def _markdown_method_summary(self):
        by_method = defaultdict(list)
        for result in self._all_results():
            by_method[result.method].append(result)

        by_verdict = defaultdict(list)
        for method, results in sorted(by_method.items()):
            by_verdict[self._method_verdict(results)].append((method, results))

        lines = ["| Category | Methods | Details |", "| --- | ---: | --- |"]
        for verdict in [
            "schema_correct",
            "not_implemented",
            "request_schema_wrong",
            "response_schema_wrong",
            "mixed_wrong",
        ]:
            methods = by_verdict.get(verdict, [])
            details = []
            for method, results in methods:
                counts = defaultdict(int)
                for result in results:
                    counts[result.category] += 1
                count_text = ", ".join(
                    f"{category}={counts[category]}" for category in sorted(counts)
                )
                details.append(f"`{method}` ({count_text})")
            lines.append(
                "| {} | {} | {} |".format(
                    SCHEMA_CATEGORY_TITLES[verdict],
                    len(methods),
                    "<br>".join(details) if details else "-",
                )
            )
        return lines

    def _markdown_null_result_summary(self):
        expected_null = []
        unexpected_null = []
        for result in self._all_results():
            expected = getattr(result, "expected", {})
            actual = getattr(result, "actual", {})
            if (
                "result" in expected
                and "result" in actual
                and expected.get("result") is None
                and actual.get("result") is None
            ):
                expected_null.append(result)
            elif (
                "result" in expected
                and "result" in actual
                and expected.get("result") is not None
                and actual.get("result") is None
            ):
                unexpected_null.append(result)

        lines = [
            "| Category | Cases | Details |",
            "| --- | ---: | --- |",
            "| expected `result: null` | {} | {} |".format(
                len(expected_null),
                ", ".join(f"`{result.spec_name}`" for result in expected_null) or "-",
            ),
            "| unexpected `result: null` | {} | {} |".format(
                len(unexpected_null),
                ", ".join(f"`{result.spec_name}`" for result in unexpected_null) or "-",
            ),
        ]
        return lines

    def _markdown_expected_failure_whitelist(self):
        lines = [
            "These method-level lists document known gaps that are currently "
            "excluded from the schema test failure condition. Reduce these lists "
            "as methods are implemented or response schemas are fixed.",
            "",
            "### Unimplemented RPC Methods",
            "",
        ]
        lines.extend(f"- `{method}`" for method in sorted(UNIMPLEMENTED_RPC_METHODS))
        lines.extend(
            [
                "",
                "### Schema Mismatch Whitelist",
                "",
                "| Category | RPC Methods |",
                "| --- | --- |",
            ]
        )
        for category in [
            "request_schema_wrong",
            "response_schema_wrong",
            "mixed_wrong",
        ]:
            methods = sorted(SCHEMA_MISMATCH_WHITELIST[category])
            lines.append(
                "| {} | {} |".format(
                    SCHEMA_CATEGORY_TITLES[category],
                    ", ".join(f"`{method}`" for method in methods) or "-",
                )
            )
        return lines

    def _markdown_details(self):
        lines = []
        for attr in [
            "not_implemented",
            "request_schema_wrong",
            "response_schema_wrong",
        ]:
            results = getattr(self, attr)
            lines.extend([f"### {SCHEMA_CATEGORY_TITLES[attr]}", ""])
            if not results:
                lines.extend(["No cases.", ""])
                continue

            for result in results:
                lines.extend(
                    [
                        f"#### `{result.spec_name}`",
                        "",
                        f"- Method: `{result.method}`",
                        f"- Reason: {result.reason}",
                        f"- Comment: {result.comment or '-'}",
                        "",
                    ]
                )
                if result.category == "response_schema_wrong":
                    lines.extend(
                        [
                            "Schema differences:",
                            "",
                            "```text",
                            _format_schema_differences(
                                result.schema_expected, result.schema_actual
                            ),
                            "```",
                            "",
                        ]
                    )
                lines.extend(
                    [
                        "Request:",
                        "",
                        "```json",
                        _markdown_json(result.request),
                        "```",
                        "",
                        "Expected:",
                        "",
                        "```json",
                        _markdown_json(result.expected),
                        "```",
                        "",
                        "Actual:",
                        "",
                        "```json",
                        _markdown_json(result.actual),
                        "```",
                        "",
                    ]
                )
        return lines

    def _format_method_summary(self):
        by_method = defaultdict(list)
        for result in self._all_results():
            by_method[result.method].append(result)

        by_verdict = defaultdict(list)
        for method, results in sorted(by_method.items()):
            verdict = self._method_verdict(results)
            by_verdict[verdict].append((method, results))

        lines = ["", "ethermint RPC spec schema method summary (exclusive)"]
        for verdict in [
            "schema_correct",
            "not_implemented",
            "request_schema_wrong",
            "response_schema_wrong",
            "mixed_wrong",
        ]:
            methods = by_verdict.get(verdict, [])
            lines.append(f"- {SCHEMA_CATEGORY_TITLES[verdict]}: {len(methods)}")
            for method, results in methods:
                counts = defaultdict(int)
                for result in results:
                    counts[result.category] += 1
                details = ", ".join(
                    f"{category}={counts[category]}" for category in sorted(counts)
                )
                lines.append(f"  {method}: {details}")
        unexpected_null = [
            result
            for result in self._all_results()
            if "result" in getattr(result, "expected", {})
            and "result" in getattr(result, "actual", {})
            and getattr(result, "expected", {}).get("result") is not None
            and getattr(result, "actual", {}).get("result") is None
        ]
        lines.extend(
            [
                "",
                "ethermint RPC spec schema null-result summary",
                f"- unexpected result: null: {len(unexpected_null)}",
            ]
        )
        return lines

    def _all_results(self):
        return (
            self.schema_correct
            + self.not_implemented
            + self.request_schema_wrong
            + self.response_schema_wrong
        )

    @staticmethod
    def _method_verdict(results):
        categories = {result.category for result in results}
        if categories == {"schema_correct"}:
            return "schema_correct"
        if categories == {"not_implemented"}:
            return "not_implemented"

        wrong_categories = categories - {"schema_correct"}
        if len(wrong_categories) == 1:
            return next(iter(wrong_categories))
        return "mixed_wrong"

    def method_verdicts(self):
        by_method = defaultdict(list)
        for result in self._all_results():
            by_method[result.method].append(result)
        return {
            method: self._method_verdict(results)
            for method, results in by_method.items()
        }


def _classify_schema(spec_name, request, expected, actual):
    method = request.get("method", "<unknown>")

    if _is_not_implemented(actual):
        return RpcSpecResult(
            spec_name,
            method,
            "not_implemented",
            actual.get("error", {}).get("message", "method not implemented"),
        )

    if _is_local_receipt_future_null_result_error(spec_name, expected, actual):
        return RpcSpecResult(
            spec_name,
            method,
            "schema_correct",
            "matching future block not found response",
        )

    expected_kind = _response_kind(expected)
    actual_kind = _response_kind(actual)

    if expected_kind == "error" and actual_kind == "result":
        return RpcSpecResult(
            spec_name,
            method,
            "request_schema_wrong",
            "spec expects request rejection, ethermint accepted it",
        )

    if (
        expected_kind == "result"
        and actual_kind == "error"
        and _is_request_schema_error(actual)
    ):
        return RpcSpecResult(
            spec_name,
            method,
            "request_schema_wrong",
            f"expected {expected_kind}, got request validation error: "
            f"{actual.get('error', {}).get('message')}",
        )

    if expected_kind != actual_kind:
        if method in INCOMPLETE_UNIMPLEMENTED_RPC_METHODS:
            return RpcSpecResult(
                spec_name,
                method,
                "not_implemented",
                f"incomplete implementation: expected {expected_kind} response, "
                f"got {actual_kind}",
            )
        return RpcSpecResult(
            spec_name,
            method,
            "response_schema_wrong",
            f"expected {expected_kind} response, got {actual_kind}",
        )

    schema_expected, schema_actual = _normalize_schema_exceptions(
        spec_name, expected, actual
    )
    if not _same_schema(schema_expected, schema_actual):
        mismatch = (
            _first_schema_mismatch(schema_expected, schema_actual) or "schema differs"
        )
        if method in INCOMPLETE_UNIMPLEMENTED_RPC_METHODS:
            return RpcSpecResult(
                spec_name,
                method,
                "not_implemented",
                f"incomplete implementation: {mismatch}",
            )
        return RpcSpecResult(
            spec_name,
            method,
            "response_schema_wrong",
            mismatch,
        )

    if actual_kind == "error" and _is_request_schema_error(actual):
        return RpcSpecResult(
            spec_name,
            method,
            "schema_correct",
            "matching request validation error schema",
        )

    return RpcSpecResult(spec_name, method, "schema_correct", "schema match")


def _normalize_schema_exceptions(spec_name, expected, actual):
    schema_expected, schema_actual = _normalize_historical_block_schema_exception(
        spec_name, expected, actual
    )
    schema_expected, schema_actual = _normalize_local_receipt_schema_exception(
        spec_name, schema_expected, schema_actual
    )
    schema_expected, schema_actual = _normalize_legacy_receipt_schema_exception(
        spec_name, schema_expected, schema_actual
    )
    schema_expected, schema_actual = _normalize_local_transaction_schema_exception(
        spec_name, schema_expected, schema_actual
    )
    return _normalize_eth_simulate_schema_exception(
        spec_name, schema_expected, schema_actual
    )


def _attach_details(result, request, expected, actual, comment):
    result.request = request
    result.expected = expected
    result.actual = actual
    result.schema_expected, result.schema_actual = _normalize_schema_exceptions(
        result.spec_name, expected, actual
    )
    result.comment = comment
    return result


def _has_non_null_result(expected):
    return expected.get("result") is not None


def _first_receipt_block_number(expected):
    result = expected.get("result")
    if not isinstance(result, list) or not result:
        return None

    first_receipt = result[0]
    if not isinstance(first_receipt, dict):
        return None

    block_number = first_receipt.get("blockNumber")
    return block_number if isinstance(block_number, str) else None


def _first_log_block_number(expected):
    result = expected.get("result")
    if not isinstance(result, list) or not result:
        return None

    first_log = result[0]
    if not isinstance(first_log, dict):
        return None

    block_number = first_log.get("blockNumber")
    return block_number if isinstance(block_number, str) else None


def _is_hex_quantity(value):
    return isinstance(value, str) and value.startswith("0x") and len(value) < 66


def _hex_quantity_to_int(value):
    if not _is_hex_quantity(value):
        return None
    try:
        return int(value, 16)
    except ValueError:
        return None


def _eth_simulate_block_state_calls(request):
    params = request.get("params")
    if not isinstance(params, list) or not params:
        return None

    options = params[0]
    if not isinstance(options, dict):
        return None

    block_state_calls = options.get("blockStateCalls")
    return block_state_calls if isinstance(block_state_calls, list) else None


def _block_override_values(block_state_calls, key):
    values = []
    for block_state_call in block_state_calls:
        if not isinstance(block_state_call, dict):
            continue

        block_overrides = block_state_call.get("blockOverrides")
        if not isinstance(block_overrides, dict):
            continue

        value = _hex_quantity_to_int(block_overrides.get(key))
        if value is not None:
            values.append(value)
    return values


def _shift_block_override_values(block_state_calls, key, base_value):
    values = _block_override_values(block_state_calls, key)
    if not values:
        return False

    first_value = min(values)
    for block_state_call in block_state_calls:
        if not isinstance(block_state_call, dict):
            continue

        block_overrides = block_state_call.get("blockOverrides")
        if not isinstance(block_overrides, dict):
            continue

        value = _hex_quantity_to_int(block_overrides.get(key))
        if value is not None:
            block_overrides[key] = hex(base_value + value - first_value)

    return True


def _first_eth_simulate_result_quantity(expected, key):
    result = expected.get("result")
    if not isinstance(result, list) or not result:
        return None

    first_block = result[0]
    if not isinstance(first_block, dict):
        return None

    return _hex_quantity_to_int(first_block.get(key))


def _rewrite_eth_simulate_request_for_local_schema_fixture(request, expected, context):
    if "result" not in expected or expected.get("result") is None:
        return request, False

    block_state_calls = _eth_simulate_block_state_calls(request)
    if block_state_calls is None:
        return request, False

    number_values = _block_override_values(block_state_calls, "number")
    time_values = _block_override_values(block_state_calls, "time")
    if not number_values and not time_values:
        return request, False

    rewritten = deepcopy(request)
    rewritten_block_state_calls = _eth_simulate_block_state_calls(rewritten)
    latest_block = context["w3"].eth.get_block("latest")
    rewritten_any = False

    if number_values:
        first_result_number = _first_eth_simulate_result_quantity(expected, "number")
        leading_block_count = 0
        if first_result_number is not None:
            leading_block_count = max(0, min(number_values) - first_result_number)
        number_base = int(latest_block.number) + 1 + leading_block_count
        rewritten_any |= _shift_block_override_values(
            rewritten_block_state_calls, "number", number_base
        )

    if time_values:
        next_safe_time = int(latest_block.timestamp) + ETH_SIMULATE_TIMESTAMP_HEADROOM
        time_base = max(min(time_values), next_safe_time)
        rewritten_any |= _shift_block_override_values(
            rewritten_block_state_calls, "time", time_base
        )

    return rewritten, rewritten_any


def _rewrite_request_for_local_schema_fixture(spec_name, request, expected, context):
    rewritten = deepcopy(request)
    method = rewritten.get("method")
    params = rewritten.get("params") or []
    if not params:
        return request, False

    if method == "eth_simulateV1":
        return _rewrite_eth_simulate_request_for_local_schema_fixture(
            request, expected, context
        )

    if method == "eth_getBlockReceipts":
        block_id = params[0]
        expected_result = expected.get("result")

        if isinstance(expected_result, list) and expected_result:
            if block_id == "latest":
                params[0] = context["block_number"]
            elif isinstance(block_id, str) and len(block_id) == 66:
                block_number = _first_receipt_block_number(expected)
                block_hash = context["fixture_block_hashes"].get(block_number)
                if block_hash is None:
                    return request, False
                params[0] = block_hash
            else:
                return request, False
        elif expected_result is None and _is_hex_quantity(block_id):
            params[0] = context["future_block_number"]
        else:
            return request, False

        rewritten["params"] = params
        return rewritten, True

    if method == "eth_getLogs" and spec_name in LOCAL_LOG_SCHEMA_EXCEPTIONS:
        filter_params = params[0]
        if not isinstance(filter_params, dict):
            return request, False
        if not isinstance(filter_params.get("blockHash"), str):
            return request, False

        block_number = _first_log_block_number(expected)
        block_hash = context["fixture_block_hashes"].get(block_number)
        if block_hash is None:
            return request, False

        filter_params["blockHash"] = block_hash
        rewritten["params"] = params
        return rewritten, True

    if method == "eth_getLogs" and spec_name in LOCAL_LOG_FUTURE_BLOCK_RANGE_EXCEPTIONS:
        filter_params = params[0]
        if not isinstance(filter_params, dict):
            return request, False

        filter_params["fromBlock"] = context["block_number"]
        filter_params["toBlock"] = context["future_block_number"]
        rewritten["params"] = params
        return rewritten, True

    if method == "eth_getProof" and spec_name in LOCAL_PROOF_SCHEMA_EXCEPTIONS:
        if len(params) < 3 or not isinstance(params[2], str) or len(params[2]) != 66:
            return request, False
        params[2] = context["block_hash"]
        rewritten["params"] = params
        return rewritten, True

    if not _has_non_null_result(expected):
        return request, False

    if method in {
        "eth_getBlockByHash",
        "eth_getBlockTransactionCountByHash",
        "eth_getTransactionByBlockHashAndIndex",
    }:
        params[0] = context["block_hash"]
    elif method == "eth_getTransactionByBlockNumberAndIndex":
        params[0] = context["block_number"]
    elif (
        method == "eth_getTransactionByHash"
        and spec_name in TRANSACTION_BY_HASH_LOCAL_TX_KEYS
    ):
        params[0] = context["tx_hashes"][TRANSACTION_BY_HASH_LOCAL_TX_KEYS[spec_name]]
    elif (
        method == "eth_getTransactionReceipt"
        and spec_name in TRANSACTION_RECEIPT_LOCAL_TX_KEYS
    ):
        params[0] = context["tx_hashes"][TRANSACTION_RECEIPT_LOCAL_TX_KEYS[spec_name]]
    elif (
        method == "eth_sendRawTransaction"
        and spec_name in SEND_RAW_TRANSACTION_LOCAL_TX_KEYS
    ):
        params[0] = context["send_raw_txs"][
            SEND_RAW_TRANSACTION_LOCAL_TX_KEYS[spec_name]
        ]
    elif method in {"eth_getTransactionByHash", "eth_getTransactionReceipt"}:
        params[0] = context["tx_hash"]
    elif (
        # The copied execution-api fixture uses a geth block hash. For the
        # Ethermint schema test, replace it with a block hash produced by this
        # local test chain so eth_getBalance queries an existing historical state.
        method == "eth_getBalance"
        and len(params) >= 2
        and isinstance(params[1], str)
        and len(params[1]) == 66
        and params[1].startswith("0x")
    ):
        params[1] = context["block_hash"]
    else:
        return request, False

    rewritten["params"] = params
    return rewritten, True


def _prepare_schema_request(spec_name, request_body, expected_body, rpc_context):
    request = json.loads(request_body)
    expected = json.loads(expected_body)
    request, runtime_rewrite_note = _rewrite_request_for_ethermint_runtime_fixture(
        spec_name, request
    )
    request, rewritten = _rewrite_request_for_local_schema_fixture(
        spec_name, request, expected, rpc_context
    )
    return request, expected, runtime_rewrite_note, rewritten


def _format_case_context(comments, runtime_rewrite_note, rewritten, extra_note=None):
    context = comments[0] if comments else ""
    if runtime_rewrite_note:
        context = f"{context} ({runtime_rewrite_note})"
    if rewritten:
        context = f"{context} (request rewritten to local Ethermint fixture identifier)"
    if extra_note:
        context = f"{context} ({extra_note})"
    return context


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


def _format_method_set(methods):
    return ", ".join(f"`{method}`" for method in sorted(methods)) or "-"


def _schema_mismatches(summary):
    mismatches = summary.request_schema_wrong + summary.response_schema_wrong
    return [
        f"{result.spec_name} ({result.method}): {result.reason}"
        for result in mismatches
    ]


def _expected_failure_drift(summary):
    verdicts = summary.method_verdicts()
    expected_by_verdict = {
        "not_implemented": UNIMPLEMENTED_RPC_METHODS,
        **SCHEMA_MISMATCH_WHITELIST,
    }
    actual_by_verdict = {
        verdict: {method for method, actual in verdicts.items() if actual == verdict}
        for verdict in expected_by_verdict
    }

    drift = []
    for verdict, expected_methods in expected_by_verdict.items():
        actual_methods = actual_by_verdict[verdict]
        unexpected = actual_methods - expected_methods
        stale = expected_methods - actual_methods
        if unexpected:
            drift.append(
                "{} has unlisted methods: {}".format(
                    SCHEMA_CATEGORY_TITLES[verdict],
                    _format_method_set(unexpected),
                )
            )
        if stale:
            drift.append(
                "{} whitelist has stale methods: {}".format(
                    SCHEMA_CATEGORY_TITLES[verdict],
                    _format_method_set(stale),
                )
            )

    allowed_wrong_verdicts = set(expected_by_verdict)
    unexpected_wrong_verdicts = {
        verdict
        for verdict in verdicts.values()
        if verdict != "schema_correct" and verdict not in allowed_wrong_verdicts
    }
    for verdict in sorted(unexpected_wrong_verdicts):
        methods = {method for method, actual in verdicts.items() if actual == verdict}
        drift.append(
            "{} has no whitelist: {}".format(
                SCHEMA_CATEGORY_TITLES[verdict],
                _format_method_set(methods),
            )
        )
    return drift


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

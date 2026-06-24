from copy import deepcopy

from _schema_constants import (
    ETH_SIMULATE_BLOCK_HARDFORK_FIELDS,
    ETH_SIMULATE_TRANSACTION_HARDFORK_FIELDS,
    ETHERMINT_LOCAL_TX_SCHEMA_EXCEPTIONS,
    ETHERMINT_MODERN_BLOCK_FIELD_SCHEMA_EXCEPTIONS,
    INCOMPLETE_UNIMPLEMENTED_RPC_METHODS,
    LEGACY_RECEIPT_ROOT_STATUS_SCHEMA_EXCEPTIONS,
    LOCAL_FIXTURE_RECEIPT_ADDRESS_FIELDS,
    LOCAL_FIXTURE_TX_FIELDS,
    LOCAL_RECEIPT_FUTURE_NULL_RESULT_ERROR_EXCEPTIONS,
    LOCAL_RECEIPT_SCHEMA_EXCEPTIONS,
    LOCAL_TRANSACTION_SCHEMA_EXCEPTIONS,
    MODERN_BLOCK_FIELDS,
    RELAXED_BLOCK_TRANSACTION_SCHEMA_EXCEPTIONS,
)
from test_rpc_spec import (
    RpcSpecResult,
    _first_schema_mismatch,
    _is_not_implemented,
    _is_request_schema_error,
    _response_kind,
    _same_schema,
)


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


def _drop_asymmetric_fields(expected, actual, fields):
    for field in fields:
        if field in expected and field in actual:
            continue
        expected.pop(field, None)
        actual.pop(field, None)


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


def _attach_details(result, request, expected, actual, comment):
    result.request = request
    result.expected = expected
    result.actual = actual
    result.schema_expected, result.schema_actual = _normalize_schema_exceptions(
        result.spec_name, expected, actual
    )
    result.comment = comment
    return result

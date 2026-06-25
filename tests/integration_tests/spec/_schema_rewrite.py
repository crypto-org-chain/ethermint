# Rewrites upstream Geth fixture request parameters to use identifiers that
# exist on the local Ethermint test chain.  Block hashes, transaction hashes,
# and block numbers from the copied Geth fixture are replaced with live values
# supplied via the rpc_context fixture so each RPC call returns a real response.

import json
from copy import deepcopy

from _schema_constants import (
    ETH_SIMULATE_TIMESTAMP_HEADROOM,
    LOCAL_LOG_FUTURE_BLOCK_RANGE_EXCEPTIONS,
    LOCAL_LOG_SCHEMA_EXCEPTIONS,
    LOCAL_PROOF_SCHEMA_EXCEPTIONS,
    SEND_RAW_TRANSACTION_LOCAL_TX_KEYS,
    TRANSACTION_BY_HASH_LOCAL_TX_KEYS,
    TRANSACTION_RECEIPT_LOCAL_TX_KEYS,
)
from _rpc_spec_common import _rewrite_request_for_ethermint_runtime_fixture


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
    # Shift each override so the smallest fixture value maps to base_value,
    # preserving relative spacing between blocks in the simulate call.
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
            # The fixture may start the simulate window a few blocks after the
            # first overridden block number. Preserve that gap so the request
            # targets blocks that don't yet exist on the local chain.
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
        "debug_traceBlockByHash",
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
    elif method in {"debug_traceTransaction", "eth_getTransactionByHash", "eth_getTransactionReceipt"}:
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

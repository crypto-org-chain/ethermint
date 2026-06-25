# Shared utilities for the RPC spec test suite: HTTP JSON-RPC transport,
# response classification helpers (not-implemented vs schema error vs match),
# structural JSON schema comparison, and fixture-level request patching used
# by both test_rpc_spec_schema.py and test_simulate.py.

import json
import urllib.error
import urllib.request
from copy import deepcopy
from pathlib import Path

SPEC_ROOT = Path(__file__).parent
REQUEST_SCHEMA_ERROR_CODES = {-32600, -32602, -32700}
CATEGORY_TITLES = {
    "correct": "correct implemented",
    "not_implemented": "not implemented",
    "request_schema_wrong": "implemented, but request schema is wrong",
    "response_schema_wrong": "implemented, but response schema is wrong",
    "value_wrong": "implemented, but value is wrong",
    "mixed_wrong": "implemented, but multiple categories are wrong",
}


def _collect_spec_files():
    return sorted(
        path.relative_to(SPEC_ROOT).with_suffix("").as_posix()
        for path in SPEC_ROOT.glob("*/*.io")
    )


SPEC_FILES = _collect_spec_files()
ETH_SIMULATE_FUTURE_BLOCK_SPEC = (
    "eth_simulateV1/ethSimulate-empty-with-block-num-set-plus1"
)
ETH_SIMULATE_FUTURE_BLOCK_NUMBER = "0x111"


class RpcSpecResult:
    def __init__(self, spec_name, method, category, reason):
        self.spec_name = spec_name
        self.method = method
        self.category = category
        self.reason = reason


def _send_rpc(endpoint, request_body):
    req = urllib.request.Request(
        endpoint,
        data=request_body.encode(),
        headers={"Content-Type": "application/json"},
    )
    try:
        with urllib.request.urlopen(req, timeout=30) as resp:
            return json.loads(resp.read().decode())
    except urllib.error.HTTPError as err:
        body = err.read().decode()
        try:
            return json.loads(body)
        except json.JSONDecodeError:
            return {
                "jsonrpc": "2.0",
                "id": None,
                "error": {
                    "code": err.code,
                    "message": body,
                },
            }


def _rewrite_request_for_ethermint_runtime_fixture(spec_name, request):
    # One fixture requests a future block by absolute number that exceeds the
    # upstream geth chain height. Rewrite it to a block number that Ethermint
    # will also treat as a future (not-yet-produced) block.
    if spec_name != ETH_SIMULATE_FUTURE_BLOCK_SPEC:
        return request, None

    rewritten = deepcopy(request)
    params = rewritten.get("params")
    if isinstance(params, list) and len(params) >= 2:
        params[1] = ETH_SIMULATE_FUTURE_BLOCK_NUMBER
        return (
            rewritten,
            "request block number rewritten to an Ethermint future block",
        )
    return request, None


def _response_kind(response):
    if "result" in response:
        return "result"
    if "error" in response:
        return "error"
    return "unknown"


def _is_not_implemented(response):
    error = response.get("error") or {}
    message = str(error.get("message", "")).lower()
    # -32601 is the standard JSON-RPC "method not found" code; also catch
    # implementation-specific messages that convey the same meaning.
    return error.get("code") == -32601 or (
        "method" in message
        and (
            "not found" in message
            or "does not exist" in message
            or "not available" in message
            or "unsupported" in message
        )
    )


def _is_request_schema_error(response):
    error = response.get("error") or {}
    message = str(error.get("message", "")).lower()
    return error.get("code") in REQUEST_SCHEMA_ERROR_CODES or any(
        marker in message
        for marker in [
            "invalid argument",
            "invalid input",
            "invalid params",
            "missing",
            "cannot unmarshal",
            "required",
            "too many arguments",
            "argument count",
        ]
    )


def _same_schema(expected, actual):
    # Compares JSON structure (key sets and value types) without checking values.
    # Two JSON numbers are always schema-compatible regardless of their actual value.
    if isinstance(expected, dict) and isinstance(actual, dict):
        if set(expected) != set(actual):
            return False
        return all(_same_schema(expected[key], actual[key]) for key in expected)
    if isinstance(expected, list) and isinstance(actual, list):
        if not expected or not actual:
            return True
        return all(_same_schema(exp, act) for exp, act in zip(expected, actual))
    if _is_json_number(expected) and _is_json_number(actual):
        return True
    return type(expected) is type(actual)


def _is_json_number(value):
    return type(value) in (int, float)


def _first_schema_mismatch(expected, actual, path="$"):
    if isinstance(expected, dict) and isinstance(actual, dict):
        expected_keys = set(expected)
        actual_keys = set(actual)
        missing = sorted(expected_keys - actual_keys)
        extra = sorted(actual_keys - expected_keys)
        if missing:
            return f"{path}: missing keys {missing}"
        if extra:
            return f"{path}: extra keys {extra}"
        for key in sorted(expected_keys):
            mismatch = _first_schema_mismatch(
                expected[key], actual[key], f"{path}.{key}"
            )
            if mismatch:
                return mismatch
        return None
    if isinstance(expected, list) and isinstance(actual, list):
        if not expected or not actual:
            return None
        for index, (exp, act) in enumerate(zip(expected, actual)):
            mismatch = _first_schema_mismatch(exp, act, f"{path}[{index}]")
            if mismatch:
                return mismatch
        return None
    if _is_json_number(expected) and _is_json_number(actual):
        return None
    if type(expected) is not type(actual):
        return (
            f"{path}: expected {type(expected).__name__}, "
            f"got {type(actual).__name__}"
        )
    return None


def _format_json(value, limit=1200, indent=None):
    text = json.dumps(value, sort_keys=True, indent=indent)
    if limit is None or len(text) <= limit:
        return text
    return f"{text[:limit]}... <truncated {len(text) - limit} chars>"


def _markdown_json(value):
    return _format_json(value, limit=None, indent=2)

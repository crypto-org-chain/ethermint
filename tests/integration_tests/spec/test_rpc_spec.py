import json
import time
import urllib.error
import urllib.request
from collections import defaultdict
from pathlib import Path

import pytest

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


class RpcSpecSummary:
    def __init__(self):
        self.correct = []
        self.not_implemented = []
        self.request_schema_wrong = []
        self.response_schema_wrong = []
        self.value_wrong = []

    def add(self, result):
        getattr(self, result.category).append(result)

    def format(self):
        lines = ["", "ethermint RPC spec case summary"]
        for attr in [
            "correct",
            "not_implemented",
            "request_schema_wrong",
            "response_schema_wrong",
            "value_wrong",
        ]:
            results = getattr(self, attr)
            lines.append(f"- {CATEGORY_TITLES[attr]}: {len(results)}")
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

    def _format_method_summary(self):
        by_method = defaultdict(list)
        for result in self._all_results():
            by_method[result.method].append(result)

        by_verdict = defaultdict(list)
        for method, results in sorted(by_method.items()):
            verdict = self._method_verdict(results)
            by_verdict[verdict].append((method, results))

        lines = ["", "ethermint RPC spec method summary (exclusive)"]
        for verdict in [
            "correct",
            "not_implemented",
            "request_schema_wrong",
            "response_schema_wrong",
            "value_wrong",
            "mixed_wrong",
        ]:
            methods = by_verdict.get(verdict, [])
            lines.append(f"- {CATEGORY_TITLES[verdict]}: {len(methods)}")
            for method, results in methods:
                counts = defaultdict(int)
                for result in results:
                    counts[result.category] += 1
                details = ", ".join(
                    f"{category}={counts[category]}" for category in sorted(counts)
                )
                lines.append(f"  {method}: {details}")
        return lines

    def _all_results(self):
        return (
            self.correct
            + self.not_implemented
            + self.request_schema_wrong
            + self.response_schema_wrong
            + self.value_wrong
        )

    @staticmethod
    def _method_verdict(results):
        categories = {result.category for result in results}
        if categories == {"correct"}:
            return "correct"
        if categories == {"not_implemented"}:
            return "not_implemented"

        wrong_categories = categories - {"correct"}
        if len(wrong_categories) == 1:
            return next(iter(wrong_categories))
        return "mixed_wrong"


class RpcSpecResult:
    def __init__(self, spec_name, method, category, reason):
        self.spec_name = spec_name
        self.method = method
        self.category = category
        self.reason = reason


def _parse_spec_file(spec_name):
    filepath = SPEC_ROOT / f"{spec_name}.io"
    request_line = None
    expected_line = None
    comments = []
    with filepath.open() as f:
        for line in f:
            line = line.strip()
            if line.startswith("//"):
                comments.append(line[2:].strip())
            elif line.startswith(">> "):
                request_line = line[3:]
            elif line.startswith("<< "):
                expected_line = line[3:]
    return request_line, expected_line, comments


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


def _response_kind(response):
    if "result" in response:
        return "result"
    if "error" in response:
        return "error"
    return "unknown"


def _is_not_implemented(response):
    error = response.get("error") or {}
    message = str(error.get("message", "")).lower()
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
    if isinstance(expected, dict) and isinstance(actual, dict):
        if set(expected) != set(actual):
            return False
        return all(_same_schema(expected[key], actual[key]) for key in expected)
    if isinstance(expected, list) and isinstance(actual, list):
        if not expected or not actual:
            return True
        return all(_same_schema(exp, act) for exp, act in zip(expected, actual))
    return type(expected) is type(actual)


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
    if type(expected) is not type(actual):
        return (
            f"{path}: expected {type(expected).__name__}, "
            f"got {type(actual).__name__}"
        )
    return None


def _first_value_mismatch(expected, actual, path="$"):
    if expected == actual:
        return None
    if isinstance(expected, dict) and isinstance(actual, dict):
        for key in sorted(expected):
            mismatch = _first_value_mismatch(
                expected[key], actual[key], f"{path}.{key}"
            )
            if mismatch:
                return mismatch
        return None
    if isinstance(expected, list) and isinstance(actual, list):
        if len(expected) != len(actual):
            return f"{path}: expected length {len(expected)}, got {len(actual)}"
        for index, (exp, act) in enumerate(zip(expected, actual)):
            mismatch = _first_value_mismatch(exp, act, f"{path}[{index}]")
            if mismatch:
                return mismatch
        return None
    return f"{path}: expected {expected!r}, got {actual!r}"


def _compact_json(value, limit=1200):
    text = json.dumps(value, sort_keys=True)
    if len(text) <= limit:
        return text
    return f"{text[:limit]}... <truncated {len(text) - limit} chars>"


def _classify(spec_name, request, expected, actual):
    method = request.get("method", "<unknown>")
    if expected == actual:
        return RpcSpecResult(spec_name, method, "correct", "strict match")

    if _is_not_implemented(actual):
        return RpcSpecResult(
            spec_name,
            method,
            "not_implemented",
            actual.get("error", {}).get("message", "method not implemented"),
        )

    expected_kind = _response_kind(expected)
    actual_kind = _response_kind(actual)

    if actual_kind == "error" and _is_request_schema_error(actual):
        return RpcSpecResult(
            spec_name,
            method,
            "request_schema_wrong",
            f"expected {expected_kind}, got request validation error: "
            f"{actual.get('error', {}).get('message')}",
        )

    if expected_kind == "error" and actual_kind == "result":
        return RpcSpecResult(
            spec_name,
            method,
            "request_schema_wrong",
            "spec expects request rejection, ethermint accepted it",
        )

    if expected_kind != actual_kind:
        return RpcSpecResult(
            spec_name,
            method,
            "value_wrong",
            f"expected {expected_kind} response, got {actual_kind}",
        )

    if not _same_schema(expected, actual):
        return RpcSpecResult(
            spec_name,
            method,
            "response_schema_wrong",
            _first_schema_mismatch(expected, actual) or "schema differs",
        )

    return RpcSpecResult(
        spec_name,
        method,
        "value_wrong",
        _first_value_mismatch(expected, actual) or "values differ",
    )


@pytest.fixture(scope="module")
def rpc_endpoint(ethermint):
    """Wait for the chain to reach the highest block used by the copied specs."""
    w3 = ethermint.w3
    for _ in range(480):
        try:
            if w3.eth.block_number >= 45:
                break
        except Exception:
            pass
        time.sleep(0.5)
    else:
        raise TimeoutError("ethermint did not reach block 45 within timeout")
    return ethermint.w3_http_endpoint


@pytest.fixture(scope="module")
def rpc_spec_summary():
    summary = RpcSpecSummary()
    yield summary
    print(summary.format())


@pytest.mark.parametrize("spec_name", SPEC_FILES)
def test_ethermint_rpc_matches_execution_api_spec(
    rpc_endpoint, rpc_spec_summary, spec_name
):
    request_body, expected_body, comments = _parse_spec_file(spec_name)
    assert request_body, f"no request line (>> ...) in {spec_name}.io"
    assert expected_body, f"no expected response line (<< ...) in {spec_name}.io"

    request = json.loads(request_body)
    expected = json.loads(expected_body)
    actual = _send_rpc(rpc_endpoint, request_body)

    result = _classify(spec_name, request, expected, actual)
    rpc_spec_summary.add(result)

    context = comments[0] if comments else spec_name
    assert result.category == "correct", (
        f"{spec_name}: {context}: {result.reason}\n"
        f"expected: {_compact_json(expected)}\n"
        f"actual:   {_compact_json(actual)}"
    )

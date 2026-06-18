import json
from copy import deepcopy
from collections import defaultdict
from pathlib import Path

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

REPORT_PATH = Path(__file__).with_name("rpc_schema_report.md")
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
    "txpool_contentFrom",
}
SCHEMA_MISMATCH_WHITELIST = {
    "response_schema_wrong": {
        "eth_getBlockReceipts",
        "eth_getLogs",
        "eth_getProof",
        "eth_getTransactionByBlockHashAndIndex",
        "eth_getTransactionByBlockNumberAndIndex",
        "eth_getTransactionByHash",
        "eth_getTransactionReceipt",
        "eth_sendRawTransaction",
        "eth_simulateV1",
        "txpool_content",
    },
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
LOCAL_FIXTURE_TX_FIELDS = {
    "chainId",
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


@pytest.fixture(scope="module")
def rpc_context(rpc_endpoint):
    import sys

    sys.path.append(str(Path(__file__).parents[1]))
    from utils import ADDRS, KEYS, send_transaction

    w3 = Web3(Web3.HTTPProvider(rpc_endpoint))
    receipt = send_transaction(
        w3,
        {"to": ADDRS["community"], "value": 1, "gasPrice": w3.eth.gas_price},
        KEYS["validator"],
    )
    return {
        "endpoint": rpc_endpoint,
        "block_hash": Web3.to_hex(receipt.blockHash),
        "block_number": hex(receipt.blockNumber),
        "tx_hash": Web3.to_hex(receipt.transactionHash),
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
        for category in ["response_schema_wrong", "mixed_wrong"]:
            methods = sorted(SCHEMA_MISMATCH_WHITELIST[category])
            lines.append(
                "| {} | {} |".format(
                    SCHEMA_CATEGORY_TITLES[category],
                    ", ".join(f"`{method}`" for method in methods),
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
                            _format_schema_differences(result.expected, result.actual),
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
        return RpcSpecResult(
            spec_name,
            method,
            "response_schema_wrong",
            f"expected {expected_kind} response, got {actual_kind}",
        )

    schema_expected, schema_actual = _normalize_historical_block_schema_exception(
        spec_name, expected, actual
    )
    if not _same_schema(schema_expected, schema_actual):
        return RpcSpecResult(
            spec_name,
            method,
            "response_schema_wrong",
            _first_schema_mismatch(schema_expected, schema_actual) or "schema differs",
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
    result.comment = comment
    return result


def _has_non_null_result(expected):
    return expected.get("result") is not None


def _rewrite_request_for_local_schema_fixture(request, expected, context):
    if not _has_non_null_result(expected):
        return request, False

    rewritten = deepcopy(request)
    method = rewritten.get("method")
    params = rewritten.get("params") or []

    if method in {
        "eth_getBlockByHash",
        "eth_getBlockTransactionCountByHash",
        "eth_getTransactionByBlockHashAndIndex",
    }:
        params[0] = context["block_hash"]
    elif method == "eth_getTransactionByBlockNumberAndIndex":
        params[0] = context["block_number"]
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
        request, expected, rpc_context
    )
    return request, expected, runtime_rewrite_note, rewritten


def _format_case_context(comments, runtime_rewrite_note, rewritten, extra_note=None):
    context = comments[0] if comments else ""
    if runtime_rewrite_note:
        context = f"{context} ({runtime_rewrite_note})"
    if rewritten:
        context = f"{context} (request rewritten to local Ethermint fixture hash)"
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
    REPORT_PATH.write_text(report)

    print("")
    print(summary.format())
    print("")
    print(f"wrote schema report: {REPORT_PATH}")

    drift = _expected_failure_drift(summary)
    assert not drift, (
        "RPC schema expected-failure whitelist drifted:\n"
        + "\n".join(f"- {line}" for line in drift)
        + f"\nSee detailed report: {REPORT_PATH}"
    )

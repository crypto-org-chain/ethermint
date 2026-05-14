import json
import os
import time
import urllib.request

import pytest

SPEC_DIR = os.path.join(os.path.dirname(os.path.abspath(__file__)), "eth_simulateV1")


def _collect_spec_files():
    if not os.path.isdir(SPEC_DIR):
        return []
    return sorted(f[:-3] for f in os.listdir(SPEC_DIR) if f.endswith(".io"))


SPEC_FILES = _collect_spec_files()


def _parse_spec_file(spec_name):
    filepath = os.path.join(SPEC_DIR, spec_name + ".io")
    request_line = None
    expected_line = None
    with open(filepath) as f:
        for line in f:
            line = line.strip()
            if line.startswith(">> "):
                request_line = line[3:]
            elif line.startswith("<< "):
                expected_line = line[3:]
    return request_line, expected_line


def _send_rpc(endpoint, request_body):
    req = urllib.request.Request(
        endpoint,
        data=request_body.encode(),
        headers={"Content-Type": "application/json"},
    )
    with urllib.request.urlopen(req, timeout=30) as resp:
        return json.loads(resp.read().decode())


def _compare_results(expected, actual):
    """Structurally compare eth_simulateV1 responses.

    Returns None on match, or a human-readable mismatch description.
    """
    exp_type = "result" if "result" in expected else "error"
    act_type = "result" if "result" in actual else "error"

    if exp_type != act_type:
        return f"expected {exp_type} response, got {act_type}"

    if exp_type == "result":
        er = expected.get("result", [])
        ar = actual.get("result", [])
        if len(er) != len(ar):
            return f"block count: expected={len(er)} got={len(ar)}"

        for i, (eb, ab) in enumerate(zip(er, ar)):
            ec = eb.get("calls", [])
            ac = ab.get("calls", [])
            if len(ec) != len(ac):
                return f"block[{i}] call count: expected={len(ec)} got={len(ac)}"
            for j, (ecall, acall) in enumerate(zip(ec, ac)):
                es = ecall.get("status", "")
                as_ = acall.get("status", "")
                if es != as_:
                    return f"block[{i}].call[{j}] status: expected={es} got={as_}"
        return None
    else:
        ec = expected.get("error", {}).get("code", 0)
        ac = actual.get("error", {}).get("code", 0)
        if ec != ac:
            return f"error code: expected={ec} got={ac}"
        return None


def _get_base_timestamp(endpoint):
    """Fetch the latest block's timestamp from the chain."""
    resp = _send_rpc(
        endpoint,
        json.dumps(
            {
                "jsonrpc": "2.0",
                "id": 99,
                "method": "eth_getBlockByNumber",
                "params": ["latest", False],
            }
        ),
    )
    return int(resp["result"]["timestamp"], 16)


def _adjust_timestamps(request_body, base_timestamp):
    """Offset explicit blockOverrides.time values so they are above base_timestamp.

    Spec files were authored against geth whose blocks have very low
    timestamps (~0-50).  Ethermint uses real wall-clock timestamps.  If
    any explicit timestamp in the request is at or below the base block
    timestamp, we shift ALL explicit timestamps upward by a constant
    delta so that relative ordering between blocks is preserved while
    the first timestamp exceeds the base.
    """
    req = json.loads(request_body)
    params = req.get("params", [])
    if not params or not isinstance(params[0], dict):
        return request_body

    block_state_calls = params[0].get("blockStateCalls", [])
    explicit_times = []
    for bsc in block_state_calls:
        bo = bsc.get("blockOverrides") or {}
        t = bo.get("time")
        if t is not None:
            explicit_times.append(int(t, 16) if isinstance(t, str) else t)

    if not explicit_times:
        return request_body

    min_time = min(explicit_times)
    if min_time > base_timestamp:
        return request_body

    delta = base_timestamp - min_time + 12

    for bsc in block_state_calls:
        bo = bsc.get("blockOverrides") or {}
        t = bo.get("time")
        if t is not None:
            old_val = int(t, 16) if isinstance(t, str) else t
            bo["time"] = hex(old_val + delta)
            bsc["blockOverrides"] = bo

    return json.dumps(req)


@pytest.fixture(scope="module")
def rpc_endpoint(ethermint):
    """Wait for the chain to reach block 45, then return the JSON-RPC URL."""
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
def base_timestamp(rpc_endpoint):
    """Cache the base block timestamp once per module."""
    return _get_base_timestamp(rpc_endpoint)


@pytest.mark.parametrize("spec_name", SPEC_FILES)
def test_eth_simulate_spec(rpc_endpoint, base_timestamp, spec_name):
    request_body, expected_body = _parse_spec_file(spec_name)
    assert request_body, f"no request line (>> ...) in {spec_name}.io"
    assert expected_body, f"no expected response line (<< ...) in {spec_name}.io"

    expected = json.loads(expected_body)

    if "result" in expected:
        request_body = _adjust_timestamps(request_body, base_timestamp)
    actual = _send_rpc(rpc_endpoint, request_body)

    mismatch = _compare_results(expected, actual)
    assert mismatch is None, f"{spec_name}: {mismatch}"

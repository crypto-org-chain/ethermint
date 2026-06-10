"""Integration tests for txpool_content and txpool_contentFrom."""
import os
import signal
import subprocess
import time
from pathlib import Path

import pytest
import web3
from web3 import Web3
from web3.middleware import ExtraDataToPOAMiddleware

from .network import Geth, setup_custom_ethermint
from .utils import ADDRS, KEYS, sign_transaction, w3_wait_for_new_blocks, wait_for_port


@pytest.fixture(scope="module")
def custom_ethermint(tmp_path_factory):
    path = tmp_path_factory.mktemp("txpool")
    cfg = Path(__file__).parent / "configs/txpool.jsonnet"
    yield from setup_custom_ethermint(path, 26850, cfg)


@pytest.fixture(scope="module")
def txpool_geth(tmp_path_factory):
    path = tmp_path_factory.mktemp("txpool_geth")
    port = 8646
    with (path / "geth.log").open("w") as logfile:
        cmd = [
            "start-geth",
            str(path),
            "--http.port",
            str(port),
            "--port",
            str(port + 1),
            "--miner.etherbase",
            "0x57f96e6B86CdeFdB3d412547816a82E3E0EbF9D2",
            "--http.api",
            "eth,net,web3,debug,txpool",
        ]
        proc = subprocess.Popen(
            cmd,
            preexec_fn=os.setsid,
            stdout=logfile,
            stderr=subprocess.STDOUT,
        )
        try:
            wait_for_port(port)
            w3 = web3.Web3(web3.providers.HTTPProvider(f"http://127.0.0.1:{port}"))
            w3.middleware_onion.inject(ExtraDataToPOAMiddleware, layer=0)
            yield Geth(w3)
        finally:
            os.killpg(os.getpgid(proc.pid), signal.SIGTERM)
            proc.wait()


def assert_pending_rpc_tx_schema(tx, label=""):
    """Assert the required pending RPCTransaction fields are Geth-compatible."""
    null_in_pending = ("blockHash", "blockNumber", "transactionIndex")
    hex_fields = (
        "gas",
        "gasPrice",
        "hash",
        "input",
        "nonce",
        "value",
        "type",
        "v",
        "r",
        "s",
    )
    str_fields = ("from",)

    prefix = f"{label}: " if label else ""
    required = set(null_in_pending) | set(hex_fields) | set(str_fields)
    missing = required - set(tx)
    assert not missing, f"{prefix}missing keys in pending RPCTransaction: {missing}"

    for key in null_in_pending:
        assert tx[key] is None, (
            f"{prefix}'{key}' must be null for a pending tx, got {tx[key]!r}"
        )

    for key in hex_fields:
        assert isinstance(tx[key], str), (
            f"{prefix}'{key}' must be a string, got {type(tx[key]).__name__}"
        )
        assert tx[key].startswith("0x"), (
            f"{prefix}'{key}' must be 0x-prefixed, got {tx[key]!r}"
        )

    for key in str_fields:
        assert isinstance(tx[key], str), (
            f"{prefix}'{key}' must be a string, got {type(tx[key]).__name__}"
        )
        assert tx[key].startswith("0x"), f"{prefix}'{key}' must be an 0x address"


def poll_content_for_sender(rpc, sender_lower, timeout=2.0):
    deadline = time.time() + timeout
    while time.time() < deadline:
        rsp = rpc.make_request("txpool_content", [])
        result = rsp.get("result", {})
        pending = {k.lower(): v for k, v in result.get("pending", {}).items()}
        if sender_lower in pending and pending[sender_lower]:
            return result
        time.sleep(0.1)
    return None


def poll_content_from(rpc, sender_addr, timeout=2.0):
    deadline = time.time() + timeout
    while time.time() < deadline:
        rsp = rpc.make_request("txpool_contentFrom", [sender_addr])
        result = rsp.get("result", {})
        if result.get("pending"):
            return result
        time.sleep(0.1)
    return None


def test_txpool_content_empty_matches_geth(custom_ethermint, txpool_geth):
    """Empty-pool response must be {pending: {}, queued: {}} on both nodes."""
    eth_rpc = custom_ethermint.w3.provider
    geth_rpc = txpool_geth.w3.provider
    w3_wait_for_new_blocks(custom_ethermint.w3, 1)

    for node, rpc in [("ethermint", eth_rpc), ("geth", geth_rpc)]:
        rsp = rpc.make_request("txpool_content", [])
        assert "result" in rsp, f"{node} txpool_content returned an error: {rsp}"
        result = rsp["result"]
        assert isinstance(result, dict), f"{node}: result must be a dict"
        keys = set(result.keys())
        assert keys == {"pending", "queued"}, (
            f"{node}: expected top-level keys {{pending, queued}}, got {keys}"
        )
        assert isinstance(result["pending"], dict), f"{node}: 'pending' must be a dict"
        assert isinstance(result["queued"], dict), f"{node}: 'queued' must be a dict"
        assert result["pending"] == {}, f"{node}: expected empty pending on idle node"
        assert result["queued"] == {}, f"{node}: expected empty queued on idle node"


def test_txpool_content_pending_schema(custom_ethermint):
    """txpool_content pending entry must expose required RPCTransaction fields."""
    w3 = custom_ethermint.w3
    eth_rpc = w3.provider

    signed = sign_transaction(
        w3, {"to": ADDRS["community"], "value": 100}, key=KEYS["validator"]
    )
    txhash = w3.eth.send_raw_transaction(signed.raw_transaction)
    sender = ADDRS["validator"].lower()

    result = poll_content_for_sender(eth_rpc, sender)
    assert result is not None, (
        f"tx {Web3.to_hex(txhash)} never appeared in txpool_content pending"
    )

    pending = {k.lower(): v for k, v in result["pending"].items()}
    nonce_map = pending[sender]
    assert nonce_map, "nonce map for sender must be non-empty"

    for nonce_str, tx in nonce_map.items():
        assert nonce_str.isdigit(), (
            f"nonce key must be a decimal string, got {nonce_str!r}"
        )
        assert_pending_rpc_tx_schema(tx, label=f"nonce={nonce_str}")
        assert tx["from"].lower() == sender

    w3.eth.wait_for_transaction_receipt(txhash, timeout=30)


def test_txpool_content_from_filters_by_sender(custom_ethermint):
    """txpool_contentFrom must return only the requested sender's pending txs."""
    w3 = custom_ethermint.w3
    eth_rpc = w3.provider
    sender = ADDRS["validator"]

    signed = sign_transaction(
        w3, {"to": ADDRS["community"], "value": 200}, key=KEYS["validator"]
    )
    txhash = w3.eth.send_raw_transaction(signed.raw_transaction)

    result = poll_content_from(eth_rpc, sender)
    assert result is not None, (
        f"tx {Web3.to_hex(txhash)} never appeared in txpool_contentFrom"
    )

    assert set(result.keys()) == {"pending", "queued"}, (
        f"contentFrom must return {{pending, queued}}, got {set(result.keys())}"
    )
    assert isinstance(result["queued"], dict)

    for nonce_str, tx in result["pending"].items():
        assert nonce_str.isdigit(), (
            f"nonce key must be a decimal string, got {nonce_str!r}"
        )
        assert_pending_rpc_tx_schema(tx, label=f"nonce={nonce_str}")
        assert tx["from"].lower() == sender.lower()

    w3.eth.wait_for_transaction_receipt(txhash, timeout=30)


def test_txpool_content_from_unknown_address(custom_ethermint):
    """txpool_contentFrom for an address with no pending txs returns empty buckets."""
    eth_rpc = custom_ethermint.w3.provider
    unknown = "0x0000000000000000000000000000000000000001"
    rsp = eth_rpc.make_request("txpool_contentFrom", [unknown])
    assert "result" in rsp, f"txpool_contentFrom error: {rsp}"
    result = rsp["result"]
    assert set(result.keys()) == {"pending", "queued"}
    assert result["pending"] == {}
    assert result["queued"] == {}

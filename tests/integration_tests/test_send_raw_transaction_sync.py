import time
from concurrent.futures import ThreadPoolExecutor
from pathlib import Path

import pytest
from web3 import Web3

from .network import setup_custom_ethermint
from .utils import (
    ADDRS,
    CONTRACTS,
    KEYS,
    create_contract_transaction,
    derive_random_account,
    sign_transaction,
    w3_wait_for_new_blocks,
)

METHOD = "eth_sendRawTransactionSync"
ERR_CODE_TIMEOUT = 4
ERR_CODE_LIMIT_EXCEEDED = -32005
ERR_CODE_METHOD_NOT_FOUND = -32601
# matches max-receipt-waiters in configs/tx_sync.jsonnet
MAX_RECEIPT_WAITERS = 2


@pytest.fixture(scope="module")
def custom_ethermint(tmp_path_factory):
    path = tmp_path_factory.mktemp("tx_sync")
    cfg = Path(__file__).parent / "configs/tx_sync.jsonnet"
    yield from setup_custom_ethermint(path, 27200, cfg)


@pytest.fixture(scope="module")
def w3(custom_ethermint):
    node = custom_ethermint.copy()
    node.use_websocket(False)
    yield node.w3


def send_sync(w3, raw, *timeout_ms):
    return w3.provider.make_request(METHOD, [Web3.to_hex(raw), *timeout_ms])


def signed_transfer(w3, key, value=1000, **tx):
    return sign_transaction(w3, {"to": ADDRS["community"], "value": value, **tx}, key)


def get_receipt(w3, tx_hash):
    return w3.provider.make_request("eth_getTransactionReceipt", [tx_hash])["result"]


def wait_for_block_start(w3):
    """Return right after a commit so a submitted tx waits a full 3s block."""
    w3_wait_for_new_blocks(w3, 1, sleep=0.1)


def test_returns_receipt(w3):
    sender = ADDRS["signer1"]
    balance = w3.eth.get_balance(ADDRS["community"])
    signed = signed_transfer(w3, KEYS["signer1"])

    rsp = send_sync(w3, signed.raw_transaction)
    assert "error" not in rsp, rsp
    receipt = rsp["result"]
    tx_hash = Web3.to_hex(signed.hash)
    assert receipt["transactionHash"] == tx_hash
    assert receipt["status"] == "0x1"
    assert receipt["from"].lower() == sender.lower()
    assert receipt["blockNumber"] is not None
    assert receipt == get_receipt(w3, tx_hash)
    assert w3.eth.get_balance(ADDRS["community"]) == balance + 1000


def test_explicit_timeout(w3):
    signed = signed_transfer(w3, KEYS["signer1"])
    rsp = send_sync(w3, signed.raw_transaction, 10000)
    assert "error" not in rsp, rsp
    assert rsp["result"]["transactionHash"] == Web3.to_hex(signed.hash)
    assert rsp["result"]["status"] == "0x1"


def test_contract_creation(w3):
    tx = create_contract_transaction(w3, CONTRACTS["TestRevert"], key=KEYS["signer1"])
    signed = sign_transaction(w3, tx, KEYS["signer1"])

    rsp = send_sync(w3, signed.raw_transaction)
    assert "error" not in rsp, rsp
    receipt = rsp["result"]
    assert receipt["status"] == "0x1"
    assert receipt["contractAddress"] is not None
    assert len(w3.eth.get_code(receipt["contractAddress"])) > 0


def test_reverted_tx_returns_receipt(w3):
    tx = create_contract_transaction(w3, CONTRACTS["TestRevert"], key=KEYS["signer1"])
    rsp = send_sync(w3, sign_transaction(w3, tx, KEYS["signer1"]).raw_transaction)
    contract = rsp["result"]["contractAddress"]

    # revertWithMsg(); explicit gas skips estimation, which would fail on revert
    tx = {"to": contract, "data": "0x9ffb86a5", "gas": 100000}
    signed = sign_transaction(w3, tx, KEYS["signer1"])
    rsp = send_sync(w3, signed.raw_transaction)
    assert "error" not in rsp, rsp
    assert rsp["result"]["transactionHash"] == Web3.to_hex(signed.hash)
    assert rsp["result"]["status"] == "0x0"


def test_resend_included_tx_returns_receipt(w3):
    signed = signed_transfer(w3, KEYS["signer1"])
    first = send_sync(w3, signed.raw_transaction)
    assert "error" not in first, first

    second = send_sync(w3, signed.raw_transaction)
    assert "error" not in second, second
    assert second["result"] == first["result"]


def test_timeout_returns_tx_hash(w3):
    signed = signed_transfer(w3, KEYS["signer1"])
    tx_hash = Web3.to_hex(signed.hash)

    wait_for_block_start(w3)
    start = time.monotonic()
    rsp = send_sync(w3, signed.raw_transaction, 500)
    elapsed = time.monotonic() - start

    assert "result" not in rsp, rsp
    assert rsp["error"]["code"] == ERR_CODE_TIMEOUT, rsp
    assert rsp["error"]["data"] == tx_hash
    assert elapsed < 2.5, f"waited {elapsed:.2f}s for a 500ms timeout"

    receipt = w3.eth.wait_for_transaction_receipt(signed.hash, timeout=20)
    assert receipt.status == 1


def test_rejected_tx_returns_error(w3):
    acct = derive_random_account()
    assert w3.eth.get_balance(acct.address) == 0
    # explicit gas skips estimation, which already fails for an unfunded sender
    signed = signed_transfer(w3, acct.key, gas=21000)

    start = time.monotonic()
    rsp = send_sync(w3, signed.raw_transaction)
    elapsed = time.monotonic() - start

    assert "result" not in rsp, rsp
    assert rsp["error"]["code"] != ERR_CODE_TIMEOUT, rsp
    assert "insufficient funds" in rsp["error"]["message"], rsp
    assert elapsed < 5, f"rejected tx took {elapsed:.2f}s"


def test_invalid_payload_returns_error(w3):
    rsp = send_sync(w3, b"\x01\x02\x03")
    assert "result" not in rsp, rsp
    assert rsp["error"]["code"] != ERR_CODE_TIMEOUT, rsp


def test_websocket_rejected_before_submit(custom_ethermint):
    node = custom_ethermint.copy()
    node.use_websocket(True)
    ws = node.w3
    signed = signed_transfer(ws, KEYS["signer1"])

    rsp = send_sync(ws, signed.raw_transaction)
    assert "result" not in rsp, rsp
    assert rsp["error"]["code"] == ERR_CODE_METHOD_NOT_FOUND, rsp
    assert get_receipt(ws, Web3.to_hex(signed.hash)) is None


def test_max_receipt_waiters(custom_ethermint):
    endpoint = custom_ethermint.w3_http_endpoint
    w3 = Web3(Web3.HTTPProvider(endpoint))
    holders = ["validator", "signer2"][:MAX_RECEIPT_WAITERS]
    signed_holders = [signed_transfer(w3, KEYS[name]) for name in holders]
    rejected = signed_transfer(w3, KEYS["community"])
    rejected_nonce = w3.eth.get_transaction_count(ADDRS["community"], "pending")

    def call(signed):
        return send_sync(Web3(Web3.HTTPProvider(endpoint)), signed.raw_transaction)

    wait_for_block_start(w3)
    with ThreadPoolExecutor(MAX_RECEIPT_WAITERS) as executor:
        tasks = [executor.submit(call, signed) for signed in signed_holders]
        # let the holders take every slot while their txs wait for the next block
        time.sleep(0.5)
        rsp = call(rejected)
        results = [t.result() for t in tasks]

    assert "result" not in rsp, rsp
    assert rsp["error"]["code"] == ERR_CODE_LIMIT_EXCEEDED, rsp
    for signed, res in zip(signed_holders, results):
        assert "error" not in res, res
        assert res["result"]["transactionHash"] == Web3.to_hex(signed.hash)
        assert res["result"]["status"] == "0x1"

    assert w3.eth.get_transaction_count(ADDRS["community"], "pending") == rejected_nonce
    assert get_receipt(w3, Web3.to_hex(rejected.hash)) is None

    rsp = send_sync(w3, rejected.raw_transaction)
    assert "error" not in rsp, rsp
    assert rsp["result"]["status"] == "0x1"

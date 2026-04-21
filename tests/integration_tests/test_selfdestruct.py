from concurrent.futures import ThreadPoolExecutor

from web3 import Web3

from .utils import ADDRS, CONTRACTS, KEYS, deploy_contract, send_transaction

# ---------------------------------------------------------------------------
# Shared helpers
# ---------------------------------------------------------------------------


def _run(w3: Web3, salt: bytes, value: int):
    factory, deploy_receipt = deploy_contract(
        w3, CONTRACTS["SelfDestructExploitFactory"]
    )
    assert deploy_receipt.status == 1

    child_addr = Web3.to_checksum_address(
        factory.functions.predictChildAddress(salt).call(
            block_identifier=deploy_receipt.blockNumber
        )
    )
    assert w3.eth.get_balance(child_addr) == 0

    receipt = send_transaction(
        w3,
        factory.functions.attackInOneTx(salt).build_transaction(
            {"from": ADDRS["validator"], "value": value}
        ),
        KEYS["validator"],
    )
    assert receipt.status == 1, "transaction should succeed"
    return factory, child_addr, receipt


def _trace_tx(w3: Web3, tx_hash, tracer: dict) -> dict:
    result = w3.provider.make_request(
        "debug_traceTransaction",
        [Web3.to_hex(tx_hash), tracer],
    )
    assert "result" in result, f"trace error: {result.get('error')}"
    return result["result"]


def _find_frames(frame: dict, match_type: str) -> list:
    """Recursively collect all callTracer frames with a given type."""
    found = []
    if frame.get("type") == match_type:
        found.append(frame)
    for sub in frame.get("calls", []):
        found.extend(_find_frames(sub, match_type))
    return found


def test_selfdestruct_post_destruction_balance_burned(ethermint, geth):
    salt = bytes(31) + b"\x01"
    value = 10**9

    def process(w3):
        _, child_addr, _ = _run(w3, salt, value)
        return {
            "child_balance": w3.eth.get_balance(child_addr),
            "child_code": w3.eth.get_code(child_addr),
            "child_addr": child_addr,
        }

    with ThreadPoolExecutor(2) as pool:
        futs = [pool.submit(process, w3) for w3 in [ethermint.w3, geth.w3]]
        results = {name: f.result() for name, f in zip(["ethermint", "geth"], futs)}

    for name, r in results.items():
        assert r["child_balance"] == 0, (
            f"{name}: post-selfdestruct balance must be 0 (burned at commit), "
            f"but eth_getBalance({r['child_addr']}) returned {r['child_balance']}."
        )
        assert (
            r["child_code"] == b""
        ), f"{name}: self-destructed account must have no code after commit."


def test_selfdestruct_recreated_address_cannot_recover_funds(ethermint, geth):
    """
    Recreating the child at the same CREATE2 address must not expose any
    preserved balance to the new contract.
    """
    salt = bytes(31) + b"\x02"
    value = 10**9

    def process(w3):
        factory, child_addr, _ = _run(w3, salt, value)
        assert w3.eth.get_balance(child_addr) == 0

        validator_balance_before = w3.eth.get_balance(ADDRS["validator"])

        redeploy_receipt = send_transaction(
            w3,
            factory.functions.redeployChild(salt).build_transaction(
                {"from": ADDRS["validator"]}
            ),
            KEYS["validator"],
        )
        assert redeploy_receipt.status == 1

        return {
            "child_balance_after_redeploy": w3.eth.get_balance(child_addr),
            "validator_gained": w3.eth.get_balance(ADDRS["validator"])
            > validator_balance_before,
            "child_addr": child_addr,
        }

    with ThreadPoolExecutor(2) as pool:
        futs = [pool.submit(process, w3) for w3 in [ethermint.w3, geth.w3]]
        results = {name: f.result() for name, f in zip(["ethermint", "geth"], futs)}

    for name, r in results.items():
        assert r["child_balance_after_redeploy"] == 0, (
            f"{name}: redeployed child must have 0 balance"
            f"(child={r['child_addr']})."
        )
        assert not r[
            "validator_gained"
        ], f"{name}: validator must not gain funds from the recovery attempt."


def test_selfdestruct_calltrace_parity(ethermint, geth):
    salt = bytes(31) + b"\x11"
    value = 10**9

    def process(w3):
        factory, child_addr, receipt = _run(w3, salt, value)
        trace = _trace_tx(w3, receipt.transactionHash, {"tracer": "callTracer"})

        # --- SELFDESTRUCT frame ---
        sd_frames = _find_frames(trace, "SELFDESTRUCT")
        assert (
            len(sd_frames) >= 1
        ), f"Expected at least one SELFDESTRUCT frame, got: {sd_frames}"
        sd = sd_frames[0]
        assert (
            sd["from"].lower() == child_addr.lower()
        ), f"SELFDESTRUCT must originate from the child contract, got from={sd['from']}"
        # Child had 0 balance at destruction time (no ETH was sent before destroy()).
        assert sd.get("value", "0x0") in ("0x0", "0x00", None, ""), (
            f"SELFDESTRUCT value must be 0 (child had no pre-existing balance), "
            f"got value={sd.get('value')}"
        )

        # --- Post-destruction CALL frame (value-bearing) ---
        post_sd_calls = [
            c
            for c in _find_frames(trace, "CALL")
            if c.get("to", "").lower() == child_addr.lower()
            and int(c.get("value", "0x0"), 16) == value
        ]
        assert len(post_sd_calls) >= 1, (
            f"Expected a value-bearing CALL to dead child ({child_addr}) "
            f"with value={hex(value)}, found none in trace."
        )

        return {
            "selfdestruct_from": sd["from"].lower(),
            "selfdestruct_to": sd.get("to", "").lower(),
            "post_sd_call_value": hex(value),
            "child_addr": child_addr.lower(),
        }

    with ThreadPoolExecutor(2) as pool:
        futs = [pool.submit(process, w3) for w3 in [ethermint.w3, geth.w3]]
        results = {name: f.result() for name, f in zip(["ethermint", "geth"], futs)}

    def _normalize(r):
        return {
            "selfdestruct_from_is_child": r["selfdestruct_from"] == r["child_addr"],
            "selfdestruct_to_is_set": r["selfdestruct_to"] != "",
            "post_sd_call_value": r["post_sd_call_value"],
        }

    assert _normalize(results["ethermint"]) == _normalize(results["geth"]), (
        f"callTracer key fields differ between Ethermint and Geth:\n"
        f"  Ethermint: {results['ethermint']}\n"
        f"  Geth:      {results['geth']}"
    )


def test_selfdestruct_prestate_diff_parity(ethermint, geth):
    """
    Verify that debug_traceTransaction (prestateTracer diffMode) shows
    identical pre/post state for the child address on Ethermint and Geth.

    The burn of post-selfdestruct balance (BalanceDecreaseSelfdestructBurn,
    reason=14) is NOT a named entry in the diff — it is the ABSENCE of a
    balance entry for the child in the post-state.  Both nodes must agree that
    the child has no entry (i.e. no surviving balance) after the transaction.
    """
    salt = bytes(31) + b"\x12"
    value = 10**9

    def process(w3):
        _, child_addr, receipt = _run(w3, salt, value)
        diff = _trace_tx(
            w3,
            receipt.transactionHash,
            {"tracer": "prestateTracer", "tracerConfig": {"diffMode": True}},
        )
        post = diff.get("post", {})
        child_lower = child_addr.lower()
        child_post = post.get(child_lower, {})
        # With the fix (or on Geth): child must have 0 or absent balance in post-state.
        child_post_balance = int(child_post.get("balance", "0x0"), 16)
        return child_post_balance, child_addr.lower()

    with ThreadPoolExecutor(2) as pool:
        futs = [pool.submit(process, w3) for w3 in [ethermint.w3, geth.w3]]
        results = {name: f.result() for name, f in zip(["ethermint", "geth"], futs)}

    bal_ethermint, addr_ethermint = results["ethermint"]
    bal_geth, _ = results["geth"]

    assert bal_geth == 0, f"Geth: child post-state balance must be 0, got {bal_geth}"
    assert bal_ethermint == 0, (
        f"Ethermint: child post-state balance must be 0 (burn fix active), "
        f"got {bal_ethermint} for {addr_ethermint}. "
        f"The SELFDESTRUCT bank-balance drain is not active."
    )

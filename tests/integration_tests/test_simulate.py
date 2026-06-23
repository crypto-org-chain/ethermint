from concurrent.futures import ThreadPoolExecutor

import pytest  # type: ignore[import-not-found]

from .utils import ADDRS, KEYS, send_transaction

METHOD = "eth_simulateV1"

pytestmark = pytest.mark.filter

# Test addresses
SENDER = "0xc000000000000000000000000000000000000000"
RECEIVER = "0xc100000000000000000000000000000000000000"
RECEIVER2 = "0xc200000000000000000000000000000000000000"


def execute(ethermint, geth, params):
    """Run eth_simulateV1 on both ethermint and geth, return results."""

    def process(w3):
        return w3.provider.make_request(METHOD, params)

    providers = [ethermint.w3, geth.w3]
    with ThreadPoolExecutor(len(providers)) as exc:
        tasks = [exc.submit(process, w3) for w3 in providers]
        results = [t.result() for t in tasks]
    return results


def test_simulate_does_not_affect_real_state(ethermint):
    """Simulation must not mutate on-chain state."""
    w3 = ethermint.w3
    sender_addr = ADDRS["validator"]
    recipient_addr = ADDRS["community"]

    receipt = send_transaction(
        w3,
        {"to": recipient_addr, "value": 1000},
        KEYS["validator"],
    )
    assert receipt.status == 1, f"real tx failed: {receipt}"

    sender_balance = w3.eth.get_balance(sender_addr, "latest")
    recipient_balance = w3.eth.get_balance(recipient_addr, "latest")

    sim_params = [
        {
            "blockStateCalls": [
                {
                    "calls": [
                        {
                            "from": sender_addr,
                            "to": recipient_addr,
                            "value": "0x1",
                        }
                    ],
                }
            ]
        },
        "latest",
    ]
    sim_result = w3.provider.make_request(METHOD, sim_params)
    assert "result" in sim_result, f"simulate failed: {sim_result.get('error')}"

    sim_call = sim_result["result"][0]["calls"][0]
    assert (
        sim_call["status"] == "0x1"
    ), f"simulated call failed: {sim_call.get('error')}"

    assert (
        w3.eth.get_balance(sender_addr, "latest") == sender_balance
    ), "sender balance changed after simulation — simulate mutated real state"
    assert (
        w3.eth.get_balance(recipient_addr, "latest") == recipient_balance
    ), "recipient balance changed after simulation — simulate mutated real state"


def test_simulate_simple_transfer(ethermint, geth):
    """Basic ETH transfer with balance override."""
    params = [
        {
            "blockStateCalls": [
                {
                    "stateOverrides": {
                        SENDER: {"balance": "0x3e8"},
                    },
                    "calls": [
                        {
                            "from": SENDER,
                            "to": RECEIVER,
                            "value": "0x3e8",
                        }
                    ],
                }
            ]
        },
        "latest",
    ]
    results = execute(ethermint, geth, params)
    eth_res, geth_res = results

    assert "result" in eth_res, f"ethermint error: {eth_res.get('error')}"
    assert "result" in geth_res, f"geth error: {geth_res.get('error')}"

    eth_block = eth_res["result"][0]
    geth_block = geth_res["result"][0]

    eth_call = eth_block["calls"][0]
    geth_call = geth_block["calls"][0]
    assert eth_call["status"] == geth_call["status"] == "0x1"
    assert eth_call["gasUsed"] == geth_call["gasUsed"]
    assert eth_call["returnData"] == geth_call["returnData"]
    assert eth_call["logs"] == geth_call["logs"] == []


def test_simulate_multi_block(ethermint, geth):
    """State carries across blocks."""
    params = [
        {
            "blockStateCalls": [
                {
                    "stateOverrides": {
                        SENDER: {"balance": "0xde0b6b3a7640000"},
                    },
                    "calls": [
                        {
                            "from": SENDER,
                            "to": RECEIVER,
                            "value": "0x1",
                        },
                        {
                            "from": SENDER,
                            "to": RECEIVER2,
                            "value": "0x1",
                        },
                    ],
                },
                {
                    "calls": [
                        {
                            "from": RECEIVER,
                            "to": RECEIVER2,
                            "value": "0x1",
                        },
                    ],
                },
            ]
        },
        "latest",
    ]
    results = execute(ethermint, geth, params)
    eth_res, geth_res = results

    assert "result" in eth_res, f"ethermint error: {eth_res.get('error')}"
    assert "result" in geth_res, f"geth error: {geth_res.get('error')}"

    # Both should have 2 blocks
    assert len(eth_res["result"]) == len(geth_res["result"]) == 2

    # All calls should succeed
    for bi in range(2):
        for ci in range(len(eth_res["result"][bi]["calls"])):
            assert (
                eth_res["result"][bi]["calls"][ci]["status"]
                == geth_res["result"][bi]["calls"][ci]["status"]
                == "0x1"
            )
            assert (
                eth_res["result"][bi]["calls"][ci]["gasUsed"]
                == geth_res["result"][bi]["calls"][ci]["gasUsed"]
            )


def test_simulate_insufficient_funds(ethermint, geth):
    """Transfer without balance override in validation mode -> error."""
    params = [
        {
            "blockStateCalls": [
                {
                    "calls": [
                        {
                            "from": SENDER,
                            "to": RECEIVER,
                            "value": "0x3e8",
                            "nonce": "0x0",
                        }
                    ],
                }
            ],
            "validation": True,
        },
        "latest",
    ]
    results = execute(ethermint, geth, params)
    eth_res, geth_res = results

    # Both should return an error
    assert "error" in eth_res, f"ethermint succeeded unexpectedly: {eth_res}"
    assert "error" in geth_res, f"geth succeeded unexpectedly: {geth_res}"

    eth_err = eth_res["error"]
    geth_err = geth_res["error"]
    assert eth_err["code"] == geth_err["code"]


def test_simulate_block_overrides(ethermint, geth):
    """NUMBER opcode in default simulate context returns a positive block (parity)."""
    # No blockOverrides here — verifies default simulated block.number > 0 vs geth.
    # Bytecode: NUMBER PUSH0 MSTORE PUSH1 0x20 PUSH0 RETURN
    # 43 5f 52 6020 5f f3
    code = "0x435f5260205ff3"
    params = [
        {
            "blockStateCalls": [
                {
                    "stateOverrides": {
                        RECEIVER: {"code": code},
                    },
                    "calls": [
                        {
                            "from": SENDER,
                            "to": RECEIVER,
                        }
                    ],
                }
            ]
        },
        "latest",
    ]
    results = execute(ethermint, geth, params)
    eth_res, geth_res = results

    assert "result" in eth_res, f"ethermint error: {eth_res.get('error')}"
    assert "result" in geth_res, f"geth error: {geth_res.get('error')}"

    eth_call = eth_res["result"][0]["calls"][0]
    geth_call = geth_res["result"][0]["calls"][0]

    assert eth_call["status"] == geth_call["status"] == "0x1", (
        f"eth status={eth_call['status']} error={eth_call.get('error')}, "
        f"geth status={geth_call['status']} error={geth_call.get('error')}"
    )
    # returnData should contain a block number > 0
    assert int(eth_call["returnData"], 16) > 0
    assert int(geth_call["returnData"], 16) > 0


def test_simulate_block_overrides_number_opcode(ethermint, geth):
    """blockOverrides.number is what NUMBER returns; ethermint matches geth."""
    code = "0x435f5260205ff3"

    def process(w3):
        current = int(w3.provider.make_request("eth_blockNumber", [])["result"], 16)
        target = current + 1
        params = [
            {
                "blockStateCalls": [
                    {
                        "blockOverrides": {"number": hex(target)},
                        "stateOverrides": {RECEIVER: {"code": code}},
                        "calls": [{"from": SENDER, "to": RECEIVER}],
                    }
                ]
            },
            hex(current),  # pin to the block we read, not "latest" which may advance
        ]
        return w3.provider.make_request(METHOD, params), target

    providers = [ethermint.w3, geth.w3]
    with ThreadPoolExecutor(len(providers)) as exc:
        tasks = [exc.submit(process, w3) for w3 in providers]
        (eth_res, eth_target), (geth_res, geth_target) = [t.result() for t in tasks]

    assert "result" in eth_res, f"ethermint error: {eth_res.get('error')}"
    assert "result" in geth_res, f"geth error: {geth_res.get('error')}"

    def get_first_call(sim_result):
        for block in sim_result["result"]:
            calls = block.get("calls") or []
            if calls:
                return calls[0]
        raise AssertionError(f"no calls in simulate result: {sim_result!r}")

    eth_data = get_first_call(eth_res)["returnData"]
    geth_data = get_first_call(geth_res)["returnData"]
    assert int(eth_data, 16) == eth_target
    assert int(geth_data, 16) == geth_target


def test_simulate_block_number_order(ethermint, geth):
    """Block 12 then block 11 -> error."""
    params = [
        {
            "blockStateCalls": [
                {
                    "blockOverrides": {"number": "0xc"},
                },
                {
                    "blockOverrides": {"number": "0xb"},
                },
            ]
        },
        "latest",
    ]
    results = execute(ethermint, geth, params)
    eth_res, geth_res = results

    assert "error" in eth_res
    assert "error" in geth_res
    assert eth_res["error"]["code"] == geth_res["error"]["code"]


def test_simulate_storage_contract(ethermint, geth):
    """Override code, store(5) then retrieve() across calls."""
    # Runtime code: if calldatasize > 0, store calldataload(0) at slot 0, then stop
    # Else: load slot 0 and return 32 bytes
    #
    # CALLDATASIZE ISZERO PUSH1 0x0a JUMPI   (36 15 600a 57)
    # PUSH0 CALLDATALOAD PUSH0 SSTORE STOP   (5f 35 5f 55 00)
    # JUMPDEST PUSH0 SLOAD PUSH0 MSTORE PUSH1 0x20 PUSH0 RETURN
    # (5b 5f 54 5f 52 6020 5f f3)
    runtime = "0x3615600a575f355f55005b5f545f5260205ff3"
    data = "0x0000000000000000000000000000000000000000000000000000000000000005"

    params = [
        {
            "blockStateCalls": [
                {
                    "stateOverrides": {
                        RECEIVER: {"code": runtime},
                    },
                    "calls": [
                        {
                            "from": SENDER,
                            "to": RECEIVER,
                            "data": data,
                        },
                        {
                            "from": SENDER,
                            "to": RECEIVER,
                        },
                    ],
                }
            ]
        },
        "latest",
    ]
    results = execute(ethermint, geth, params)
    eth_res, geth_res = results

    assert "result" in eth_res, f"ethermint error: {eth_res.get('error')}"
    assert "result" in geth_res, f"geth error: {geth_res.get('error')}"

    # Both calls should succeed
    eth_calls = eth_res["result"][0]["calls"]
    geth_calls = geth_res["result"][0]["calls"]

    for i in range(2):
        assert eth_calls[i]["status"] == geth_calls[i]["status"] == "0x1", (
            f"call {i}: eth status={eth_calls[i]['status']} "
            f"error={eth_calls[i].get('error')}, "
            f"geth status={geth_calls[i]['status']} "
            f"error={geth_calls[i].get('error')}"
        )

    # Second call should return the stored value (5)
    assert eth_calls[1]["returnData"] == geth_calls[1]["returnData"]


def test_simulate_transfer_logs(ethermint, geth):
    """traceTransfers=true -> synthetic Transfer logs."""
    params = [
        {
            "blockStateCalls": [
                {
                    "stateOverrides": {
                        SENDER: {"balance": "0xde0b6b3a7640000"},
                    },
                    "calls": [
                        {
                            "from": SENDER,
                            "to": RECEIVER,
                            "value": "0x1",
                        }
                    ],
                }
            ],
            "traceTransfers": True,
        },
        "latest",
    ]
    results = execute(ethermint, geth, params)
    eth_res, geth_res = results

    assert "result" in eth_res, f"ethermint error: {eth_res.get('error')}"
    assert "result" in geth_res, f"geth error: {geth_res.get('error')}"

    eth_logs = eth_res["result"][0]["calls"][0]["logs"]
    geth_logs = geth_res["result"][0]["calls"][0]["logs"]

    # Should have synthetic Transfer log
    assert len(eth_logs) == len(geth_logs)
    assert len(eth_logs) > 0

    # Check the synthetic Transfer log
    eth_log = eth_logs[0]
    geth_log = geth_logs[0]

    # Address should be the ERC-7528 address
    expected_addr = "0xeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee"
    assert eth_log["address"].lower() == expected_addr
    assert geth_log["address"].lower() == expected_addr

    # Topics should match
    assert eth_log["topics"] == geth_log["topics"]
    assert eth_log["data"] == geth_log["data"]


def test_simulate_validation_nonce_error(ethermint, geth):
    """Nonce too high in validation mode -> error."""
    params = [
        {
            "blockStateCalls": [
                {
                    "stateOverrides": {
                        SENDER: {"balance": "0xde0b6b3a7640000"},
                    },
                    "calls": [
                        {
                            "from": SENDER,
                            "to": RECEIVER,
                            "nonce": "0xff",
                        }
                    ],
                }
            ],
            "validation": True,
        },
        "latest",
    ]
    results = execute(ethermint, geth, params)
    eth_res, geth_res = results

    assert "error" in eth_res, "ethermint succeeded unexpectedly"
    assert "error" in geth_res, "geth succeeded unexpectedly"

    assert eth_res["error"]["code"] == geth_res["error"]["code"]


def test_simulate_validation_success(ethermint, geth):
    """Valid tx with proper nonce and sufficient maxFeePerGas in validation mode."""
    # In validation mode, baseFee is calculated from parent. We must provide
    # a maxFeePerGas high enough to cover it and enough balance for gas * fee + value.
    params = [
        {
            "blockStateCalls": [
                {
                    "stateOverrides": {
                        # 100 ETH — enough for gas * maxFeePerGas + value
                        SENDER: {
                            "balance": "0x56bc75e2d63100000",
                            "nonce": "0x0",
                        },
                    },
                    "calls": [
                        {
                            "from": SENDER,
                            "to": RECEIVER,
                            "value": "0x1",
                            "nonce": "0x0",
                            "maxFeePerGas": "0x174876e800",
                            "maxPriorityFeePerGas": "0x0",
                        }
                    ],
                }
            ],
            "validation": True,
        },
        "latest",
    ]
    results = execute(ethermint, geth, params)
    eth_res, geth_res = results

    assert "result" in eth_res, f"ethermint error: {eth_res.get('error')}"
    assert "result" in geth_res, f"geth error: {geth_res.get('error')}"

    eth_call = eth_res["result"][0]["calls"][0]
    geth_call = geth_res["result"][0]["calls"][0]
    assert eth_call["status"] == geth_call["status"] == "0x1"
    assert eth_call["gasUsed"] == geth_call["gasUsed"]


def test_simulate_chain_linkage(ethermint, geth):
    """parentHash linkage across 3 blocks."""
    params = [
        {
            "blockStateCalls": [
                {},
                {},
                {},
            ]
        },
        "latest",
    ]
    results = execute(ethermint, geth, params)
    eth_res, geth_res = results

    assert "result" in eth_res, f"ethermint error: {eth_res.get('error')}"
    assert "result" in geth_res, f"geth error: {geth_res.get('error')}"

    assert len(eth_res["result"]) == len(geth_res["result"]) == 3

    # Check that block numbers are sequential (relative, not absolute)
    for res in [eth_res, geth_res]:
        blocks = res["result"]
        for i in range(1, len(blocks)):
            prev_num = int(blocks[i - 1]["number"], 16)
            cur_num = int(blocks[i]["number"], 16)
            assert (
                cur_num == prev_num + 1
            ), f"block numbers not sequential: {prev_num} -> {cur_num}"


def test_simulate_gap_fill(ethermint, geth):
    """Block number gap with small gap -> empty blocks inserted."""
    # Use relative block numbers: current+1 then current+4 = 2 gap blocks
    # We do NOT hardcode block numbers; use two separate requests with overrides
    # relative to the first block.

    def process(w3):
        # First get current block number
        current = int(w3.provider.make_request("eth_blockNumber", [])["result"], 16)
        n1 = hex(current + 1)
        n2 = hex(current + 4)  # gap of 2 empty blocks
        params = [
            {
                "blockStateCalls": [
                    {"blockOverrides": {"number": n1}},
                    {"blockOverrides": {"number": n2}},
                ]
            },
            hex(current),  # pin base block to avoid race with chain advancing
        ]
        return w3.provider.make_request(METHOD, params)

    providers = [ethermint.w3, geth.w3]
    with ThreadPoolExecutor(len(providers)) as exc:
        tasks = [exc.submit(process, w3) for w3 in providers]
        results = [t.result() for t in tasks]
    eth_res, geth_res = results

    assert "result" in eth_res, f"ethermint error: {eth_res.get('error')}"
    assert "result" in geth_res, f"geth error: {geth_res.get('error')}"

    # Should have 4 blocks: n1, n1+1 (gap), n1+2 (gap), n2
    assert len(eth_res["result"]) == len(geth_res["result"]) == 4


def test_simulate_basefee_non_validation(ethermint, geth):
    """BASEFEE=0 by default in non-validation mode, respects override."""
    # Bytecode: BASEFEE PUSH0 MSTORE PUSH1 0x20 PUSH0 RETURN
    # 48 5f 52 6020 5f f3
    code = "0x485f5260205ff3"

    params = [
        {
            "blockStateCalls": [
                {
                    "stateOverrides": {
                        RECEIVER: {"code": code},
                    },
                    "calls": [
                        {
                            "from": SENDER,
                            "to": RECEIVER,
                        }
                    ],
                }
            ]
        },
        "latest",
    ]
    results = execute(ethermint, geth, params)
    eth_res, geth_res = results

    assert "result" in eth_res, f"ethermint error: {eth_res.get('error')}"
    assert "result" in geth_res, f"geth error: {geth_res.get('error')}"

    eth_call = eth_res["result"][0]["calls"][0]
    geth_call = geth_res["result"][0]["calls"][0]
    assert eth_call["status"] == geth_call["status"] == "0x1", (
        f"eth status={eth_call['status']} error={eth_call.get('error')}, "
        f"geth status={geth_call['status']} error={geth_call.get('error')}"
    )
    # Base fee should be 0 in non-validation mode
    assert eth_call["returnData"] == geth_call["returnData"]
    assert int(eth_call["returnData"], 16) == 0


def test_simulate_precompile_override(ethermint, geth):
    """Override ecrecover (0x01) with custom bytecode."""
    # Override ecrecover with bytecode that returns 0x42
    # PUSH1 0x42 PUSH0 MSTORE PUSH1 0x20 PUSH0 RETURN
    # 6042 5f 52 6020 5f f3
    code = "0x60425f5260205ff3"
    ecrecover_addr = "0x0000000000000000000000000000000000000001"

    params = [
        {
            "blockStateCalls": [
                {
                    "stateOverrides": {
                        ecrecover_addr: {"code": code},
                    },
                    "calls": [
                        {
                            "from": SENDER,
                            "to": ecrecover_addr,
                        }
                    ],
                }
            ]
        },
        "latest",
    ]
    results = execute(ethermint, geth, params)
    eth_res, geth_res = results

    assert "result" in eth_res, f"ethermint error: {eth_res.get('error')}"
    assert "result" in geth_res, f"geth error: {geth_res.get('error')}"

    eth_call = eth_res["result"][0]["calls"][0]
    geth_call = geth_res["result"][0]["calls"][0]
    assert eth_call["status"] == geth_call["status"] == "0x1", (
        f"eth status={eth_call['status']} error={eth_call.get('error')}, "
        f"geth status={geth_call['status']} error={geth_call.get('error')}"
    )
    assert eth_call["returnData"] == geth_call["returnData"]
    assert int(eth_call["returnData"], 16) == 0x42


def test_simulate_block_schema_and_full_transactions(ethermint, geth):
    base_params = [
        {
            "blockStateCalls": [
                {
                    "stateOverrides": {
                        SENDER: {"balance": "0xde0b6b3a7640000"},
                    },
                    "calls": [
                        {
                            "from": SENDER,
                            "to": RECEIVER,
                            "value": "0x1",
                        }
                    ],
                }
            ]
        },
        "latest",
    ]
    eth_res, geth_res = execute(ethermint, geth, base_params)

    assert "result" in eth_res, f"ethermint error: {eth_res.get('error')}"
    assert "result" in geth_res, f"geth error: {geth_res.get('error')}"

    eth_block = eth_res["result"][0]
    geth_block = geth_res["result"][0]

    for key in [
        "hash",
        "parentHash",
        "stateRoot",
        "receiptsRoot",
        "transactionsRoot",
        "transactions",
        "difficulty",
    ]:
        assert key in eth_block
        assert key in geth_block

    assert len(eth_block["transactions"]) == len(geth_block["transactions"]) == 1

    full_params = [
        {
            "blockStateCalls": base_params[0]["blockStateCalls"],
            "returnFullTransactions": True,
        },
        "latest",
    ]
    eth_res, geth_res = execute(ethermint, geth, full_params)

    assert "result" in eth_res, f"ethermint error: {eth_res.get('error')}"
    assert "result" in geth_res, f"geth error: {geth_res.get('error')}"

    eth_tx = eth_res["result"][0]["transactions"][0]
    geth_tx = geth_res["result"][0]["transactions"][0]
    assert eth_tx["from"].lower() == geth_tx["from"].lower() == SENDER
    assert eth_tx["to"].lower() == geth_tx["to"].lower() == RECEIVER


def test_simulate_blockhash_uses_base_block(ethermint, geth):

    def process(w3):
        latest = w3.provider.make_request("eth_getBlockByNumber", ["latest", False])[
            "result"
        ]
        block_num = int(latest["number"], 16)
        code = f"0x7f{block_num:064x}405f5260205ff3"
        params = [
            {
                "blockStateCalls": [
                    {
                        "stateOverrides": {
                            RECEIVER: {"code": code},
                        },
                        "calls": [
                            {
                                "from": SENDER,
                                "to": RECEIVER,
                            }
                        ],
                    }
                ]
            },
            "latest",
        ]
        result = w3.provider.make_request(METHOD, params)
        expected = "0x" + latest["hash"][2:].rjust(64, "0").lower()
        return result, expected

    providers = [ethermint.w3, geth.w3]
    with ThreadPoolExecutor(len(providers)) as exc:
        tasks = [exc.submit(process, w3) for w3 in providers]
        (eth_res, eth_hash), (geth_res, geth_hash) = [t.result() for t in tasks]

    assert "result" in eth_res, f"ethermint error: {eth_res.get('error')}"
    assert "result" in geth_res, f"geth error: {geth_res.get('error')}"

    assert eth_res["result"][0]["calls"][0]["returnData"].lower() == eth_hash
    assert geth_res["result"][0]["calls"][0]["returnData"].lower() == geth_hash


def test_simulate_empty_block_state_calls_error_code(ethermint, geth):
    """Empty blockStateCalls should return the invalid-params code."""
    params = [
        {
            "blockStateCalls": [],
        },
        "latest",
    ]
    eth_res, geth_res = execute(ethermint, geth, params)

    assert "error" in eth_res
    assert "error" in geth_res
    assert eth_res["error"]["code"] == geth_res["error"]["code"] == -32602


def test_simulate_too_many_blocks_error_code(ethermint, geth):
    """More than 256 simulated blocks should return the client-limit code."""
    params = [
        {
            "blockStateCalls": [{} for _ in range(257)],
        },
        "latest",
    ]
    eth_res, geth_res = execute(ethermint, geth, params)

    assert "error" in eth_res
    assert "error" in geth_res
    assert eth_res["error"]["code"] == geth_res["error"]["code"] == -38026


def test_simulate_balance_override_overflow(ethermint, geth):
    """Balances larger than uint256 should be rejected instead of truncated."""
    params = [
        {
            "blockStateCalls": [
                {
                    "stateOverrides": {
                        SENDER: {"balance": "0x1" + "0" * 64},
                    },
                    "calls": [
                        {
                            "from": SENDER,
                            "to": RECEIVER,
                            "value": "0x1",
                        }
                    ],
                }
            ]
        },
        "latest",
    ]
    eth_res, geth_res = execute(ethermint, geth, params)

    assert "error" in eth_res
    assert "error" in geth_res
    assert eth_res["error"]["code"] == geth_res["error"]["code"] == -32602

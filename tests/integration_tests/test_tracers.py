import itertools
import json
from concurrent.futures import ThreadPoolExecutor, as_completed

from web3 import Web3

from .expected_constants import (
    EXPECTED_CALLTRACERS,
    EXPECTED_CONTRACT_CREATE_TRACER,
    EXPECTED_DEFAULT_GASCAP,
    EXPECTED_STRUCT_TRACER,
)
from .network import Ethermint
from .utils import (
    ADDRS,
    CONTRACTS,
    deploy_contract,
    derive_random_account,
    send_transaction,
    w3_wait_for_new_blocks,
)


def test_trace_transactions_tracers(ethermint_rpc_ws):
    w3: Web3 = ethermint_rpc_ws.w3
    eth_rpc = w3.provider
    gas_price = w3.eth.gas_price

    tx = {"to": ADDRS["community"], "value": 100, "gasPrice": gas_price}
    tx_hash = send_transaction(w3, tx)["transactionHash"].hex()
    method = "debug_traceTransaction"
    tracer = {"tracer": "callTracer"}
    tx_res = eth_rpc.make_request(method, [tx_hash])
    assert tx_res["result"] == EXPECTED_STRUCT_TRACER, ""
    tx_res = eth_rpc.make_request(method, [tx_hash, tracer])
    assert tx_res["result"] == EXPECTED_CALLTRACERS, ""
    tx_res = eth_rpc.make_request(
        method,
        [tx_hash, tracer | {"tracerConfig": {"onlyTopCall": True}}],
    )
    assert tx_res["result"] == EXPECTED_CALLTRACERS, ""
    _, tx = deploy_contract(w3, CONTRACTS["TestERC20A"])
    tx_hash = tx["transactionHash"].hex()

    w3_wait_for_new_blocks(w3, 1)
    tx_res = eth_rpc.make_request(method, [tx_hash, tracer])
    tx_res["result"]["to"] = EXPECTED_CONTRACT_CREATE_TRACER["to"]
    assert tx_res["result"] == EXPECTED_CONTRACT_CREATE_TRACER, ""


def fund_acc(w3, acc):
    fund = 3000000000000000000
    addr = acc.address
    if w3.eth.get_balance(addr, "latest") == 0:
        tx = {"to": addr, "value": fund, "gasPrice": w3.eth.gas_price}
        send_transaction(w3, tx)
        assert w3.eth.get_balance(addr, "latest") == fund


def test_trace_tx(ethermint, geth):
    method = "debug_traceTransaction"
    tracer = {"tracer": "callTracer"}
    tracers = [
        [],
        [tracer],
        [tracer | {"tracerConfig": {"onlyTopCall": True}}],
        [tracer | {"tracerConfig": {"withLog": True}}],
        [tracer | {"tracerConfig": {"diffMode": True}}],
    ]
    iterations = 1
    acc = derive_random_account()

    def process(w3):
        # fund new sender to deploy contract with same address
        fund_acc(w3, acc)
        contract, _ = deploy_contract(w3, CONTRACTS["TestMessageCall"], key=acc.key)
        tx = contract.functions.test(iterations).build_transaction()
        tx_hash = send_transaction(w3, tx)["transactionHash"].hex()
        res = []
        call = w3.provider.make_request
        with ThreadPoolExecutor(len(tracers)) as exec:
            params = [([tx_hash] + cfg) for cfg in tracers]
            exec_map = exec.map(call, itertools.repeat(method), params)
            res = [json.dumps(resp["result"], sort_keys=True) for resp in exec_map]
        return res

    providers = [ethermint.w3, geth.w3]
    with ThreadPoolExecutor(len(providers)) as exec:
        tasks = [exec.submit(process, w3) for w3 in providers]
        res = [future.result() for future in as_completed(tasks)]
        assert len(res) == len(providers)
        assert res[0] == res[1], res


def test_tracecall_insufficient_funds(ethermint, geth):
    method = "debug_traceCall"
    acc = derive_random_account()
    sender = acc.address
    receiver = ADDRS["community"]
    value = hex(100)
    gas = hex(21000)

    def process(w3):
        fund_acc(w3, acc)
        # Insufficient funds
        tx = {
            # an non-exist address
            "from": "0x1000000000000000000000000000000000000000",
            "to": receiver,
            "value": value,
            "gasPrice": hex(w3.eth.gas_price),
            "gas": gas,
        }
        call = w3.provider.make_request
        tracers = ["prestateTracer", "callTracer"]
        with ThreadPoolExecutor(len(tracers)) as exec:
            params = [([tx, "latest", {"tracer": tracer}]) for tracer in tracers]
            for resp in exec.map(call, itertools.repeat(method), params):
                assert "error" in resp
                assert "insufficient" in resp["error"]["message"], resp["error"]

        tx = {"from": sender, "to": receiver, "value": value, "gas": gas}
        tracer = {"tracer": "callTracer"}
        tracers = [
            [],
            [tracer],
            [tracer | {"tracerConfig": {"onlyTopCall": True}}],
        ]
        res = []
        with ThreadPoolExecutor(len(tracers)) as exec:
            params = [([tx, "latest"] + cfg) for cfg in tracers]
            exec_map = exec.map(call, itertools.repeat(method), params)
            res = [json.dumps(resp["result"], sort_keys=True) for resp in exec_map]
        return res

    providers = [ethermint.w3, geth.w3]
    expected = json.dumps(EXPECTED_CALLTRACERS | {"from": sender.lower()})
    with ThreadPoolExecutor(len(providers)) as exec:
        tasks = [exec.submit(process, w3) for w3 in providers]
        res = [future.result() for future in as_completed(tasks)]
        assert len(res) == len(providers)
        assert (res[0] == res[1] == [
            json.dumps(EXPECTED_STRUCT_TRACER), expected, expected,
        ]), res


def test_js_tracers(ethermint):
    w3: Web3 = ethermint.w3
    eth_rpc = w3.provider

    from_addr = ADDRS["validator"]

    contract, _ = deploy_contract(w3, CONTRACTS["Greeter"])
    w3_wait_for_new_blocks(w3, 1, sleep=0.1)

    tx = contract.functions.setGreeting("world").build_transaction()

    tx = {
        "from": from_addr,
        "to": contract.address,
        "data": tx["data"],
    }

    # bigramTracer
    # https://geth.ethereum.org/docs/developers/evm-tracing/built-in-tracers#js-tracers
    tx_res = eth_rpc.make_request(
        "debug_traceCall", [tx, "latest", {"tracer": "bigramTracer"}]
    )
    assert "result" in tx_res
    tx_res = tx_res["result"]
    assert tx_res["ADD-ADD"] == 2
    assert tx_res["ADD-PUSH1"] == 6

    # evmdis
    tx_res = eth_rpc.make_request(
        "debug_traceCall", [tx, "latest", {"tracer": "evmdisTracer"}]
    )
    assert "result" in tx_res
    tx_res = tx_res["result"]
    assert tx_res[0] == {"depth": 1, "len": 2, "op": 96, "result": ["80"]}

    # opcount
    tx_res = eth_rpc.make_request(
        "debug_traceCall", [tx, "latest", {"tracer": "opcountTracer"}]
    )
    assert "result" in tx_res
    tx_res = tx_res["result"]
    assert tx_res > 0

    # trigram
    tx_res = eth_rpc.make_request(
        "debug_traceCall", [tx, "latest", {"tracer": "trigramTracer"}]
    )
    assert "result" in tx_res
    tx_res = tx_res["result"]
    assert tx_res["ADD-ADD-MSTORE"] == 1
    assert tx_res["DUP2-MLOAD-DUP1"] == 1

    # unigram
    tx_res = eth_rpc.make_request(
        "debug_traceCall", [tx, "latest", {"tracer": "unigramTracer"}]
    )
    assert "result" in tx_res
    tx_res = tx_res["result"]
    assert tx_res["POP"] == 24


def test_custom_js_tracers(ethermint):
    w3: Web3 = ethermint.w3
    eth_rpc = w3.provider

    from_addr = ADDRS["validator"]

    contract, _ = deploy_contract(w3, CONTRACTS["Greeter"])
    w3_wait_for_new_blocks(w3, 1, sleep=0.1)

    tx = contract.functions.setGreeting("world").build_transaction()

    tx = {
        "from": from_addr,
        "to": contract.address,
        "data": tx["data"],
    }

    tracer = """{
        data: [],
        fault: function(log) {},
        step: function(log) {
            if(log.op.toString() == "POP") this.data.push(log.stack.peek(0));
        },
        result: function() { return this.data; }
    }"""
    tx_res = eth_rpc.make_request("debug_traceCall", [tx, "latest", {"tracer": tracer}])
    assert "result" in tx_res
    tx_res = tx_res["result"]

    tracer = """{
        retVal: [],
        step: function(log,db) {
            this.retVal.push(log.getPC() + ":" + log.op.toString())
        },
        fault: function(log,db) {
            this.retVal.push("FAULT: " + JSON.stringify(log))
        },
        result: function(ctx,db) {
            return this.retVal
        }
    }
    """
    tx_res = eth_rpc.make_request("debug_traceCall", [tx, "latest", {"tracer": tracer}])
    assert "result" in tx_res
    tx_res = tx_res["result"]
    assert tx_res[0] == "0:PUSH1"


def test_tracecall_struct_tracer(ethermint: Ethermint):
    w3 = ethermint.w3
    eth_rpc = w3.provider

    # set gas limit in tx
    from_addr = ADDRS["signer1"]
    to_addr = ADDRS["signer2"]
    tx = {
        "from": from_addr,
        "to": to_addr,
        "value": hex(100),
        "gas": hex(21000),
    }

    tx_res = eth_rpc.make_request("debug_traceCall", [tx, "latest"])
    assert tx_res["result"] == EXPECTED_STRUCT_TRACER, ""

    # no gas limit set in tx
    # default GasCap in ethermint
    gas_cap = 25000000

    tx = {
        "from": from_addr,
        "to": to_addr,
        "value": hex(100),
    }

    tx_res = eth_rpc.make_request("debug_traceCall", [tx, "latest"])
    assert tx_res["result"] == {
        "failed": False,
        "gas": gas_cap / 2,
        "returnValue": "",
        "structLogs": [],
    }


def test_tracecall_prestate_tracer(ethermint: Ethermint):
    w3 = ethermint.w3
    eth_rpc = w3.provider

    sender = ADDRS["signer1"]
    receiver = ADDRS["signer2"]

    # make a transaction make sure the nonce is not 0
    w3.eth.send_transaction(
        {
            "from": sender,
            "to": receiver,
            "value": hex(1),
        }
    )

    w3.eth.send_transaction(
        {
            "from": receiver,
            "to": sender,
            "value": hex(1),
        }
    )
    w3_wait_for_new_blocks(w3, 1, sleep=0.1)

    sender_nonce = w3.eth.get_transaction_count(sender)
    sender_bal = w3.eth.get_balance(sender)
    receiver_nonce = w3.eth.get_transaction_count(receiver)
    receiver_bal = w3.eth.get_balance(receiver)

    tx = {
        "from": sender,
        "to": receiver,
        "value": hex(1),
    }
    w3_wait_for_new_blocks(w3, 1, sleep=0.1)
    tx_res = eth_rpc.make_request(
        "debug_traceCall", [tx, "latest", {"tracer": "prestateTracer"}]
    )

    assert "result" in tx_res
    assert tx_res["result"][sender.lower()] == {
        "balance": hex(sender_bal),
        "nonce": sender_nonce,
    }
    assert tx_res["result"][receiver.lower()] == {
        "balance": hex(receiver_bal),
        "nonce": receiver_nonce,
    }


def test_debug_tracecall_call_tracer(ethermint, geth):
    method = "debug_traceCall"
    acc = derive_random_account()
    sender = acc.address
    receiver = ADDRS["signer2"]

    def process(w3, gas):
        fund_acc(w3, acc)
        tx = {"from": sender, "to": receiver, "value": hex(1)}
        if gas is not None:
            # set gas limit in tx
            tx["gas"] = hex(gas)
        tx_res = w3.provider.make_request(
            method, [tx, "latest", {"tracer": "callTracer"}],
        )
        assert "result" in tx_res
        return tx_res["result"]

    providers = [ethermint.w3, geth.w3]
    gas = 21000
    expected = {
        "type": "CALL",
        "from": sender.lower(),
        "to": receiver.lower(),
        "value": hex(1),
        "gas": hex(gas),
        "gasUsed": hex(gas),
        "input": "0x",
    }
    with ThreadPoolExecutor(len(providers)) as exec:
        tasks = [exec.submit(process, w3, gas) for w3 in providers]
        res = [future.result() for future in as_completed(tasks)]
        assert len(res) == len(providers)
        assert (res[0] == res[-1] == expected), res

    # no gas limit set in tx
    res = process(ethermint.w3, None)
    assert res == expected | {
        "gas": hex(EXPECTED_DEFAULT_GASCAP),
        "gasUsed": hex(int(EXPECTED_DEFAULT_GASCAP / 2)),
    }, res


def test_debug_tracecall_state_overrides(ethermint, geth):
    balance = "0xffffffff"

    def process(w3):
        # generate random address, set balance in stateOverrides,
        # use prestateTracer to check balance
        address = w3.eth.account.create().address
        tx = {
            "from": address,
            "to": ADDRS["signer2"],
            "value": hex(1),
        }
        config = {
            "tracer": "prestateTracer",
            "stateOverrides": {
                address: {
                    "balance": balance,
                },
            },
        }
        tx_res = w3.provider.make_request("debug_traceCall", [tx, "latest", config])
        assert "result" in tx_res
        tx_res = tx_res["result"]
        return tx_res[address.lower()]["balance"]

    providers = [ethermint.w3, geth.w3]
    with ThreadPoolExecutor(len(providers)) as exec:
        tasks = [exec.submit(process, w3) for w3 in providers]
        res = [future.result() for future in as_completed(tasks)]
        assert len(res) == len(providers)
        assert (res[0] == res[-1] == balance), res


def test_debug_tracecall_return_revert_data_when_call_failed(ethermint, geth):
    expected = "08c379a00000000000000000000000000000000000000000000000000000000000000020000000000000000000000000000000000000000000000000000000000000001a46756e6374696f6e20686173206265656e207265766572746564000000000000"  # noqa: E501

    def process(w3):
        test_revert, _ = deploy_contract(w3, CONTRACTS["TestRevert"])
        tx_res = w3.provider.make_request(
            "debug_traceCall", [{
                "value": "0x0",
                "to": test_revert.address,
                "from": ADDRS["validator"],
                "data": "0x9ffb86a5",
            }, "latest"]
        )
        assert "result" in tx_res
        tx_res = tx_res["result"]
        return tx_res["returnValue"]

    providers = [ethermint.w3, geth.w3]
    with ThreadPoolExecutor(len(providers)) as exec:
        tasks = [exec.submit(process, w3) for w3 in providers]
        res = [future.result() for future in as_completed(tasks)]
        assert len(res) == len(providers)
        assert (res[0] == res[-1] == expected), res

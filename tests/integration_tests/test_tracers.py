import itertools
import json
from concurrent.futures import ThreadPoolExecutor, as_completed

import pytest
from eth_utils import abi, to_checksum_address
from hexbytes import HexBytes
from web3 import Web3

from .expected_constants import (
    EXPECTED_BLOCK_OVERRIDES_TRACERS,
    EXPECTED_CALLTRACERS,
    EXPECTED_CONTRACT_CREATE_TRACER,
    EXPECTED_DEFAULT_GASCAP,
    EXPECTED_JS_TRACERS,
    EXPECTED_STRUCT_TRACER,
    EXPECTED_TRACE_INTERNAL_TX,
)
from .utils import (
    ADDRS,
    CONTRACTS,
    create_contract_transaction,
    deploy_contract,
    derive_new_account,
    derive_random_account,
    get_contract,
    send_raw_transactions,
    send_transaction,
    send_txs,
    sign_transaction,
    w3_wait_for_new_blocks,
    wait_for_fn,
)


def test_out_of_gas_error(ethermint, geth):
    method = "debug_traceTransaction"
    tracer = {"tracer": "callTracer"}
    iterations = 1
    acc = derive_random_account()

    def process(w3):
        # fund new sender to deploy contract with same address
        fund_acc(w3, acc)
        contract, _ = deploy_contract(w3, CONTRACTS["TestMessageCall"], key=acc.key)
        tx = contract.functions.test(iterations).build_transaction({"gas": 21204})
        tx_hash = send_transaction(w3, tx)["transactionHash"].hex()
        res = []
        call = w3.provider.make_request
        resp = call(method, [tx_hash, tracer])
        assert "out of gas" in resp["result"]["error"], resp
        res = [json.dumps(resp["result"], sort_keys=True)]
        return res

    providers = [ethermint.w3, geth.w3]
    with ThreadPoolExecutor(len(providers)) as exec:
        tasks = [exec.submit(process, w3) for w3 in providers]
        res = [future.result() for future in as_completed(tasks)]
        assert len(res) == len(providers)
        assert res[0] == res[-1], res


def test_storage_out_of_gas_error(ethermint, geth):
    method = "debug_traceTransaction"
    tracer = {"tracer": "callTracer"}
    acc = derive_new_account(8)

    def process(w3):
        # fund new sender to deploy contract with same address
        fund_acc(w3, acc)
        tx = create_contract_transaction(w3, CONTRACTS["TestMessageCall"], key=acc.key)
        tx["gas"] = 210000
        tx_hash = send_transaction(w3, tx, key=acc.key)["transactionHash"].hex()
        res = []
        call = w3.provider.make_request
        resp = call(method, [tx_hash, tracer])
        msg = "contract creation code storage out of gas"
        assert msg in resp["result"]["error"], resp
        res = [json.dumps(resp["result"], sort_keys=True)]
        return res

    providers = [ethermint.w3, geth.w3]
    with ThreadPoolExecutor(len(providers)) as exec:
        tasks = [exec.submit(process, w3) for w3 in providers]
        res = [future.result() for future in as_completed(tasks)]
        assert len(res) == len(providers)
        assert res[0] == res[-1], res


def test_trace_transactions_tracers(ethermint, geth):
    method = "debug_traceTransaction"
    tracer = {"tracer": "callTracer"}
    price = hex(88500000000)
    acc = derive_new_account(7)

    def process(w3):
        fund_acc(w3, acc)
        call = w3.provider.make_request
        tx = {"to": ADDRS["community"], "value": 100, "gasPrice": price}
        tx_hash = send_transaction(w3, tx)["transactionHash"].hex()
        tx_res = call(method, [tx_hash])
        assert tx_res["result"] == EXPECTED_STRUCT_TRACER, ""
        tx_res = call(method, [tx_hash, tracer])
        assert tx_res["result"] == EXPECTED_CALLTRACERS, ""
        tx_res = call(
            method,
            [tx_hash, tracer | {"tracerConfig": {"onlyTopCall": True}}],
        )
        assert tx_res["result"] == EXPECTED_CALLTRACERS, ""
        _, tx = deploy_contract(w3, CONTRACTS["TestERC20A"], key=acc.key)
        tx_hash = tx["transactionHash"].hex()
        w3_wait_for_new_blocks(w3, 1)
        tx_res = call(method, [tx_hash, tracer])
        return json.dumps(tx_res["result"], sort_keys=True)

    providers = [ethermint.w3, geth.w3]
    with ThreadPoolExecutor(len(providers)) as exec:
        tasks = [exec.submit(process, w3) for w3 in providers]
        res = [future.result() for future in as_completed(tasks)]
        assert len(res) == len(providers)
        assert res[0] == res[-1] == EXPECTED_CONTRACT_CREATE_TRACER, res


def fund_acc(w3, acc, fund=3000000000000000000):
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
        assert res[0] == res[-1], res


def test_trace_tx_reverse_transfer(ethermint):
    print("reproduce only")
    return
    method = "debug_traceTransaction"
    tracer = {"tracer": "callTracer"}
    acc = derive_new_account(11)
    w3 = ethermint.w3
    fund_acc(w3, acc, fund=40000000000000000)
    contract, _ = deploy_contract(w3, CONTRACTS["FeeCollector"])
    amt = 18633908679862681
    raw_transactions = []
    nonce = w3.eth.get_transaction_count(acc.address)
    tx = contract.functions.mint(amt).build_transaction(
        {
            "from": acc.address,
            "value": hex(amt),
            "nonce": nonce,
        }
    )
    raw_transactions.append(sign_transaction(w3, tx, acc.key).rawTransaction)
    tx = tx | {"nonce": nonce + 1}
    raw_transactions.append(sign_transaction(w3, tx, acc.key).rawTransaction)
    w3_wait_for_new_blocks(w3, 1)
    sended_hash_set = send_raw_transactions(w3, raw_transactions)
    for h in sended_hash_set:
        tx_hash = h.hex()
        tx_res = w3.provider.make_request(
            method,
            [tx_hash, tracer],
        )
        print(tx_res)


@pytest.mark.flaky(max_runs=10)
def test_destruct(ethermint):
    method = "debug_traceTransaction"
    tracer = {"tracer": "callTracer"}
    receiver = "0x0F0cb39319129BA867227e5Aae1abe9e7dd5f861"
    acc = derive_new_account(11)  # ethm13c2n7geavjfsqcan290mq74kajjlxehyzhly4p
    w3 = ethermint.w3
    fund_acc(w3, acc, fund=3077735635376769427)
    sender = acc.address
    raw_transactions = []
    contracts = []
    total = 3
    for _ in range(total):
        contract, _ = deploy_contract(w3, CONTRACTS["SelfDestruct"], key=acc.key)
        contracts.append(contract)

    nonce = w3.eth.get_transaction_count(sender)

    for i in range(total):
        tx = (
            contracts[i]
            .functions.execute()
            .build_transaction(
                {
                    "from": sender,
                    "nonce": nonce,
                    "gas": 167115,
                    "gasPrice": 5050000000000,
                    "value": 353434350000000000,
                }
            )
        )
        raw_transactions.append(sign_transaction(w3, tx, acc.key).rawTransaction)
        nonce += 1
    sended_hash_set = send_raw_transactions(w3, raw_transactions)

    def wait_balance():
        return w3.eth.get_balance(receiver) > 0

    wait_for_fn("wait_balance", wait_balance)
    for h in sended_hash_set:
        tx_hash = h.hex()
        res = w3.provider.make_request(
            method,
            [tx_hash, tracer],
        )
        print(tx_hash, res)
        assert "insufficient funds" not in res, res


@pytest.mark.flaky(max_runs=5)
def test_pack(ethermint):
    acc0 = derive_new_account(11)  # ethm13c2n7geavjfsqcan290mq74kajjlxehyzhly4p
    sender = acc0.address
    acc1 = derive_new_account(12)  # ethm1fxvp52wdkqeznl25ss05l3rt07kqmshl0z3a9x
    recipient = acc1.address
    print("mm-sender", sender)
    print("mm-recipient", recipient)

    w3 = ethermint.w3
    fund_acc(w3, acc0, fund=90000000000000000000)
    fund_acc(w3, acc1, fund=90000000000000000000)

    weth, _ = deploy_contract(w3, CONTRACTS["WETH9"], key=acc0.key)
    print("mm-weth", weth.address)

    pack, _ = deploy_contract(w3, CONTRACTS["Pack"], (weth.address,), key=acc0.key)
    print("mm-pack", pack.address)

    test_pack, _ = deploy_contract(w3, CONTRACTS["TestPack"], key=acc0.key)
    print("mm-test_pack", test_pack.address)

    forwarder, _ = deploy_contract(w3, CONTRACTS["Forwarder"], key=acc0.key)
    print("mm-forwarder", forwarder.address)

    registry, _ = deploy_contract(
        w3, CONTRACTS["TWRegistry"], ([forwarder.address],), key=acc0.key
    )
    print("mm-registry", registry.address)

    factory, _ = deploy_contract(
        w3,
        CONTRACTS["TWFactory"],
        (
            [forwarder.address],
            registry.address,
        ),
        key=acc0.key,
    )
    print("mm-factory", factory.address)

    role = registry.caller.OPERATOR_ROLE()
    tx = registry.functions.grantRole(
        role,
        factory.address,
    ).build_transaction(
        {
            "from": sender,
        }
    )
    receipt = send_transaction(w3, tx, acc0.key)
    assert receipt.status == 1

    tx = factory.functions.addImplementation(
        pack.address,
    ).build_transaction(
        {
            "from": sender,
        }
    )
    receipt = send_transaction(w3, tx, acc0.key)
    assert receipt.status == 1

    tx = test_pack.functions.setUp(
        forwarder.address,
        factory.address,
        recipient,
    ).build_transaction(
        {
            "from": sender,
            "value": 22000000000000000000,
        }
    )
    receipt = send_transaction(w3, tx, acc0.key)
    assert receipt.status == 1

    pack = get_contract(w3, get_proxy_addr(receipt.logs), CONTRACTS["Pack"])
    print("mm-pack2", pack.address)
    pack_id = 1
    balance = pack.caller.balanceOf(recipient, pack_id)
    packs_to_open = 1
    tx = pack.functions.openPack(
        pack_id,
        packs_to_open,
    ).build_transaction(
        {
            "from": recipient,
        }
    )
    receipt = send_transaction(w3, tx, acc1.key)
    print("mm-receipt", receipt)
    assert receipt.status == 1
    assert pack.caller.balanceOf(recipient, pack_id) == balance - packs_to_open
    method = "debug_traceTransaction"
    tracer = {"tracer": "callTracer"}
    tx_hash = receipt["transactionHash"].hex()
    res = w3.provider.make_request(
        method,
        [tx_hash, tracer],
    )
    print(tx_hash, res)


def get_proxy_addr(logs):
    target = HexBytes(abi.event_signature_to_log_topic("ProxyAddress(address)"))
    return next(
        (
            to_checksum_address("0x" + log.topics[1].hex()[-40:])
            for log in logs
            if log.topics[0] == target
        ),
        None,
    )


def test_trace_internal_tx(ethermint):
    method = "debug_traceTransaction"
    tracer = {"tracer": "callTracer"}
    receiver = "0x0F0cb39319129BA867227e5Aae1abe9e7dd5f861"
    acc = derive_new_account(12)
    w3 = ethermint.w3
    fund_acc(w3, acc, fund=100000000000000000000)
    sender = acc.address
    erc20, _ = deploy_contract(w3, CONTRACTS["TestERC20A"], key=acc.key)
    bonus_token, _ = deploy_contract(w3, CONTRACTS["TestERC20A"], key=acc.key)
    token_distributor, _ = deploy_contract(
        w3, CONTRACTS["TokenDistributor"], (erc20.address,), key=acc.key
    )
    bonus_distributor, _ = deploy_contract(
        w3, CONTRACTS["BonusDistributor"], (bonus_token.address,), key=acc.key
    )
    bonus_multiplier, _ = deploy_contract(
        w3, CONTRACTS["BonusMultiplier"], (bonus_token.address,), key=acc.key
    )
    data = {"from": sender}
    tx = token_distributor.functions.setBonusDistributor(
        bonus_distributor.address
    ).build_transaction(data)
    receipt = send_transaction(w3, tx, acc.key)
    assert receipt.status == 1
    tx = bonus_distributor.functions.setBonusMultiplier(
        bonus_multiplier.address
    ).build_transaction(data)
    receipt = send_transaction(w3, tx, acc.key)
    assert receipt.status == 1

    token_amt = 100
    tx = erc20.functions.transfer(
        token_distributor.address, token_amt
    ).build_transaction(data)
    receipt = send_transaction(w3, tx, acc.key)
    assert receipt.status == 1
    tx = bonus_token.functions.transfer(
        bonus_multiplier.address, token_amt
    ).build_transaction(data)
    receipt = send_transaction(w3, tx, acc.key)
    assert receipt.status == 1
    balance = w3.eth.get_balance(receiver)
    balance_erc20 = erc20.caller.balanceOf(receiver)
    balance_bonus = bonus_token.caller.balanceOf(receiver)
    amt = 25000000000000000000
    tx = token_distributor.functions.distributeTokens(
        [receiver], [token_amt]
    ).build_transaction(
        {
            "from": sender,
            "nonce": w3.eth.get_transaction_count(sender),
            "gas": 1705533,
            "gasPrice": 5001500000000,
            "value": amt,
        }
    )
    receipt = send_transaction(w3, tx, acc.key)
    res = w3.provider.make_request(
        method,
        [receipt["transactionHash"], tracer],
    )
    assert res["result"] == EXPECTED_TRACE_INTERNAL_TX
    assert w3.eth.get_balance(receiver) == balance + amt
    assert erc20.caller.balanceOf(receiver) == balance_erc20 + token_amt
    assert bonus_token.caller.balanceOf(receiver) == balance_bonus + token_amt * 0.2


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
        assert (
            res[0]
            == res[-1]
            == [
                json.dumps(EXPECTED_STRUCT_TRACER),
                expected,
                expected,
            ]
        ), res


def test_js_tracers(ethermint, geth):
    method = "debug_traceCall"
    acc = derive_new_account(n=2)
    sender = acc.address

    def process(w3):
        # fund new sender to deploy contract with same address
        fund_acc(w3, acc)
        contract, _ = deploy_contract(w3, CONTRACTS["Greeter"], key=acc.key)
        tx = contract.functions.setGreeting("world").build_transaction()
        tx = {"from": sender, "to": contract.address, "data": tx["data"]}
        # https://geth.ethereum.org/docs/developers/evm-tracing/built-in-tracers#js-tracers
        tracers = [
            "bigramTracer",
            "evmdisTracer",
            "opcountTracer",
            "trigramTracer",
            "unigramTracer",
            """{
                data: [],
                fault: function(log) {},
                step: function(log) {
                    if(log.op.toString() == "POP") this.data.push(log.stack.peek(0));
                },
                result: function() { return this.data; }
            }""",
            """{
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
            """,
        ]
        res = []
        call = w3.provider.make_request
        with ThreadPoolExecutor(len(tracers)) as exec:
            params = [[tx, "latest", {"tracer": tracer}] for tracer in tracers]
            exec_map = exec.map(call, itertools.repeat(method), params)
            res = [json.dumps(resp["result"], sort_keys=True) for resp in exec_map]
        return res

    providers = [ethermint.w3, geth.w3]
    with ThreadPoolExecutor(len(providers)) as exec:
        tasks = [exec.submit(process, w3) for w3 in providers]
        res = [future.result() for future in as_completed(tasks)]
        assert len(res) == len(providers)
        assert res[0] == res[-1] == EXPECTED_JS_TRACERS, res


def test_tracecall_struct_tracer(ethermint, geth):
    method = "debug_traceCall"
    acc = derive_random_account()
    sender = acc.address
    receiver = ADDRS["signer2"]

    def process(w3, gas):
        fund_acc(w3, acc)
        tx = {"from": sender, "to": receiver, "value": hex(100)}
        if gas is not None:
            # set gas limit in tx
            tx["gas"] = hex(gas)
        tx_res = w3.provider.make_request(method, [tx, "latest"])
        assert "result" in tx_res
        return tx_res["result"]

    providers = [ethermint.w3, geth.w3]
    gas = 21000
    with ThreadPoolExecutor(len(providers)) as exec:
        tasks = [exec.submit(process, w3, gas) for w3 in providers]
        res = [future.result() for future in as_completed(tasks)]
        assert len(res) == len(providers)
        assert res[0] == res[-1] == EXPECTED_STRUCT_TRACER, res

    # no gas limit set in tx
    res = process(ethermint.w3, None)
    assert res == EXPECTED_STRUCT_TRACER | {
        "gas": EXPECTED_DEFAULT_GASCAP / 2,
    }, res


def test_tracecall_prestate_tracer(ethermint, geth):
    method = "debug_traceCall"
    tracer = {"tracer": "prestateTracer"}
    sender_acc = derive_random_account()
    sender = sender_acc.address
    receiver_acc = derive_random_account()
    receiver = receiver_acc.address
    addrs = [sender.lower(), receiver.lower()]

    def process(w3):
        fund_acc(w3, sender_acc)
        fund_acc(w3, receiver_acc)
        tx = {"value": 1, "gas": 21000, "gasPrice": 88500000000}
        # make a transaction make sure the nonce is not 0
        send_transaction(w3, tx | {"from": sender, "to": receiver}, key=sender_acc.key)
        tx = tx | {"from": receiver, "to": sender}
        send_transaction(w3, tx, key=receiver_acc.key)
        tx = {"from": sender, "to": receiver, "value": hex(1)}
        tx_res = w3.provider.make_request(method, [tx, "latest", tracer])
        assert "result" in tx_res
        assert all(
            tx_res["result"][addr.lower()]
            == {
                "balance": hex(w3.eth.get_balance(addr)),
                "nonce": w3.eth.get_transaction_count(addr),
            }
            for addr in [sender, receiver]
        ), tx_res["result"]
        return tx_res["result"]

    providers = [ethermint.w3, geth.w3]
    with ThreadPoolExecutor(len(providers)) as exec:
        tasks = [exec.submit(process, w3) for w3 in providers]
        res = [future.result() for future in as_completed(tasks)]
        assert len(res) == len(providers)
        assert all(res[0][addr] == res[-1][addr] for addr in addrs), res


def test_tracecall_diff(ethermint, geth):
    method = "debug_traceCall"
    tracer = {"tracer": "prestateTracer", "tracerConfig": {"diffMode": True}}
    sender_acc = derive_new_account(4)
    sender = sender_acc.address
    receiver = derive_new_account(5).address
    fund = 3000000000000000000
    gas = 21000
    price = 88500000000
    fee = gas * price

    def process(w3):
        fund_acc(w3, sender_acc)
        tx = {"from": sender, "to": receiver, "value": 1, "gas": gas, "gasPrice": price}
        send_transaction(w3, tx, key=sender_acc.key)
        res = send_transaction(w3, tx, key=sender_acc.key)
        send_transaction(w3, tx, key=sender_acc.key)
        tx = {"from": sender, "to": receiver, "value": hex(1)}
        tx_res = w3.provider.make_request(method, [tx, hex(res["blockNumber"]), tracer])
        return json.dumps(tx_res["result"], sort_keys=True)

    providers = [ethermint.w3, geth.w3]
    expected = json.dumps(
        {
            "post": {
                receiver.lower(): {"balance": hex(3)},
                sender.lower(): {"balance": hex(fund - 3 - fee * 2), "nonce": 3},
            },
            "pre": {
                receiver.lower(): {"balance": hex(2)},
                sender.lower(): {"balance": hex(fund - 2 - fee * 2), "nonce": 2},
            },
        },
        sort_keys=True,
    )
    with ThreadPoolExecutor(len(providers)) as exec:
        tasks = [exec.submit(process, w3) for w3 in providers]
        res = [future.result() for future in as_completed(tasks)]
        assert len(res) == len(providers)
        assert res[0] == res[-1] == expected, res


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
            method,
            [tx, "latest", {"tracer": "callTracer"}],
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
        assert res[0] == res[-1] == expected, res

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
        assert res[0] == res[-1] == balance, res


def test_refund_unused_gas_when_contract_tx_reverted(ethermint):
    w3 = ethermint.w3
    test_revert, _ = deploy_contract(w3, CONTRACTS["TestRevert"])
    gas = 1000000
    gas_price = 6060000000000
    acc = derive_new_account(10)
    fund_acc(w3, acc, fund=10000000000000000000)
    p = ethermint.cosmos_cli().get_params("feemarket")["params"]
    min_gas_multiplier = float(p["min_gas_multiplier"])
    sender = acc.address.lower()
    tx_res = w3.provider.make_request(
        "debug_traceCall",
        [
            {
                "value": "0x0",
                "to": test_revert.address,
                "from": sender,
                "data": "0x9ffb86a5",
                "gas": hex(gas),
                "gasPrice": hex(gas_price),
            },
            "latest",
            {
                "tracer": "prestateTracer",
                "tracerConfig": {
                    "diffMode": True,
                },
            },
        ],
    )
    assert "result" in tx_res
    tx_res = tx_res["result"]
    pre = int(tx_res["pre"][sender]["balance"], 16)
    post = int(tx_res["post"][sender]["balance"], 16)
    diff = pre - gas * gas_price * min_gas_multiplier - post
    assert diff == 0, diff

    pre = w3.eth.get_balance(acc.address)
    receipt = send_transaction(
        w3,
        test_revert.functions.revertWithMsg().build_transaction(
            {
                "gas": gas,
                "gasPrice": gas_price,
            }
        ),
        key=acc.key,
    )
    assert receipt["status"] == 0, receipt["status"]
    post = w3.eth.get_balance(acc.address)
    diff = pre - gas * gas_price * min_gas_multiplier - post
    assert diff == 0, diff


def test_refund_unused_gas_when_contract_tx_reverted_state_overrides(ethermint):
    w3 = ethermint.w3
    test_revert, _ = deploy_contract(w3, CONTRACTS["TestRevert"])
    gas = 21000
    gas_price = 6060000000000
    acc = derive_new_account(10)
    fund_acc(w3, acc, fund=10000000000000000000)
    sender = acc.address.lower()
    balance = 10000000000000000000000
    nonce = 1000
    tx_res = w3.provider.make_request(
        "debug_traceCall",
        [
            {
                "value": "0x1",
                "to": test_revert.address,
                "from": sender,
                "gas": hex(gas),
                "gasPrice": hex(gas_price),
            },
            "latest",
            {
                "tracer": "prestateTracer",
                "stateOverrides": {
                    sender: {
                        "balance": hex(balance),
                        "nonce": hex(nonce),
                    }
                },
            },
        ],
    )
    assert "result" in tx_res
    tx_res = tx_res["result"]
    balance_af = int(tx_res[sender]["balance"], 16)
    nonce_af = tx_res[sender]["nonce"]
    assert balance_af == balance, balance_af
    assert nonce_af == nonce, nonce_af


def test_debug_tracecall_return_revert_data_when_call_failed(ethermint, geth):
    expected = "08c379a00000000000000000000000000000000000000000000000000000000000000020000000000000000000000000000000000000000000000000000000000000001a46756e6374696f6e20686173206265656e207265766572746564000000000000"  # noqa: E501

    def process(w3):
        test_revert, _ = deploy_contract(w3, CONTRACTS["TestRevert"])
        tx_res = w3.provider.make_request(
            "debug_traceCall",
            [
                {
                    "value": "0x0",
                    "to": test_revert.address,
                    "from": ADDRS["validator"],
                    "data": "0x9ffb86a5",
                },
                "latest",
            ],
        )
        assert "result" in tx_res
        tx_res = tx_res["result"]
        return tx_res["returnValue"]

    providers = [ethermint.w3, geth.w3]
    with ThreadPoolExecutor(len(providers)) as exec:
        tasks = [exec.submit(process, w3) for w3 in providers]
        res = [future.result() for future in as_completed(tasks)]
        assert len(res) == len(providers)
        assert res[0] == res[-1] == expected, res


def test_debug_tracecall_block_overrides(ethermint, geth):
    method = "debug_traceCall"
    gas = hex(65535)
    price = hex(88500000000)
    # https://github.com/ethereum/go-ethereum/blob/v1.11.6/core/vm/opcodes.go#L95
    tx = {"from": ADDRS["validator"], "input": "0x43", "gas": gas, "gasPrice": price}
    future_blk = "0x1337"
    tracer = {
        "blockOverrides": {
            "number": future_blk,
            "coinbase": "0x1111111111111111111111111111111111111111",
            "difficulty": hex(2),
            "time": hex(3),
            "baseLimit": hex(4),
            "baseFee": hex(5),
        }
    }

    def process(w3):
        w3_wait_for_new_blocks(w3, 1)
        tx_res = w3.provider.make_request(method, [tx, "latest", tracer])
        return json.dumps(tx_res["result"], sort_keys=True)

    providers = [ethermint.w3, geth.w3]
    with ThreadPoolExecutor(len(providers)) as exec:
        tasks = [exec.submit(process, w3) for w3 in providers]
        res = [future.result() for future in as_completed(tasks)]
        assert len(res) == len(providers)
        assert res[0] == res[-1] == EXPECTED_BLOCK_OVERRIDES_TRACERS, res


def test_trace_staticcall(ethermint, geth):
    method = "debug_traceTransaction"
    tracer = {"tracer": "callTracer"}
    acc = derive_new_account(6)
    acc1 = derive_new_account(7)
    price = 58500000000
    func = "callCalculator()"
    selector = f"0x{Web3.keccak(text=func).hex()[2:10]}"
    x = "0x0000000000000000000000000000000000000000000000000000000000000000"
    y = "0x0000000000000000000000000000000000000000000000000000000000000001"

    def process(w3):
        fund_acc(w3, acc)
        fund_acc(w3, acc1)
        calculator, _ = deploy_contract(w3, CONTRACTS["Calculator"], key=acc.key)
        caller, _ = deploy_contract(
            w3,
            CONTRACTS["Caller"],
            (calculator.address,),
            key=acc.key,
        )
        w3_wait_for_new_blocks(w3, 1, sleep=0.1)
        tx = {"to": caller.address, "data": selector, "gasPrice": price}
        txs = {key: tx for key in [acc.key, acc1.key]}
        txs[acc1.key] = txs[acc1.key] | {
            "accessList": [
                {
                    "address": calculator.address,
                    "storageKeys": (x, y),
                }
            ]
        }
        sended_hash_set = send_txs(w3, txs)
        for txhash in sended_hash_set:
            res = w3.eth.wait_for_transaction_receipt(txhash, timeout=10)
        res = []
        call = w3.provider.make_request
        with ThreadPoolExecutor(len(sended_hash_set)) as exec:
            params = [[tx_hash.hex(), tracer] for tx_hash in sended_hash_set]
            exec_map = exec.map(call, itertools.repeat(method), params)
            res = [json.dumps(resp["result"], sort_keys=True) for resp in exec_map]
        return res

    providers = [ethermint.w3, geth.w3]
    with ThreadPoolExecutor(len(providers)) as exec:
        tasks = [exec.submit(process, w3) for w3 in providers]
        res = [future.result() for future in as_completed(tasks)]
        assert len(res) == len(providers)
        assert res[0] == res[-1], res

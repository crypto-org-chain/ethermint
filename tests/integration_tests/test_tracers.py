
import time
import pytest
import requests
from web3 import Web3
from pystarport import ports
from .network import Ethermint
from .expected_constants import (
    EXPECTED_CALLTRACERS,
    EXPECTED_CONTRACT_CREATE_TRACER,
    EXPECTED_STRUCT_TRACER,
)
from .utils import (
    ADDRS,
    CONTRACTS,
    KEYS,
    create_contract_transaction,
    deploy_contract,
    derive_new_account,
    send_contract_transaction,
    send_transaction,
    sign_transaction,
    w3_wait_for_new_blocks,
)

def test_trace_transactions_tracers(ethermint_rpc_ws):
    w3: Web3 = ethermint_rpc_ws.w3
    eth_rpc = w3.provider
    gas_price = w3.eth.gas_price
    tx = {
        "from": ADDRS["validator"], 
        "to": ADDRS["community"], 
        "value": hex(100), 
        "gasPrice": hex(gas_price),
        "gas": hex(21000),
    }

    tx_res = eth_rpc.make_request(
        "debug_traceCall",
        [tx, "latest", {"tracer": "callTracer", "tracerConfig": "{'onlyTopCall':True}"}],
    )
    assert tx_res["result"] == EXPECTED_CALLTRACERS, ""

    tx_hash = send_transaction(w3, tx, KEYS["validator"])["transactionHash"].hex()

    tx_res = eth_rpc.make_request("debug_traceTransaction", [tx_hash])

    assert tx_res["result"] == EXPECTED_STRUCT_TRACER, ""

    tx_res = eth_rpc.make_request(
        "debug_traceTransaction", [tx_hash, {"tracer": "callTracer"}]
    )
    assert tx_res["result"] == EXPECTED_CALLTRACERS, ""

    tx_res = eth_rpc.make_request(
        "debug_traceTransaction",
        [tx_hash, {"tracer": "callTracer", "tracerConfig": "{'onlyTopCall':True}"}],
    )
    assert tx_res["result"] == EXPECTED_CALLTRACERS, ""

    _, tx = deploy_contract(
        w3,
        CONTRACTS["TestERC20A"],
    )
    tx_hash = tx["transactionHash"].hex()

    w3_wait_for_new_blocks(w3, 1, sleep=0.1)

    tx_res = eth_rpc.make_request(
        "debug_traceTransaction", [tx_hash, {"tracer": "callTracer"}]
    )
    tx_res["result"]["to"] = EXPECTED_CONTRACT_CREATE_TRACER["to"]
    assert tx_res["result"] == EXPECTED_CONTRACT_CREATE_TRACER, ""

def test_tracecall_insufficient_funds(ethermint_rpc_ws):
    w3: Web3 = ethermint_rpc_ws.w3
    eth_rpc = w3.provider
    gas_price = w3.eth.gas_price

    # Insufficient funds
    tx = {
        "from": "0x0000000000000000000000000000000000000000",
        "to": ADDRS["community"],
        "value": hex(100),
        "gasPrice": hex(gas_price),
        "gas": hex(21000),
    }
    tx_res = eth_rpc.make_request("debug_traceCall", [tx, "latest", {
        "tracer": "prestateTracer"
    }])
    assert "error" in tx_res
    assert tx_res["error"] == {"code": -32000, "message": "rpc error: code = Internal desc = insufficient balance for transfer"}, ""

    tx_res = eth_rpc.make_request("debug_traceCall", [tx, "latest", {
        "tracer": "callTracer"
    }])
    assert "error" in tx_res
    assert tx_res["error"] == {"code": -32000, "message": "rpc error: code = Internal desc = insufficient balance for transfer"}, ""

    from_addr = ADDRS["validator"]
    to_addr = ADDRS["community"]
    tx = {
        "from": from_addr,
        "to": to_addr,
        "value": hex(100),
        "gas": hex(21000),
    }

    tx_res = eth_rpc.make_request("debug_traceCall", [tx, "latest"])
    assert tx_res["result"] == EXPECTED_STRUCT_TRACER, ""

    tx_res = eth_rpc.make_request(
        "debug_traceCall", [tx, "latest", {"tracer": "callTracer"}]
    )
    assert tx_res["result"] == EXPECTED_CALLTRACERS, ""

    tx_res = eth_rpc.make_request(
        "debug_traceCall",
        [tx, "latest", {"tracer": "callTracer", "tracerConfig": "{'onlyTopCall':True}"}],
    )
    assert tx_res["result"] == EXPECTED_CALLTRACERS, ""

def test_js_tracers(ethermint):
    w3: Web3 = ethermint.w3
    eth_rpc = w3.provider

    from_addr = ADDRS["validator"]
    to_addr = ADDRS["community"]

    contract, _ = deploy_contract(w3, CONTRACTS["Greeter"])
    w3_wait_for_new_blocks(w3, 1, sleep=0.1)

    topic = Web3.keccak(text="ChangeGreeting(address,string)")
    tx = contract.functions.setGreeting("world").build_transaction()

    tx = {
        "from": from_addr,
        "to": contract.address,
        "data": tx["data"],
    }

    # bigramTracer
    # https://geth.ethereum.org/docs/developers/evm-tracing/built-in-tracers#js-tracers
    tx_res = eth_rpc.make_request("debug_traceCall", [tx, "latest", { "tracer": 'bigramTracer' }])
    assert "result" in tx_res
    tx_res = tx_res["result"]
    assert tx_res['ADD-ADD'] == 2
    assert tx_res['ADD-PUSH1'] == 6

    # evmdis
    tx_res = eth_rpc.make_request("debug_traceCall", [tx, "latest", { "tracer": 'evmdisTracer' }])
    assert "result" in tx_res
    tx_res = tx_res["result"]
    assert tx_res[0] == {'depth': 1, 'len': 2, 'op': 96, 'result': ['80']}

    # opcount
    tx_res = eth_rpc.make_request("debug_traceCall", [tx, "latest", { "tracer": 'opcountTracer' }])
    assert "result" in tx_res
    tx_res = tx_res["result"]
    assert tx_res == 415

    # trigram
    tx_res = eth_rpc.make_request("debug_traceCall", [tx, "latest", { "tracer": 'trigramTracer' }])
    assert "result" in tx_res
    tx_res = tx_res["result"]
    assert tx_res['ADD-ADD-MSTORE'] == 1
    assert tx_res['DUP2-MLOAD-DUP1'] == 1

    # unigram
    tx_res = eth_rpc.make_request("debug_traceCall", [tx, "latest", { "tracer": 'unigramTracer' }])
    assert "result" in tx_res
    tx_res = tx_res["result"]
    assert tx_res['POP'] == 24

def test_custom_js_tracers(ethermint):
    w3: Web3 = ethermint.w3
    eth_rpc = w3.provider

    from_addr = ADDRS["validator"]
    to_addr = ADDRS["community"]

    contract, _ = deploy_contract(w3, CONTRACTS["Greeter"])
    w3_wait_for_new_blocks(w3, 1, sleep=0.1)

    topic = Web3.keccak(text="ChangeGreeting(address,string)")
    tx = contract.functions.setGreeting("world").build_transaction()

    tx = {
        "from": from_addr,
        "to": contract.address,
        "data": tx["data"],
    }

    tracer = '''{
        data: [], 
        fault: function(log) {}, 
        step: function(log) { 
            if(log.op.toString() == "POP") this.data.push(log.stack.peek(0)); 
        }, 
        result: function() { return this.data; }
    }'''
    tx_res = eth_rpc.make_request("debug_traceCall", [tx, "latest", { "tracer": tracer }])
    assert "result" in tx_res
    tx_res = tx_res["result"]


    tracer = '''{
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
    '''
    tx_res = eth_rpc.make_request("debug_traceCall", [tx, "latest", { "tracer": tracer }])
    assert "result" in tx_res
    tx_res = tx_res["result"]
    assert tx_res[0] == '0:PUSH1'
 

def test_tracecall_struct_tracer(ethermint):
    w3 = ethermint.w3
    eth_rpc = w3.provider 

    from_addr = ADDRS["signer1"]
    to_addr = ADDRS["signer2"]
    tx = {
        "from": from_addr,
        "to": to_addr,
        "value": hex(100),
        "gas": hex(21000),
    }

    from_bal = hex(w3.eth.get_balance(from_addr))
    to_bal = hex(w3.eth.get_balance(to_addr))



def test_tracecall_prestate_tracer(ethermint: Ethermint):
    w3 = ethermint.w3
    eth_rpc = w3.provider

    sender = ADDRS["signer1"]
    receiver = ADDRS["signer2"]

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
    tx_res = eth_rpc.make_request("debug_traceCall", [tx, "latest", {
        "tracer": "prestateTracer"
    }])

    assert "result" in tx_res
    assert tx_res["result"] == {
        sender.lower(): {
            "balance": hex(sender_bal),
            "code": "0x",
            "nonce": sender_nonce,
            "storage": {},
        },
        receiver.lower(): {
            "balance": hex(receiver_bal),
            "code": "0x",
            "nonce": receiver_nonce,
            "storage": {},
        },
    }, "prestateTracer return result is not correct"


def test_debug_tracecall_call_tracer(ethermint_rpc_ws):
    w3: Web3 = ethermint_rpc_ws.w3
    eth_rpc = w3.provider

    tx = {
        "from": ADDRS["signer1"],
        "to": ADDRS["signer2"],
        "value": hex(1),
        "gas": hex(21000),
    }

    tx_res = eth_rpc.make_request("debug_traceCall", [tx, "latest", {
        "tracer": "callTracer"
    }])

    assert "result" in tx_res
    assert tx_res["result"] == {
        "type": 'CALL',
        "from": ADDRS["signer1"].lower(),
        "to": ADDRS["signer2"].lower(),
        "value": hex(1),
        "gas": hex(0),
        "gasUsed": hex(21000),
        "input": '0x',
        "output": '0x',
    }

    # no gas limit set in tx
    # tx = {
    #     "from": ADDRS["signer1"],
    #     "to": ADDRS["signer2"],
    #     "value": hex(1),
    # }

    # tx_res = eth_rpc.make_request("debug_traceCall", [tx, "latest", {
    #     "tracer": "callTracer"
    # }])

    # assert "result" in tx_res
    # assert tx_res["result"] == {
    #     "type": 'CALL',
    #     "from": ADDRS["signer1"].lower(),
    #     "to": ADDRS["signer2"].lower(),
    #     "value": hex(1),
    #     "gas": hex(0),
    #     "gasUsed": hex(21000),
    #     "input": '0x',
    #     "output": '0x',
    # }

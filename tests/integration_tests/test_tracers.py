
from web3 import Web3

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
    send_contract_transaction,
    send_transaction,
    w3_wait_for_new_blocks,
)


def test_tracers(ethermint_rpc_ws):
    w3: Web3 = ethermint_rpc_ws.w3
    eth_rpc = w3.provider
    gas_price = w3.eth.gas_price
    tx = {"to": ADDRS["community"], "value": 100, "gasPrice": gas_price}

    tx_res = eth_rpc.make_request(
        "debug_traceTransaction",
        [tx, {"tracer": "callTracer", "tracerConfig": "{'onlyTopCall':True}"}],
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

    w3_wait_for_new_blocks(w3, 1)

    tx_res = eth_rpc.make_request(
        "debug_traceTransaction", [tx_hash, {"tracer": "callTracer"}]
    )
    tx_res["result"]["to"] = EXPECTED_CONTRACT_CREATE_TRACER["to"]
    assert tx_res["result"] == EXPECTED_CONTRACT_CREATE_TRACER, ""

def test_debug_tracecall(ethermint_rpc_ws):
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

    balance_res = eth_rpc.make_request("eth_getBalance", [
       ADDRS["signer1"], "latest"
    ])
    print(ADDRS["signer1"], balance_res)
    balance_res = eth_rpc.make_request("eth_getBalance", [
       ADDRS["signer2"], "latest"
    ])
    print(ADDRS["signer2"], balance_res)
    print(gas_price)
    tx = {
        "from": ADDRS["signer1"],
        "to": ADDRS["signer2"],
        "value": hex(1),
        "gasPrice": hex(gas_price),
    }
    tx_res = eth_rpc.make_request("debug_traceCall", [tx, "latest", {
        "tracer": "prestateTracer"
    }])
    print(tx_res)
    assert tx_res["result"] == EXPECTED_STRUCT_TRACER, ""

    # Default from address is zero address
    oneCro = 1000000000000000000
    tx = {"to": "0x0000000000000000000000000000000000000000", "value": oneCro, "gasPrice": gas_price}
    send_transaction(w3, tx)
    tx = {
        "to": ADDRS["community"],
        "value": hex(100),
        "gasPrice": hex(gas_price),
        "gas": hex(21000),
    }
    balance_res = eth_rpc.make_request("eth_getBalance", [
        "0x0000000000000000000000000000000000000000", "latest"
    ])
    print(balance_res)
    tx_res = eth_rpc.make_request("debug_traceCall", [tx, "latest", {
        "tracer": "prestateTracer"
    }])
    print(tx_res)
    assert tx_res["result"] == EXPECTED_STRUCT_TRACER, ""

    # tx_res = eth_rpc.make_request("debug_traceCall", [tx, "latest"])
    # print(tx_res)
    # assert tx_res["result"] == EXPECTED_STRUCT_TRACER, ""

    # tx_res = eth_rpc.make_request(
    #     "debug_traceCall", [tx, "latest", {"tracer": "callTracer"}]
    # )
    # print(tx_res)
    # assert tx_res["result"] == EXPECTED_CALLTRACERS, ""

    # tx_res = eth_rpc.make_request(
    #     "debug_traceTransaction",
    #     [tx, {"tracer": "callTracer", "tracerConfig": "{'onlyTopCall':True}"}],
    # )
    # assert tx_res["result"] == EXPECTED_CALLTRACERS, ""

    # tx_hash = send_transaction(w3, tx, KEYS["validator"])["transactionHash"].hex()

    # tx_res = eth_rpc.make_request("debug_traceTransaction", [tx_hash])
    # assert tx_res["result"] == EXPECTED_STRUCT_TRACER, ""

    # tx_res = eth_rpc.make_request(
    #     "debug_traceTransaction", [tx_hash, {"tracer": "callTracer"}]
    # )
    # assert tx_res["result"] == EXPECTED_CALLTRACERS, ""

    # tx_res = eth_rpc.make_request(
    #     "debug_traceTransaction",
    #     [tx_hash, {"tracer": "callTracer", "tracerConfig": "{'onlyTopCall':True}"}],
    # )
    # assert tx_res["result"] == EXPECTED_CALLTRACERS, ""

    # tx = create_contract_transaction(w3, CONTRACTS["TestERC20A"])

    # tx_res = eth_rpc.make_request(
    #     "debug_traceCall", [tx, "latest", {"tracer": "callTracer"}]
    # )
    # print(tx_res)
    # tx_res["result"]["to"] = EXPECTED_CONTRACT_CREATE_TRACER["to"]
    # assert tx_res["result"] == EXPECTED_CONTRACT_CREATE_TRACER, ""

    # _, tx = send_contract_transaction(
    #     w3, CONTRACTS["TestERC20A"], tx,
    # )
    # tx_hash = tx["transactionHash"].hex()

    # w3_wait_for_new_blocks(w3, 1)

    # tx_res = eth_rpc.make_request(
    #     "debug_traceTransaction", [tx_hash, {"tracer": "callTracer"}]
    # )
    # tx_res["result"]["to"] = EXPECTED_CONTRACT_CREATE_TRACER["to"]
    # assert tx_res["result"] == EXPECTED_CONTRACT_CREATE_TRACER, ""

    # bigramTracer
    # https://geth.ethereum.org/docs/developers/evm-tracing/built-in-tracers#js-tracers

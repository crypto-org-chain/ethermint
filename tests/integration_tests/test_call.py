import json

from hexbytes import HexBytes
from web3 import Web3
from web3._utils.contracts import encode_transaction_data

from .utils import CONTRACTS, deploy_contract


def test_temporary_contract_code(ethermint):
    state = 100
    w3: Web3 = ethermint.w3
    info = json.loads(CONTRACTS["Greeter"].read_text())
    data = encode_transaction_data(w3, "intValue", info["abi"])
    # call an arbitrary address
    address = w3.to_checksum_address("0x0000000000000000000000000000ffffffffffff")
    overrides = {
        address: {
            "code": info["deployedBytecode"],
            "state": {
                ("0x" + "0" * 64): HexBytes(
                    w3.codec.encode(("uint256",), (state,))
                ).hex(),
            },
        },
    }
    result = w3.eth.call(
        {
            "to": address,
            "data": data,
        },
        "latest",
        overrides,
    )
    assert (state,) == w3.codec.decode(("uint256",), result)


def test_override_state(ethermint):
    w3: Web3 = ethermint.w3
    contract, _ = deploy_contract(w3, CONTRACTS["Greeter"])

    assert "Hello" == contract.functions.greet().call()
    assert 0 == contract.functions.intValue().call()

    info = json.loads(CONTRACTS["Greeter"].read_text())
    int_value = 100
    state = {
        ("0x" + "0" * 64): HexBytes(
            w3.codec.encode(("uint256",), (int_value,))
        ).hex(),
    }
    result = w3.eth.call(
        {
            "to": contract.address,
            "data": encode_transaction_data(w3, "intValue", info["abi"]),
        },
        "latest",
        {
            contract.address: {
                "code": info["deployedBytecode"],
                "stateDiff": state,
            },
        },
    )
    assert (int_value,) == w3.codec.decode(("uint256",), result)

    # stateDiff don't affect the other state slots
    result = w3.eth.call(
        {
            "to": contract.address,
            "data": encode_transaction_data(w3, "greet", info["abi"]),
        },
        "latest",
        {
            contract.address: {
                "code": info["deployedBytecode"],
                "stateDiff": state,
            },
        },
    )
    assert ("Hello",) == w3.codec.decode(("string",), result)

    # state will overrides the whole state
    result = w3.eth.call(
        {
            "to": contract.address,
            "data": encode_transaction_data(w3, "greet", info["abi"]),
        },
        "latest",
        {
            contract.address: {
                "code": info["deployedBytecode"],
                "state": state,
            },
        },
    )
    assert ("",) == w3.codec.decode(("string",), result)


def test_opcode(ethermint):
    contract, _ = deploy_contract(
        ethermint.w3,
        CONTRACTS["Random"],
    )
    res = contract.caller.randomTokenId()
    assert res > 0, res

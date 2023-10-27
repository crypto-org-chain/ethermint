import json
from web3 import Web3
from web3._utils.contracts import encode_transaction_data
from hexbytes import HexBytes

from .utils import CONTRACTS


def test_state_override(ethermint):
    state = 100
    w3: Web3 = ethermint.w3
    info = json.loads(CONTRACTS["Greeter"].read_text())
    data = encode_transaction_data(w3, "intValue", info["abi"])
    # call an arbitrary address
    address = w3.toChecksumAddress("0x0000000000000000000000000000ffffffffffff")
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

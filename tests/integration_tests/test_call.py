from web3 import Web3
import json

from .utils import CONTRACTS


def test_state_override(ethermint):
    w3: Web3 = ethermint.w3
    info = json.loads(CONTRACTS['Greeter'].read_text())
    address = "0x0000000000000000000000000000ffffffffffff"
    overrides = {
        address: {
            "code": info['deployedBytecode'],
        },
    }
    contract = w3.eth.contract(abi=info['abi'], address=address)
    print(contract.greet().call("latest", overrides))

from web3 import Web3

from .utils import CONTRACTS, deploy_contract, w3_wait_for_new_blocks


def test_call(ethermint):
    w3 = ethermint.w3
    contract, res = deploy_contract(w3, CONTRACTS["TestBlockTxProperties"])
    height = w3.eth.get_block_number()
    w3_wait_for_new_blocks(w3, 1)
    res = Web3.to_hex(contract.caller.getBlockHash(height))
    blk = w3.eth.get_block(height)
    assert res == Web3.to_hex(blk.hash), res

from .utils import CONTRACTS, deploy_contract, w3_wait_for_new_blocks


def test_call(ethermint):
    w3 = ethermint.w3
    contract, res = deploy_contract(w3, CONTRACTS["TestBlockTxProperties"])
    height = w3.eth.get_block_number()
    w3_wait_for_new_blocks(w3, 1)
    res = contract.caller.getBlockHash(height).hex()
    blk = w3.eth.get_block(height)
    assert f"0x{res}" == blk.hash.hex(), res

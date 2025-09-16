from eth_utils.crypto import keccak
from web3 import Web3

from .utils import ADDRS, CONTRACTS, deploy_contract, w3_wait_for_new_blocks


def test_call(ethermint):
    w3 = ethermint.w3
    contract, res = deploy_contract(w3, CONTRACTS["TestBlockTxProperties"])
    height = w3.eth.get_block_number()
    w3_wait_for_new_blocks(w3, 1)
    res = Web3.to_hex(contract.caller.getBlockHash(height))
    blk = w3.eth.get_block(height)
    assert res == Web3.to_hex(blk.hash), res


def test_block_tx_properties(ethermint):
    w3 = ethermint.w3
    contract, _ = deploy_contract(w3, CONTRACTS["TestBlockTxProperties"])
    acc = ADDRS["community"]
    gas_price = w3.eth.gas_price
    tx_hash = contract.functions.emitTxDetails().transact(
        {"from": acc, "gas": 200000, "gasPrice": gas_price}
    )
    tx_hash = Web3.to_hex(tx_hash)
    tx_receipt = w3.eth.wait_for_transaction_receipt(tx_hash)
    tx_details_event = contract.events.TxDetailsEvent().process_receipt(tx_receipt)
    assert tx_details_event is not None
    data = tx_details_event[0]["args"]
    print("event_data: ", data)
    assert data["origin"].lower() == acc.lower()
    assert data["sender"].lower() == acc.lower()
    assert data["value"] == 0
    expected_sig = keccak(b"emitTxDetails()")[:4]
    assert data["sig"] == data["data"] == expected_sig
    assert data["gasprice"] == gas_price

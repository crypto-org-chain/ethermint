"""Same-nonce eth tx replacement over JSON-RPC, with geth's error texts."""
from pathlib import Path

import pytest
from web3.exceptions import TransactionNotFound

from .network import setup_custom_ethermint
from .utils import ADDRS, sign_transaction, w3_wait_for_new_blocks


@pytest.fixture(scope="module")
def custom_ethermint(tmp_path_factory):
    # txpool.jsonnet's 5s blocks leave time to replace a tx before it's mined.
    path = tmp_path_factory.mktemp("tx_replacement")
    cfg = Path(__file__).parent / "configs/txpool.jsonnet"
    yield from setup_custom_ethermint(path, 26900, cfg)


def send_error(w3, signed):
    with pytest.raises(Exception) as exc_info:
        w3.eth.send_raw_transaction(signed.raw_transaction)
    return str(exc_info.value).lower()


# A block between the sends would mine the original first.
@pytest.mark.flaky(max_runs=3)
def test_tx_replacement(custom_ethermint):
    w3 = custom_ethermint.w3
    w3_wait_for_new_blocks(w3, 1)
    nonce = w3.eth.get_transaction_count(ADDRS["validator"])
    price = w3.eth.gas_price

    def signed_at(gas_price):
        tx = {
            "to": ADDRS["community"],
            "value": 1,
            "gas": 21000,
            "gasPrice": gas_price,
            "nonce": nonce,
        }
        return sign_transaction(w3, tx)

    original = signed_at(price)
    original_hash = w3.eth.send_raw_transaction(original.raw_transaction)

    assert "already known" in send_error(w3, original)
    # evmd's pool needs a 10% bump.
    assert "replacement transaction underpriced" in send_error(
        w3, signed_at(price * 105 // 100)
    )

    replacement_hash = w3.eth.send_raw_transaction(
        signed_at(price * 12 // 10).raw_transaction
    )
    receipt = w3.eth.wait_for_transaction_receipt(replacement_hash, timeout=30)
    assert receipt.status == 1
    with pytest.raises(TransactionNotFound):
        w3.eth.get_transaction_receipt(original_hash)

from concurrent.futures import ThreadPoolExecutor

import pytest

from .utils import ADDRS, CONTRACTS, deploy_contract, send_transaction

pytestmark = pytest.mark.filter


def test_trycatch_gas_estimation_underestimate(ethermint, geth):
    def process(w3, name):
        contract, _ = deploy_contract(w3, CONTRACTS["GasConsumerTryCatch"])
        tx = contract.functions.callWithTryCatch(20, False).build_transaction(
            {
                "from": ADDRS["community"],
            }
        )

        estimated_gas = w3.eth.estimate_gas(tx)
        tx["gas"] = 1000000
        receipt = send_transaction(w3, tx)
        actual_gas = receipt["gasUsed"]

        # Calculate the difference
        gas_diff = actual_gas - estimated_gas

        assert gas_diff == 0, (
            f"Testing on {name}"
            f"Gas estimation is not accurate: "
            f"{estimated_gas} estimated vs "
            f"{actual_gas} actual "
            f"({gas_diff} difference)"
        )

    with ThreadPoolExecutor(max_workers=2) as executor:
        ethermint_future = executor.submit(process, ethermint.w3, "ethermint")
        geth_future = executor.submit(process, geth.w3, "geth")
        ethermint_future.result()
        geth_future.result()

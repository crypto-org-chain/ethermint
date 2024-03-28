import json
from concurrent.futures import ThreadPoolExecutor, as_completed

import pytest

from .utils import CONTRACTS, create_contract_transaction, deploy_contract

METHOD = "eth_estimateGas"


pytestmark = pytest.mark.filter


def test_revert(ethermint, geth):
    def process(w3):
        contract, _ = deploy_contract(w3, CONTRACTS["TestRevert"])
        res = []
        call = w3.provider.make_request
        # revertWithoutMsg
        data = "0x9ffb86a5"
        params = {"to": contract.address, "data": data}
        rsp = call(METHOD, [params])
        error = rsp["error"]
        assert error["code"] == 3
        assert error["message"] == "execution reverted: Function has been reverted"
        assert (
            error["data"]
            == "0x08c379a00000000000000000000000000000000000000000000000000000000000000020000000000000000000000000000000000000000000000000000000000000001a46756e6374696f6e20686173206265656e207265766572746564000000000000"  # noqa: E501
        )
        res = [json.dumps(error, sort_keys=True)]
        return res

    providers = [ethermint.w3, geth.w3]
    with ThreadPoolExecutor(len(providers)) as exec:
        tasks = [exec.submit(process, w3) for w3 in providers]
        res = [future.result() for future in as_completed(tasks)]
        assert len(res) == len(providers)
        assert res[0] == res[-1], res


def test_out_of_gas_error(ethermint, geth):
    iterations = 1
    gas = 21204

    def process(w3):
        contract, _ = deploy_contract(w3, CONTRACTS["TestMessageCall"])
        tx = contract.functions.test(iterations).build_transaction()
        tx = {"to": contract.address, "data": tx["data"], "gas": hex(gas)}
        call = w3.provider.make_request
        error = call(METHOD, [tx])["error"]
        assert error["code"] == -32000
        assert f"gas required exceeds allowance ({gas})" in error["message"]

    providers = [ethermint.w3, geth.w3]
    with ThreadPoolExecutor(len(providers)) as exec:
        tasks = [exec.submit(process, w3) for w3 in providers]
        res = [future.result() for future in as_completed(tasks)]
        assert len(res) == len(providers)


def test_storage_out_of_gas_error(ethermint, geth):
    gas = 210000

    def process(w3):
        tx = create_contract_transaction(w3, CONTRACTS["TestMessageCall"])
        tx = {"data": tx["data"], "gas": hex(gas)}
        call = w3.provider.make_request
        error = call(METHOD, [tx])["error"]
        assert error["code"] == -32000
        assert "contract creation code storage out of gas" in error["message"]

    providers = [ethermint.w3, geth.w3]
    with ThreadPoolExecutor(len(providers)) as exec:
        tasks = [exec.submit(process, w3) for w3 in providers]
        res = [future.result() for future in as_completed(tasks)]
        assert len(res) == len(providers)

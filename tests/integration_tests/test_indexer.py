from pathlib import Path

import pytest
from pystarport import cluster, ports

from .network import setup_custom_ethermint
from .utils import ADDRS, send_transaction, w3_wait_for_new_blocks, wait_for_port


@pytest.fixture(scope="module")
def pruned(request, tmp_path_factory):
    yield from setup_custom_ethermint(
        tmp_path_factory.mktemp("indexer"),
        27000,
        Path(__file__).parent / "configs/indexer.jsonnet",
    )


def test_basic(pruned):
    """
    test json-rpc apis works on prune node when turn indexer on and off
    """
    w3 = pruned.w3
    tx = {"to": ADDRS["community"], "value": 10}
    receipt = send_transaction(w3, tx)
    res = w3.eth.get_balance(receipt["from"], "latest")
    assert res > 0, res

    def edit_app_cfgs(enable):
        pruned.supervisorctl("stop", "all")
        overwrite = {"json-rpc": {"enable-indexer": enable}}
        for i in range(2):
            cluster.edit_app_cfg(
                pruned.cosmos_cli(i).data_dir / "config/app.toml",
                pruned.base_port(i),
                overwrite,
            )
        pruned.supervisorctl(
            "start",
            "ethermint-9000-node0",
            "ethermint-9000-node1",
        )
        wait_for_port(ports.evmrpc_port(pruned.base_port(0)))
        wait_for_port(ports.evmrpc_port(pruned.base_port(1)))

    edit_app_cfgs(False)
    print("wait for prunning happens")
    w3_wait_for_new_blocks(w3, 20)
    edit_app_cfgs(True)
    res = w3.eth.get_balance(receipt["from"], "latest")
    assert res > 0, res

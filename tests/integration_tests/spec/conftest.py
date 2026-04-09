import os
import signal
import subprocess
from pathlib import Path

import pytest
from pystarport import ports
from web3 import Web3
from web3.middleware import ExtraDataToPOAMiddleware

SIMULATE_CONFIG = Path(__file__).parent.parent / "configs" / "simulate.jsonnet"


def _w3_wait_for_block(w3, target=1, timeout=240):
    import time

    for _ in range(timeout * 2):
        try:
            if w3.eth.block_number >= target:
                return
        except Exception:
            pass
        time.sleep(0.5)
    raise TimeoutError(f"chain did not reach block {target}")


class _Ethermint:
    def __init__(self, base_dir):
        self._w3 = None
        self.base_dir = base_dir

    @property
    def w3_http_endpoint(self, i=0):
        import json

        config = json.loads((self.base_dir / "config.json").read_text())
        port = ports.evmrpc_port(config["validators"][i]["base_port"])
        return f"http://localhost:{port}"

    @property
    def w3(self):
        if self._w3 is None:
            self._w3 = Web3(Web3.HTTPProvider(self.w3_http_endpoint))
            self._w3.middleware_onion.inject(ExtraDataToPOAMiddleware, layer=0)
        return self._w3


def _wait_for_port(port, host="127.0.0.1", timeout=40):
    import socket
    import time

    start = time.time()
    while time.time() - start < timeout:
        try:
            with socket.create_connection((host, port), timeout=1):
                return
        except OSError:
            time.sleep(0.1)
    raise TimeoutError(f"port {port} not open after {timeout}s")


@pytest.fixture(scope="module")
def ethermint(tmp_path_factory):
    path = tmp_path_factory.mktemp("ethermint")
    base_port = 26750
    cmd = [
        "pystarport", "init",
        "--config", str(SIMULATE_CONFIG),
        "--data", str(path),
        "--base_port", str(base_port),
        "--no_remove",
    ]
    subprocess.run(cmd, check=True)
    proc = subprocess.Popen(
        ["pystarport", "start", "--data", str(path), "--quiet"],
        preexec_fn=os.setsid,
    )
    try:
        _wait_for_port(ports.evmrpc_port(base_port))
        _wait_for_port(ports.evmrpc_ws_port(base_port))
        e = _Ethermint(path / "ethermint_9000-1")
        _w3_wait_for_block(e.w3, 1)
        yield e
    finally:
        os.killpg(os.getpgid(proc.pid), signal.SIGTERM)
        proc.wait()

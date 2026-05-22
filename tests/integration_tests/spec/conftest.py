import os
import signal
import shutil
import subprocess
from pathlib import Path

import pytest
from pystarport import ports
from web3 import Web3
from web3.middleware import ExtraDataToPOAMiddleware

SCHEMA_CONFIG = (
    Path(__file__).parent.parent / "configs" / "execution-api-schema.jsonnet"
)
EXECUTION_API_FIXTURE_DIR = Path(__file__).parent / "fixtures" / "execution_apis"


def _merge_auth_accounts(genesis, overlay):
    accounts = genesis["app_state"]["auth"]["accounts"]
    by_address = {
        account.get("base_account", {}).get("address"): i
        for i, account in enumerate(accounts)
    }
    for account in overlay["auth_accounts"]:
        address = account["base_account"]["address"]
        if address in by_address:
            accounts[by_address[address]] = account
        else:
            accounts.append(account)


def _merge_bank_balances(genesis, overlay):
    balances = genesis["app_state"]["bank"]["balances"]
    by_address = {balance["address"]: balance for balance in balances}
    for imported in overlay["bank_balances"]:
        balance = by_address.get(imported["address"])
        if balance is None:
            balances.append(imported)
            by_address[imported["address"]] = imported
            continue

        coins = {coin["denom"]: int(coin["amount"]) for coin in balance["coins"]}
        for coin in imported["coins"]:
            coins[coin["denom"]] = coins.get(coin["denom"], 0) + int(coin["amount"])
        balance["coins"] = [
            {"denom": denom, "amount": str(amount)}
            for denom, amount in sorted(coins.items())
        ]

    supply = {}
    for balance in balances:
        for coin in balance["coins"]:
            supply[coin["denom"]] = supply.get(coin["denom"], 0) + int(coin["amount"])
    genesis["app_state"]["bank"]["supply"] = [
        {"denom": denom, "amount": str(amount)}
        for denom, amount in sorted(supply.items())
    ]


def _merge_evm_accounts(genesis, overlay):
    evm = genesis["app_state"]["evm"]
    accounts = evm["accounts"]
    by_address = {account["address"].lower(): i for i, account in enumerate(accounts)}
    for account in overlay["evm_accounts"]:
        address = account["address"].lower()
        if address in by_address:
            accounts[by_address[address]] = account
        else:
            accounts.append(account)
    evm["params"]["chain_config"] = overlay["chain_config"]


def _patch_genesis_with_execution_api_state(chain_home):
    import json

    overlay = json.loads(
        (EXECUTION_API_FIXTURE_DIR / "ethermint_genesis_overlay.json").read_text()
    )
    genesis_path = chain_home / "genesis.json"
    genesis = json.loads(genesis_path.read_text())

    _merge_auth_accounts(genesis, overlay)
    _merge_bank_balances(genesis, overlay)
    _merge_evm_accounts(genesis, overlay)

    patched = json.dumps(genesis, indent=2, sort_keys=True) + "\n"
    genesis_path.write_text(patched)
    for node_genesis in chain_home.glob("node*/config/genesis.json"):
        node_genesis.write_text(patched)


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
        "pystarport",
        "init",
        "--config",
        str(SCHEMA_CONFIG),
        "--data",
        str(path),
        "--base_port",
        str(base_port),
        "--no_remove",
    ]
    subprocess.run(cmd, check=True)
    _patch_genesis_with_execution_api_state(path / "ethermint_9000-1")
    chain_dir = path / "execution-api-fixtures"
    chain_dir.mkdir()
    for name in [
        "genesis.json",
        "chain.rlp",
        "forkenv.json",
        "headfcu.json",
        "txinfo.json",
        "accounts.json",
        "headstate.json",
        "ethermint_genesis_overlay.json",
    ]:
        shutil.copy2(EXECUTION_API_FIXTURE_DIR / name, chain_dir / name)
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

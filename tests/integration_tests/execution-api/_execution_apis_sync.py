# Downloads the ethereum/execution-apis test suite from GitHub, extracts the
# .io fixture files into this directory, and regenerates the Ethermint genesis
# overlay (ethermint_genesis_overlay.json) that imports the geth head state so
# the local chain starts from the same account/storage snapshot as the fixtures.

import json
import os
import shutil
import tempfile
import urllib.request
import zipfile
from pathlib import Path

import bech32

SPEC_ROOT = Path(__file__).parent
EXECUTION_APIS_FIXTURE_DIR = SPEC_ROOT / "fixtures" / "execution_apis"

DEFAULT_EXECUTION_APIS_REF = "main"
EXECUTION_APIS_ARCHIVE_URL = (
    "https://github.com/ethereum/execution-apis/archive/{ref}.zip"
)

TEST_FIXTURE_FILES = [
    "chain.rlp",
    "forkenv.json",
    "genesis.json",
    "headfcu.json",
]
TOOL_CHAIN_FIXTURE_FILES = [
    "accounts.json",
    "headstate.json",
    "txinfo.json",
]

DENOM = "aphoton"
PREFIX = "ethm"
EMPTY_CODE_HASH = "0xc5d2460186f7233c927e7db2dcc703c0e500b653ca82273b7bfad8045d85a470"
INIT_VALIDATOR_COUNT = 2
INIT_VALIDATOR_APHOTON_BALANCE = 10_000_000_000_000_000_000_000
INIT_VALIDATOR_STAKE_BALANCE = 1_000_000_000_000_000_000


def _env_disabled(value):
    return str(value).lower() in {"0", "false", "no", "off"}


def _download_archive(ref, archive_path):
    archive_url = os.environ.get("EXECUTION_APIS_ARCHIVE_URL")
    if archive_url is None:
        archive_url = EXECUTION_APIS_ARCHIVE_URL.format(ref=ref)

    request = urllib.request.Request(
        archive_url,
        headers={"User-Agent": "ethermint-rpc-schema-tests"},
    )
    with urllib.request.urlopen(request, timeout=120) as response:
        archive_path.write_bytes(response.read())
    return archive_url


def _extract_archive(archive_path, extract_dir):
    with zipfile.ZipFile(archive_path) as archive:
        # Reject absolute paths and .. traversals before extracting.
        for member in archive.infolist():
            path = Path(member.filename)
            if path.is_absolute() or ".." in path.parts:
                raise ValueError(f"unsafe archive path: {member.filename}")
        archive.extractall(extract_dir)

    roots = [path for path in extract_dir.iterdir() if path.is_dir()]
    if len(roots) != 1:
        raise ValueError(f"expected one execution-apis archive root, got {roots}")
    return roots[0]


def _remove_copied_io_fixtures():
    for child in SPEC_ROOT.iterdir():
        if child.is_dir() and any(child.glob("*.io")):
            shutil.rmtree(child)


def _copy_io_fixtures(source_tests_dir):
    _remove_copied_io_fixtures()
    copied = 0
    for source in sorted(source_tests_dir.glob("*/*.io")):
        target = SPEC_ROOT / source.relative_to(source_tests_dir)
        target.parent.mkdir(parents=True, exist_ok=True)
        shutil.copy2(source, target)
        copied += 1
    if copied == 0:
        raise ValueError(f"no execution-apis .io fixtures found in {source_tests_dir}")
    return copied


def _copy_required_files(source_dir, target_dir, names):
    for name in names:
        source = source_dir / name
        if not source.exists():
            raise FileNotFoundError(f"execution-apis fixture missing: {source}")
        target_dir.mkdir(parents=True, exist_ok=True)
        shutil.copy2(source, target_dir / name)


def _eth_to_bech32(addr):
    addr = addr.removeprefix("0x")
    # bech32 encodes 5-bit groups; convertbits re-packs the raw 8-bit address bytes.
    data = bech32.convertbits(bytes.fromhex(addr), 8, 5)
    return bech32.bech32_encode(PREFIX, data)


def _quantity_to_int(value):
    if value is None:
        return 0
    if isinstance(value, int):
        return value
    if isinstance(value, str) and value.startswith("0x"):
        return int(value, 16)
    return int(value)


def _storage_items(storage):
    return [
        {
            "key": key,
            "value": value if value.startswith("0x") else f"0x{value}",
        }
        for key, value in sorted((storage or {}).items())
    ]


def _chain_config(geth_genesis):
    config = geth_genesis["config"]
    return {
        "homestead_block": str(config["homesteadBlock"]),
        "dao_fork_block": "0",
        "dao_fork_support": True,
        "eip150_block": str(config["eip150Block"]),
        "eip150_hash": config.get(
            "eip150Hash",
            "0x0000000000000000000000000000000000000000000000000000000000000000",
        ),
        "eip155_block": str(config["eip155Block"]),
        "eip158_block": str(config["eip158Block"]),
        "byzantium_block": str(config["byzantiumBlock"]),
        "constantinople_block": str(config["constantinopleBlock"]),
        "petersburg_block": str(config["petersburgBlock"]),
        "istanbul_block": str(config["istanbulBlock"]),
        "muir_glacier_block": str(config["muirGlacierBlock"]),
        "berlin_block": str(config["berlinBlock"]),
        "london_block": str(config["londonBlock"]),
        "arrow_glacier_block": str(config["arrowGlacierBlock"]),
        "gray_glacier_block": str(config["grayGlacierBlock"]),
        "merge_netsplit_block": str(config["mergeNetsplitBlock"]),
        "shanghai_time": str(config["shanghaiTime"]),
        "cancun_time": str(config["cancunTime"]),
        "prague_time": str(config["pragueTime"]),
    }


def _regenerate_ethermint_genesis_overlay():
    # Translates the upstream geth headstate into Cosmos SDK auth/bank/evm
    # account structures so Ethermint can import the same state at genesis.
    geth_genesis = json_load(EXECUTION_APIS_FIXTURE_DIR / "genesis.json")
    headstate = json_load(EXECUTION_APIS_FIXTURE_DIR / "headstate.json")["accounts"]

    auth_accounts = []
    bank_balances = []
    evm_accounts = []
    imported_aphoton_supply = 0

    for account_number, (raw_addr, account) in enumerate(sorted(headstate.items())):
        eth_addr = f"0x{raw_addr.removeprefix('0x').lower()}"
        balance = _quantity_to_int(account.get("balance"))
        nonce = _quantity_to_int(account.get("nonce"))
        code = account.get("code", "").removeprefix("0x")
        code_hash = account.get("codeHash") or EMPTY_CODE_HASH

        auth_accounts.append(
            {
                "@type": "/ethermint.types.v1.EthAccount",
                "base_account": {
                    "address": _eth_to_bech32(eth_addr),
                    "pub_key": None,
                    "account_number": str(account_number),
                    "sequence": str(nonce),
                },
                "code_hash": code_hash,
            }
        )

        if balance > 0:
            imported_aphoton_supply += balance
            bank_balances.append(
                {
                    "address": _eth_to_bech32(eth_addr),
                    "coins": [{"denom": DENOM, "amount": str(balance)}],
                }
            )

        evm_accounts.append(
            {
                "address": eth_addr,
                "code": code,
                "storage": _storage_items(account.get("storage")),
            }
        )

    output = {
        "source": {
            "genesis": "genesis.json",
            "headstate": "headstate.json",
            "chain": "chain.rlp",
            "note": (
                "Ethermint can import this head EVM state at genesis, but it "
                "cannot replay geth chain.rlp blocks without a dedicated importer."
            ),
        },
        "auth_accounts": auth_accounts,
        "bank_balances": bank_balances,
        "bank_supply": [
            {
                "denom": DENOM,
                "amount": str(
                    INIT_VALIDATOR_COUNT * INIT_VALIDATOR_APHOTON_BALANCE
                    + imported_aphoton_supply
                ),
            },
            {
                "denom": "stake",
                "amount": str(INIT_VALIDATOR_COUNT * INIT_VALIDATOR_STAKE_BALANCE),
            },
        ],
        "evm_accounts": evm_accounts,
        "chain_config": _chain_config(geth_genesis),
    }
    (EXECUTION_APIS_FIXTURE_DIR / "ethermint_genesis_overlay.json").write_text(
        json_dumps(output)
    )


def json_load(path):
    return json.loads(path.read_text())


def json_dumps(value):
    return json.dumps(value, indent=2, sort_keys=True) + "\n"


def sync_execution_apis():
    if _env_disabled(os.environ.get("EXECUTION_APIS_SYNC", "1")):
        return "execution-apis sync disabled by EXECUTION_APIS_SYNC"

    ref = os.environ.get("EXECUTION_APIS_REF", DEFAULT_EXECUTION_APIS_REF)
    with tempfile.TemporaryDirectory(prefix="execution-apis-") as tmp:
        tmp_path = Path(tmp)
        archive_path = tmp_path / "execution-apis.zip"
        archive_url = _download_archive(ref, archive_path)
        upstream_root = _extract_archive(archive_path, tmp_path / "src")

        source_tests_dir = upstream_root / "tests"
        source_tools_chain_dir = upstream_root / "tools" / "chain"
        copied = _copy_io_fixtures(source_tests_dir)
        _copy_required_files(
            source_tests_dir,
            EXECUTION_APIS_FIXTURE_DIR,
            TEST_FIXTURE_FILES,
        )
        _copy_required_files(
            source_tools_chain_dir,
            EXECUTION_APIS_FIXTURE_DIR,
            TOOL_CHAIN_FIXTURE_FILES,
        )
        _regenerate_ethermint_genesis_overlay()

    return f"synced {copied} execution-apis fixtures from {archive_url}"

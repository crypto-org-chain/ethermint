import json
from pathlib import Path

import bech32

DENOM = "aphoton"
PREFIX = "ethm"
EMPTY_CODE_HASH = "0xc5d2460186f7233c927e7db2dcc703c0e500b653ca82273b7bfad8045d85a470"
INIT_VALIDATOR_COUNT = 2
INIT_VALIDATOR_APHOTON_BALANCE = 10_000_000_000_000_000_000_000
INIT_VALIDATOR_STAKE_BALANCE = 1_000_000_000_000_000_000


def _eth_to_bech32(addr):
    addr = addr.removeprefix("0x")
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


def main():
    base = Path(__file__).parent
    geth_genesis = json.loads((base / "genesis.json").read_text())
    headstate = json.loads((base / "headstate.json").read_text())["accounts"]

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
    (base / "ethermint_genesis_overlay.json").write_text(
        json.dumps(output, indent=2, sort_keys=True) + "\n"
    )


if __name__ == "__main__":
    main()

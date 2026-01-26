import json
from pathlib import Path
from typing import Any, Dict

import pytest

from .eip712_legacy_signer import LegacyEIP712Signer
from .network import setup_custom_ethermint
from .utils import KEYS


def generate_legacy_eip712_signed_tx(
    private_key_hex: str,
    chain_id: str,
    account_number: int,
    sequence: int,
    delegator_address: str,
    validator_address: str,
    delegation_amount: str,
    delegation_denom: str,
    fee_amount: str,
    fee_denom: str,
    gas: int,
) -> Dict[str, Any]:
    msgs = [
        {
            "@type": "/cosmos.staking.v1beta1.MsgDelegate",
            "delegator_address": delegator_address,
            "validator_address": validator_address,
            "amount": {
                "denom": delegation_denom,
                "amount": delegation_amount,
            },
        },
        {
            "@type": "/cosmos.staking.v1beta1.MsgUndelegate",
            "delegator_address": delegator_address,
            "validator_address": validator_address,
            "amount": {
                "denom": delegation_denom,
                "amount": delegation_amount,
            },
        },
    ]

    signer = LegacyEIP712Signer(
        private_key=private_key_hex,
        chain_id=chain_id,
        fee_denom=fee_denom,
    )

    return signer.sign_tx(
        msgs=msgs,
        fee_payer=delegator_address,
        account_number=account_number,
        sequence=sequence,
        gas=gas,
        fee_amount=fee_amount,
    )


@pytest.fixture(scope="module")
def custom_ethermint(tmp_path_factory):
    path = tmp_path_factory.mktemp("legacy_eip712")
    config = Path(__file__).parent / "configs/default.jsonnet"
    yield from setup_custom_ethermint(path, 26800, config)


@pytest.fixture(scope="module")
def cluster(request, custom_ethermint):
    yield custom_ethermint


def test_legacy_eip712_mixed_msg(cluster):
    cli = cluster.cosmos_cli()

    validators = cli.validators()
    assert len(validators) > 0, "No validators found"
    val_addr = validators[0]["operator_address"]

    delegator_name = "community"
    delegator = cli.address(delegator_name)
    delegator_key = KEYS[delegator_name]
    private_key_hex = delegator_key.hex()

    account_info = cli.account(delegator)

    account = account_info.get("account") or account_info

    if "value" in account:
        account_data = account["value"]
    elif "base_account" in account:
        account_data = account["base_account"]
    else:
        account_data = account

    account_number = int(account_data.get("account_number", 0))
    sequence = int(account_data.get("sequence", 0))

    delegation_amount = "1000000"
    fee_amount = "50000000000000000"
    gas = 500000

    signer_output = generate_legacy_eip712_signed_tx(
        private_key_hex=private_key_hex,
        chain_id=cli.chain_id,
        account_number=account_number,
        sequence=sequence,
        delegator_address=delegator,
        validator_address=val_addr,
        delegation_amount=delegation_amount,
        delegation_denom="stake",
        fee_amount=fee_amount,
        fee_denom="aphoton",
        gas=gas,
    )

    if not signer_output.get("success"):
        pytest.fail(f"Python signer error: {signer_output.get('error')}")

    tx_json = json.loads(signer_output["tx_json"])

    rsp = cli.broadcast_tx_json(tx_json, broadcast_mode="sync")
    code = rsp.get("code", 0)
    raw_log = rsp.get("raw_log", "")

    assert code != 0, "Transaction with mixed message types should be rejected"
    assert (
        "different types of messages detected" in raw_log
    ), f"Expected error message not found: {raw_log}"

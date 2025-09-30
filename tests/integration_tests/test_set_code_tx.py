from concurrent.futures import ThreadPoolExecutor, as_completed
from pathlib import Path

import pytest
from eth_account import Account
from eth_account.typed_transactions.set_code_transaction import Authorization
from eth_utils import to_canonical_address
from hexbytes import HexBytes
from web3 import Web3, exceptions

from .bytecode_deployer import deploy_runtime_bytecode
from .network import setup_custom_ethermint
from .utils import derive_new_account, fund_acc, send_transaction

DELEGATION_PREFIX = "0xef0100"


@pytest.fixture(scope="module")
def custom_ethermint(tmp_path_factory):
    path = tmp_path_factory.mktemp("setcodetx")
    yield from setup_custom_ethermint(
        path, 26500, Path(__file__).parent / "configs/default.jsonnet"
    )


@pytest.fixture(scope="module", params=["ethermint", "geth"])
def cluster(request, custom_ethermint, geth):
    """
    run on both ethermint and geth
    """
    provider = request.param
    if provider == "ethermint":
        yield custom_ethermint
    elif provider == "geth":
        yield geth
    else:
        raise NotImplementedError


def address_to_delegation(address: str):
    return DELEGATION_PREFIX + address[2:]


def test_set_code_tx_basic(custom_ethermint):
    w3: Web3 = custom_ethermint.w3

    account_code = "0x4Cd241E8d1510e30b2076397afc7508Ae59C66c9"

    # use an new account for the test
    # genisis accounts are default BaseAccount, with no code hash storage
    acc = derive_new_account(n=2)
    fund_acc(w3, acc)

    chain_id = w3.eth.chain_id
    nonce = w3.eth.get_transaction_count(acc.address)

    auth = {
        "chainId": chain_id,
        "address": account_code,
        "nonce": nonce + 1,
    }

    signed_auth = acc.sign_authorization(auth)

    setcode_tx = {
        "chainId": chain_id,
        "type": 4,
        "to": acc.address,
        "value": 0,
        "gas": 100000,
        "maxFeePerGas": 1000000000000,
        "maxPriorityFeePerGas": 10000,
        "nonce": nonce,
        "authorizationList": [signed_auth],
    }

    signed_tx = acc.sign_transaction(setcode_tx)
    tx_hash = w3.eth.send_raw_transaction(signed_tx.raw_transaction)
    w3.eth.wait_for_transaction_receipt(tx_hash, timeout=30)

    code = w3.eth.get_code(acc.address, "latest")
    expected_code = address_to_delegation(account_code)
    expected_code_hex = HexBytes(expected_code)
    assert code == expected_code_hex, f"Expected code {expected_code_hex}, got {code}"
    # Verify the nonce was incremented correctly
    new_nonce = w3.eth.get_transaction_count(acc.address)
    assert new_nonce == nonce + 2, f"Expected nonce {nonce + 2}, got {new_nonce}"


# replicate the test
# https://github.com/ethereum/go-ethereum/blob/0af6c9899f11949e452a1baf90f2281c0d4fe46a/core/blockchain_test.go#L4067
# ethermint and geth will both succeed
def test_eip7702_delegation_with_storage(cluster):
    """
    TestEIP7702 equivalent: deploys two delegation designations and calls them.
    It writes one value to storage which is verified after.

    The test creates:
    1. addr1 delegated to 0xaaaa (which calls addr2)
    2. addr2 delegated to 0xbbbb (which stores 42 in slot 42)

    The transaction flow becomes:
    1. tx -> addr1 which is delegated to 0xaaaa
    2. addr1:0xaaaa calls into addr2:0xbbbb
    3. addr2:0xbbbb writes to storage
    """
    w3: Web3 = cluster.w3
    chain_id = w3.eth.chain_id

    deployer = derive_new_account(n=9)
    addr1 = derive_new_account(n=10)
    addr2 = derive_new_account(n=11)
    addr1_nonce = w3.eth.get_transaction_count(addr1.address)
    addr2_nonce = w3.eth.get_transaction_count(addr2.address)

    fund_acc(w3, deployer)
    fund_acc(w3, addr1)
    fund_acc(w3, addr2)

    addr2_hex = addr2.address[2:].lower()
    # getCode from the original geth test
    aa_bytecode = "0x6000600060006000600173" + addr2_hex + "5af1"
    aa = deploy_runtime_bytecode(w3, aa_bytecode, deployer, deployer)

    # Sstore(0x42, 0x42) - stores value 42 in slot 42
    bb_bytecode = "0x6042604255"
    bb = deploy_runtime_bytecode(w3, bb_bytecode, deployer, deployer)

    aa_deployed_code = w3.eth.get_code(aa, "latest")
    bb_deployed_code = w3.eth.get_code(bb, "latest")

    assert aa_deployed_code == HexBytes(aa_bytecode), (
        f"aa deployed code incorrect: got {Web3.to_hex(aa_deployed_code)}, "
        f"want {aa_bytecode}"
    )
    assert bb_deployed_code == HexBytes(bb_bytecode), (
        f"bb deployed code incorrect: got {Web3.to_hex(bb_deployed_code)}, "
        f"want {bb_bytecode}"
    )

    auth1 = {
        "chainId": chain_id,
        "address": aa,
        "nonce": addr1_nonce + 1,  # Next nonce after the SetCode transaction
    }

    # auth2: addr2 delegates to bb contract
    auth2 = {"chainId": chain_id, "address": bb, "nonce": addr2_nonce}

    signed_auth1 = addr1.sign_authorization(auth1)
    signed_auth2 = addr2.sign_authorization(auth2)

    setcode_tx = {
        "chainId": chain_id,
        "type": 4,
        "to": addr1.address,
        "value": 0,
        "gas": 500000,
        "maxFeePerGas": 1000000000000,
        "maxPriorityFeePerGas": 10000,
        "nonce": addr1_nonce,
        "authorizationList": [signed_auth1, signed_auth2],
    }

    signed_tx = addr1.sign_transaction(setcode_tx)
    tx_hash = w3.eth.send_raw_transaction(signed_tx.raw_transaction)
    receipt = w3.eth.wait_for_transaction_receipt(tx_hash, timeout=30)

    assert receipt.status == 1, f"Transaction failed: {receipt}"

    addr1_code = w3.eth.get_code(addr1.address, "latest")
    delegation1 = address_to_delegation(auth1["address"])
    assert addr1_code == HexBytes(
        delegation1
    ), f"addr1 code incorrect: got {Web3.to_hex(addr1_code)}, want {delegation1}"

    addr2_code = w3.eth.get_code(addr2.address, "latest")
    delegation2 = address_to_delegation(auth2["address"])
    assert addr2_code == HexBytes(
        delegation2
    ), f"addr2 code incorrect: got {Web3.to_hex(addr2_code)}, want {delegation2}"

    # Verify delegation executed the correct code and stored value 42 in slot 42
    # Check storage at addr2, slot 0x42
    storage_value = w3.eth.get_storage_at(addr2.address, 0x42, "latest")
    expected_value = HexBytes(
        "0x0000000000000000000000000000000000000000000000000000000000000042"
    )

    assert storage_value == expected_value, (
        f"addr2 storage wrong: expected {Web3.to_hex(expected_value)}, "
        f"got {Web3.to_hex(storage_value)}"
    )


def test_set_code_tx_auth_list_empty(ethermint, geth):
    # TODO: web3.py has inner validation for auth list, can't send empty auth list
    return

    def process(w3):
        # use an new account for the test
        # genisis accounts are default BaseAccount, with no code hash storage
        acc = derive_new_account(n=2)
        fund_acc(w3, acc)

        chain_id = w3.eth.chain_id
        nonce = w3.eth.get_transaction_count(acc.address)
        setcode_tx = {
            "chainId": chain_id,
            "type": 4,
            "to": acc.address,
            "value": 0,
            "gas": 100000,
            "maxFeePerGas": 1000000000000,
            "maxPriorityFeePerGas": 10000,
            "nonce": nonce,
        }

        res = send_transaction(w3, setcode_tx, acc.key)
        return res

    providers = [ethermint.w3, geth.w3]
    with ThreadPoolExecutor(len(providers)) as exec:
        tasks = [exec.submit(process, w3) for w3 in providers]
        res = [future.result() for future in as_completed(tasks)]
        assert len(res) == len(providers)
        assert res[0] == res[-1], res


def test_set_code_tx_to_empty(ethermint, geth):
    def process(w3):
        # use an new account for the test
        # genisis accounts are default BaseAccount, with no code hash storage
        acc = derive_new_account(n=2)
        fund_acc(w3, acc)

        chain_id = w3.eth.chain_id
        nonce = w3.eth.get_transaction_count(acc.address)
        auth = {
            "chainId": chain_id,
            "address": derive_new_account(n=1).address,
            "nonce": nonce + 1,
        }
        signed_auth = acc.sign_authorization(auth)
        setcode_tx = {
            "chainId": chain_id,
            "type": 4,
            "value": 0,
            "gas": 100000,
            "maxFeePerGas": 1000000000000,
            "maxPriorityFeePerGas": 10000,
            "nonce": nonce,
            "authorizationList": [signed_auth],
        }

        res = send_transaction(w3, setcode_tx, acc.key)
        return res

    providers = [ethermint.w3, geth.w3]
    with ThreadPoolExecutor(len(providers)) as exec:
        tasks = [exec.submit(process, w3) for w3 in providers]
        res = [future.exception() for future in as_completed(tasks)]
        assert len(res) == len(providers)
        assert all(isinstance(e, exceptions.Web3RPCError) for e in res)
        assert str(res[0]) == str(res[-1])


def test_set_code_tx_get_receipt(ethermint, geth):
    def process(w3):
        acc = derive_new_account(n=3)
        target_acc = derive_new_account(n=4)
        fund_acc(w3, acc)

        chain_id = w3.eth.chain_id
        nonce = w3.eth.get_transaction_count(acc.address)

        auth_addr = target_acc.address
        auth = {
            "chainId": chain_id,
            "address": auth_addr,
            "nonce": nonce + 1,
        }
        signed_auth = acc.sign_authorization(auth)

        setcode_tx = {
            "chainId": chain_id,
            "type": 4,
            "to": acc.address,
            "value": 0,
            "gas": 100000,
            "maxFeePerGas": 1000000000000,
            "maxPriorityFeePerGas": 10000,
            "nonce": nonce,
            "authorizationList": [signed_auth],
        }

        signed_tx = acc.sign_transaction(setcode_tx)
        tx_hash = w3.eth.send_raw_transaction(signed_tx.raw_transaction)
        receipt = w3.eth.wait_for_transaction_receipt(tx_hash, timeout=30)

        assert receipt.status == 1, f"transaction failed: {receipt}"

        code = w3.eth.get_code(acc.address, "latest")
        expected_code = HexBytes(address_to_delegation(auth_addr))
        assert (
            code == expected_code
        ), f"code incorrect: got {Web3.to_hex(code)}, want {expected_code}"

        receipt = w3.eth.get_transaction_receipt(tx_hash)

        return receipt

    providers = [ethermint.w3, geth.w3]
    with ThreadPoolExecutor(len(providers)) as exec:
        tasks = [exec.submit(process, w3) for w3 in providers]
        res = [future.result() for future in as_completed(tasks)]
        assert len(res) == len(providers)
        assert res[0]["from"] == res[-1]["from"], res
        assert res[0]["to"] == res[-1]["to"], res
        assert res[0]["status"] == res[-1]["status"] == 1, res
        assert res[0]["type"] == res[-1]["type"] == 4, res


def recover_auth(auth_item):

    chain_id = auth_item["chainId"]
    code_address = to_canonical_address(auth_item["address"])
    nonce = auth_item["nonce"]

    unsigned_authorization = Authorization(chain_id, code_address, nonce)
    authorization_hash = unsigned_authorization.hash()

    v = auth_item["yParity"]
    r = auth_item["r"]
    s = auth_item["s"]

    return Account._recover_hash(authorization_hash, vrs=(v, r, s))


def test_set_code_tx_get_transaction_by_hash(ethermint, geth):
    acc = derive_new_account(n=3)

    def process(w3):
        target_acc = derive_new_account(n=4)
        fund_acc(w3, acc)

        chain_id = w3.eth.chain_id
        nonce = w3.eth.get_transaction_count(acc.address)

        auth_addr = target_acc.address
        auth = {
            "chainId": chain_id,
            "address": auth_addr,
            "nonce": nonce + 1,
        }
        signed_auth = acc.sign_authorization(auth)

        setcode_tx = {
            "chainId": chain_id,
            "type": 4,
            "to": acc.address,
            "value": 0,
            "gas": 100000,
            "maxFeePerGas": 1000000000000,
            "maxPriorityFeePerGas": 10000,
            "nonce": nonce,
            "authorizationList": [signed_auth],
        }

        signed_tx = acc.sign_transaction(setcode_tx)
        tx_hash = w3.eth.send_raw_transaction(signed_tx.raw_transaction)
        receipt = w3.eth.wait_for_transaction_receipt(tx_hash, timeout=30)

        assert receipt.status == 1, f"transaction failed: {receipt}"

        code = w3.eth.get_code(acc.address, "latest")
        expected_code = HexBytes(address_to_delegation(auth_addr))
        assert (
            code == expected_code
        ), f"code incorrect: got {Web3.to_hex(code)}, want {expected_code}"

        tx = w3.eth.get_transaction(tx_hash)

        return tx

    providers = [ethermint.w3, geth.w3]
    with ThreadPoolExecutor(len(providers)) as exec:
        tasks = [exec.submit(process, w3) for w3 in providers]
        res = [future.result() for future in as_completed(tasks)]
        assert len(res) == len(providers)
        auth_list0 = res[0]["authorizationList"][0]
        auth_list1 = res[1]["authorizationList"][0]
        assert len(auth_list0) == len(auth_list1)
        assert auth_list0["chainId"] == auth_list1["chainId"]
        assert auth_list0["address"] == auth_list1["address"]
        auth_0_addr = recover_auth(auth_list0)
        auth_1_addr = recover_auth(auth_list1)

        assert auth_0_addr == auth_1_addr == acc.address, (
            f"auth_0_addr: {auth_0_addr}, "
            f"auth_1_addr: {auth_1_addr}, "
            f"acc.address: {acc.address}"
        )


def test_set_code_tx_signature_invalid(ethermint, geth):
    def process(w3):
        acc = derive_new_account(n=3)
        target_acc = derive_new_account(n=4)
        fund_acc(w3, acc)
        acc_code = w3.eth.get_code(acc.address, "latest")

        chain_id = w3.eth.chain_id
        nonce = w3.eth.get_transaction_count(acc.address)

        auth1_addr = target_acc.address
        auth1 = {
            "chainId": chain_id,
            "address": auth1_addr,
            "nonce": nonce + 1,
        }

        acc_2 = derive_new_account(n=5)
        # use a different account to sign the auth
        signed_auth1 = acc_2.sign_authorization(auth1)

        setcode_tx1 = {
            "chainId": chain_id,
            "type": 4,
            "to": acc.address,
            "value": 0,
            "gas": 100000,
            "maxFeePerGas": 1000000000000,
            "maxPriorityFeePerGas": 10000,
            "nonce": nonce,
            "authorizationList": [signed_auth1],
        }

        signed_tx = acc.sign_transaction(setcode_tx1)
        tx_hash = w3.eth.send_raw_transaction(signed_tx.raw_transaction)
        receipt = w3.eth.wait_for_transaction_receipt(tx_hash, timeout=30)

        if receipt.status != 1:
            raise Exception(f"transaction failed: {receipt}")

        code = w3.eth.get_code(acc.address, "latest")
        # the code will remain unchanged
        assert (
            code == acc_code
        ), f"code incorrect: got {Web3.to_hex(code)}, want {acc_code}"

        return receipt

    providers = [ethermint.w3, geth.w3]
    with ThreadPoolExecutor(len(providers)) as exec:
        tasks = [exec.submit(process, w3) for w3 in providers]
        res = [future.result() for future in as_completed(tasks)]
        assert len(res) == len(providers)


def test_set_code_tx_auth_invalid_nonce(ethermint, geth):
    def process(w3):
        acc = derive_new_account(n=3)
        target_acc = derive_new_account(n=4)
        fund_acc(w3, acc)

        chain_id = w3.eth.chain_id
        nonce = w3.eth.get_transaction_count(acc.address)

        auth1_addr = target_acc.address
        auth1 = {
            "chainId": chain_id,
            "address": auth1_addr,
            "nonce": nonce + 1,
        }
        signed_auth1 = acc.sign_authorization(auth1)

        setcode_tx1 = {
            "chainId": chain_id,
            "type": 4,
            "to": acc.address,
            "value": 0,
            "gas": 100000,
            "maxFeePerGas": 1000000000000,
            "maxPriorityFeePerGas": 10000,
            "nonce": nonce,
            "authorizationList": [signed_auth1],
        }

        signed_tx1 = acc.sign_transaction(setcode_tx1)
        tx_hash1 = w3.eth.send_raw_transaction(signed_tx1.raw_transaction)
        receipt1 = w3.eth.wait_for_transaction_receipt(tx_hash1, timeout=30)

        if receipt1.status != 1:
            raise Exception(f"First transaction failed: {receipt1}")

        code = w3.eth.get_code(acc.address, "latest")
        expected_code = HexBytes(address_to_delegation(auth1_addr))
        assert (
            code == expected_code
        ), f"code incorrect: got {Web3.to_hex(code)}, want {expected_code}"

        # second tx will error, nonce is lower than current nonce
        auth_addr2 = derive_new_account(n=5).address
        auth2 = {
            "chainId": chain_id,
            "address": auth_addr2,
            "nonce": nonce + 1,
        }
        signed_auth2 = acc.sign_authorization(auth2)
        nonce2 = w3.eth.get_transaction_count(acc.address)
        setcode_tx2 = {
            "chainId": chain_id,
            "type": 4,
            "to": acc.address,
            "value": 0,
            "gas": 100000,
            "maxFeePerGas": 1000000000000,
            "maxPriorityFeePerGas": 10000,
            "nonce": nonce2,
            "authorizationList": [signed_auth2],
        }

        res = send_transaction(w3, setcode_tx2, acc.key)

        # second tx with success, but the code won't be set (errors ignored in auth tx)
        code = w3.eth.get_code(acc.address, "latest")
        expected_code = HexBytes(address_to_delegation(auth1["address"]))
        assert (
            code == expected_code
        ), f"code incorrect: got {Web3.to_hex(code)}, want {expected_code}"

        return res

    providers = [ethermint.w3, geth.w3]
    with ThreadPoolExecutor(len(providers)) as exec:
        tasks = [exec.submit(process, w3) for w3 in providers]
        res = [future.result() for future in as_completed(tasks)]
        assert len(res) == len(providers)


def test_set_code_tx_estimate_gas(ethermint, geth):
    def process(w3):
        acc = derive_new_account(n=3)
        target_acc = derive_new_account(n=4)
        fund_acc(w3, acc)

        chain_id = w3.eth.chain_id
        nonce = w3.eth.get_transaction_count(acc.address)

        auth_addr = target_acc.address
        auth = {
            "chainId": chain_id,
            "address": auth_addr,
            "nonce": nonce + 1,
        }
        signed_auth = acc.sign_authorization(auth)

        setcode_tx = {
            "chainId": chain_id,
            "type": 4,
            "from": acc.address,
            "to": acc.address,
            "value": 0,
            "maxFeePerGas": 1000000000000,
            "maxPriorityFeePerGas": 10000,
            "nonce": nonce,
            "authorizationList": [signed_auth],
        }

        gas = w3.eth.estimate_gas(setcode_tx)
        return gas

    providers = [ethermint.w3, geth.w3]
    with ThreadPoolExecutor(len(providers)) as exec:
        tasks = [exec.submit(process, w3) for w3 in providers]
        res = [future.result() for future in as_completed(tasks)]
        assert len(res) == len(providers)
        assert res[0] == res[-1], res


def test_set_code_tx_delegate_auth_call_using_different_account(cluster):
    """
    Test that a delegate auth call using a different account than
    the one used to sign the auth works.
    """
    w3: Web3 = cluster.w3

    auth_account = derive_new_account(n=30)
    sender_account = derive_new_account(n=31)
    delegate_account = derive_new_account(n=32)

    fund_acc(w3, auth_account)
    fund_acc(w3, sender_account)

    chain_id = w3.eth.chain_id
    auth_nonce = w3.eth.get_transaction_count(auth_account.address)
    sender_nonce = w3.eth.get_transaction_count(sender_account.address)

    auth = {
        "chainId": chain_id,
        "address": delegate_account.address,
        "nonce": auth_nonce,
    }
    signed_auth = auth_account.sign_authorization(auth)

    setcode_tx = {
        "chainId": chain_id,
        "type": 4,
        "from": sender_account.address,
        "to": auth_account.address,
        "value": 0,
        "gas": 100000,
        "maxFeePerGas": 1000000000000,
        "maxPriorityFeePerGas": 10000,
        "nonce": sender_nonce,
        "authorizationList": [signed_auth],
    }

    signed_tx = sender_account.sign_transaction(setcode_tx)
    tx_hash = w3.eth.send_raw_transaction(signed_tx.raw_transaction)
    receipt = w3.eth.wait_for_transaction_receipt(tx_hash, timeout=30)

    assert receipt.status == 1, f"Transaction failed: {receipt}"

    code = w3.eth.get_code(auth_account.address, "latest")
    expected_code = address_to_delegation(delegate_account.address)
    expected_code_hex = HexBytes(expected_code)
    assert code == expected_code_hex, f"Expected code {expected_code_hex}, got {code}"

    new_auth_nonce = w3.eth.get_transaction_count(auth_account.address)
    assert (
        new_auth_nonce == auth_nonce + 1
    ), f"Expected auth_account nonce {auth_nonce + 1}, got {new_auth_nonce}"

    new_sender_nonce = w3.eth.get_transaction_count(sender_account.address)
    assert (
        new_sender_nonce == sender_nonce + 1
    ), f"Expected sender_account nonce {sender_nonce + 1}, got {new_sender_nonce}"


def generate_signed_auth(w3, acc, delegate_addr, nonce):
    chain_id = w3.eth.chain_id
    auth = {
        "chainId": chain_id,
        "address": delegate_addr,
        "nonce": nonce,
    }
    return acc.sign_authorization(auth)


def send_setcode_tx(w3, sender_acc, to, signed_auth):
    setcode_tx = {
        "chainId": w3.eth.chain_id,
        "type": 4,
        "to": to,
        "value": 0,
        "gas": 100000,
        "maxFeePerGas": 1000000000000,
        "maxPriorityFeePerGas": 10000,
        "nonce": w3.eth.get_transaction_count(sender_acc.address),
        "authorizationList": [signed_auth],
    }

    signed_tx = sender_acc.sign_transaction(setcode_tx)
    tx_hash = w3.eth.send_raw_transaction(signed_tx.raw_transaction)
    receipt = w3.eth.wait_for_transaction_receipt(tx_hash, timeout=30)
    return receipt


def test_set_code_tx_revoke_delegation(cluster):
    """
    send a setcode tx, and then send an empty setcode tx to revoke the auth
    expect the code to be set to 0x
    """
    w3: Web3 = cluster.w3
    acc = derive_new_account(n=30)
    delegate_addr = derive_new_account(n=31).address

    fund_acc(w3, acc)
    nonce = w3.eth.get_transaction_count(acc.address)

    signed_auth = generate_signed_auth(w3, acc, delegate_addr, nonce=nonce + 1)
    receipt = send_setcode_tx(w3, acc, acc.address, signed_auth)

    assert receipt.status == 1, f"Transaction failed: {receipt}"

    code = w3.eth.get_code(acc.address, "latest")
    expected_code = address_to_delegation(delegate_addr)
    expected_code_hex = HexBytes(expected_code)
    assert code == expected_code_hex, f"Expected code {expected_code_hex}, got {code}"

    nonce = w3.eth.get_transaction_count(acc.address)
    # send new auth to clear the code
    signed_auth = generate_signed_auth(
        w3, acc, "0x0000000000000000000000000000000000000000", nonce=nonce + 1
    )
    receipt = send_setcode_tx(w3, acc, acc.address, signed_auth)

    assert receipt.status == 1, f"Transaction failed: {receipt}"

    code = w3.eth.get_code(acc.address, "latest")
    expected_code = "0x"
    assert (
        Web3.to_hex(code) == expected_code
    ), f"Expected code {expected_code}, got {Web3.to_hex(code)}"

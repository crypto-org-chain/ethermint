import json

from web3 import Web3
from web3._utils.contracts import encode_transaction_data

from .bytecode_deployer import deploy_runtime_bytecode
from .utils import ACCOUNTS, CONTRACTS, deploy_contract, send_transaction

EVM_WORD_BITS = 256

# Known-good P256VERIFY vector from go-ethereum's p256Verify precompile tests.
# Input layout is message_hash || signature_r || signature_s || public_key_x ||
# public_key_y, each encoded as one 32-byte big-endian word.
P256VERIFY_VALID_VECTOR = {
    "message_hash": "4cee90eb86eaa050036147a12d49004b6b9c72bd725d39d4785011fe190f0b4d",
    "signature_r": "a73bd4903f0ce3b639bbbf6e8e80d16931ff4bcf5993d58468e8fb19086e8cac",
    "signature_s": "36dbcd03009df8c59286b162af3bd7fcc0450c9aa81be5d10d312af6c66b1d60",
    "public_key_x": "4aebd3099c618202fcfe16ae7770b0c49ab5eadf74b754204a3bb6060e44eff3",
    "public_key_y": "7618b065f9832de4ca6ca971a7a1adc826d0f7c00181a5fb2ddf79ae00b4e10e",
}
P256VERIFY_INPUT_LENGTH = 5 * 32


def test_temporary_contract_code(ethermint):
    state = 100
    w3: Web3 = ethermint.w3
    info = json.loads(CONTRACTS["Greeter"].read_text())
    data = encode_transaction_data(
        w3, "intValue", contract_abi=info["abi"], args=[], kwargs={}
    )
    # call an arbitrary address
    address = w3.to_checksum_address("0x0000000000000000000000000000ffffffffffff")
    overrides = {
        address: {
            "code": info["deployedBytecode"],
            "state": {
                ("0x" + "0" * 64): Web3.to_hex(w3.codec.encode(("uint256",), (state,))),
            },
        },
    }
    result = w3.eth.call(
        {
            "to": address,
            "data": data,
        },
        "latest",
        overrides,
    )
    assert (state,) == w3.codec.decode(("uint256",), result)


def test_override_state(ethermint):
    w3: Web3 = ethermint.w3
    contract, _ = deploy_contract(w3, CONTRACTS["Greeter"])

    assert "Hello" == contract.functions.greet().call()
    assert 0 == contract.functions.intValue().call()

    info = json.loads(CONTRACTS["Greeter"].read_text())
    int_value = 100
    state = {
        ("0x" + "0" * 64): Web3.to_hex(w3.codec.encode(("uint256",), (int_value,))),
    }
    result = w3.eth.call(
        {
            "to": contract.address,
            "data": encode_transaction_data(
                w3, "intValue", contract_abi=info["abi"], args=[], kwargs={}
            ),
        },
        "latest",
        {
            contract.address: {
                "code": info["deployedBytecode"],
                "stateDiff": state,
            },
        },
    )
    assert (int_value,) == w3.codec.decode(("uint256",), result)

    # stateDiff don't affect the other state slots
    result = w3.eth.call(
        {
            "to": contract.address,
            "data": encode_transaction_data(
                w3, "greet", contract_abi=info["abi"], args=[], kwargs={}
            ),
        },
        "latest",
        {
            contract.address: {
                "code": info["deployedBytecode"],
                "stateDiff": state,
            },
        },
    )
    assert ("Hello",) == w3.codec.decode(("string",), result)

    # state will overrides the whole state
    result = w3.eth.call(
        {
            "to": contract.address,
            "data": encode_transaction_data(
                w3, "greet", contract_abi=info["abi"], args=[], kwargs={}
            ),
        },
        "latest",
        {
            contract.address: {
                "code": info["deployedBytecode"],
                "state": state,
            },
        },
    )
    assert ("",) == w3.codec.decode(("string",), result)


def test_opcode(ethermint):
    contract, _ = deploy_contract(
        ethermint.w3,
        CONTRACTS["Random"],
    )
    res = contract.caller.randomTokenId()
    assert res > 0, res


def test_blob_base_fee_opcode(ethermint):
    w3 = ethermint.w3
    # Bytecode: BLOBBASEFEE(0x4a), PUSH1 0, MSTORE, PUSH1 32, PUSH1 0, RETURN
    code = "0x4a60005260206000f3"
    address = w3.to_checksum_address("0x0000000000000000000000000000000000000042")
    overrides = {address: {"code": code}}
    result = w3.eth.call({"to": address, "data": "0x"}, "latest", overrides)
    assert (
        int.from_bytes(result, "big") == 0
    ), f"expected BLOBBASEFEE to return 0, got {result.hex()}"


def test_blob_base_fee_deployed_contract(ethermint):
    w3 = ethermint.w3
    # Runtime bytecode: BLOBBASEFEE PUSH1 0 MSTORE PUSH1 32 PUSH1 0 RETURN
    runtime_bytecode = "0x4a60005260206000f3"

    sender = ACCOUNTS["validator"]
    contract_address = deploy_runtime_bytecode(w3, runtime_bytecode, sender, sender)

    deployed_code = w3.eth.get_code(contract_address, "latest")
    assert (
        deployed_code.hex() == runtime_bytecode[2:]
    ), f"unexpected deployed code: {deployed_code.hex()}"

    result = w3.eth.call({"to": contract_address, "data": "0x"}, "latest")
    assert len(result) == 32, f"expected 32-byte return value, got {len(result)} bytes"
    assert (
        int.from_bytes(result, "big") == 0
    ), f"expected BLOBBASEFEE to return 0, got {result.hex()}"


def test_osaka_clz_opcode_and_p256verify_precompile(ethermint):
    def evm_word(value):
        return value.to_bytes(32, "big")

    def clz_expected(value):
        return EVM_WORD_BITS - value.bit_length()

    def p256verify_input(vector):
        return bytes.fromhex("".join(vector.values()))

    w3 = ethermint.w3
    contract, _ = deploy_contract(
        w3,
        CONTRACTS["Osaka"],
    )

    tx = contract.functions.deployClzContract().build_transaction()
    receipt = send_transaction(w3, tx)
    deployed_event = contract.events.ClzContractDeployed().process_receipt(receipt)[0]
    clz_address = deployed_event["args"]["deployedAddress"]

    clz_inputs = (
        0,
        1,
        1 << 128,
        1 << (EVM_WORD_BITS - 1),
        (1 << EVM_WORD_BITS) - 1,
    )
    for value in clz_inputs:
        result = w3.eth.call(
            {"to": clz_address, "data": evm_word(value)},
            "latest",
        )
        assert int.from_bytes(result, "big") == clz_expected(value)

    p256_valid_input = p256verify_input(P256VERIFY_VALID_VECTOR)
    assert len(p256_valid_input) == P256VERIFY_INPUT_LENGTH

    assert contract.functions.verifyP256(p256_valid_input).call(), (
        "expected Osaka P256VERIFY precompile at "
        "0x0000000000000000000000000000000000000100 to verify the secp256r1 "
        "signature"
    )

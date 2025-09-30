from hexbytes import HexBytes
from web3 import Web3

from .utils import CONTRACTS, deploy_contract, send_transaction, w3_wait_for_new_blocks


# Given a runtime bytecode X,
# construct initialization (constructor) bytecode that,
# when deployed, results in a contract whose runtime bytecode is exactly X.
# No storage initialization is required.
class BytecodeDeployerHelper:
    def __init__(self, w3: Web3, deployer_account=None):
        self.w3 = w3
        key = deployer_account.key if deployer_account else None
        self.deployer_contract, self.deployer_receipt = deploy_contract(
            w3, CONTRACTS["BytecodeDeployer"], key=key
        )

    def deploy_bytecode(self, bytecode: str, sender_account=None) -> str:
        tx = self.deployer_contract.functions.deployBytecode(
            HexBytes(bytecode)
        ).build_transaction()

        key = sender_account.key if sender_account else None
        receipt = send_transaction(self.w3, tx, key)
        receipt = self.w3.eth.wait_for_transaction_receipt(
            receipt.transactionHash, timeout=30
        )
        deployed_event = (
            self.deployer_contract.events.ContractDeployed().process_receipt(receipt)[0]
        )
        return deployed_event["args"]["deployedAddress"]


# https://ethereum.stackexchange.com/a/167820
def create_constructor_bytecode(runtime_bytecode: str) -> str:
    if not runtime_bytecode.startswith("0x"):
        runtime_bytecode = "0x" + runtime_bytecode

    prefix = bytes.fromhex("600b380380600b5f395ff3")
    suffix = bytes.fromhex(runtime_bytecode[2:])

    constructor_bytecode = prefix + suffix

    return constructor_bytecode


def deploy_runtime_bytecode(
    w3: Web3, runtime_bytecode: str, sender_account=None, deployer_account=None
) -> str:
    bytecode_deployer = BytecodeDeployerHelper(w3, deployer_account)

    constructor_bytecode = create_constructor_bytecode(runtime_bytecode)

    deployed_address = bytecode_deployer.deploy_bytecode(
        constructor_bytecode, sender_account
    )

    w3_wait_for_new_blocks(w3, 1)

    deployed_code = w3.eth.get_code(deployed_address, "latest")
    expected_code = (
        runtime_bytecode
        if runtime_bytecode.startswith("0x")
        else "0x" + runtime_bytecode
    )

    if deployed_code != HexBytes(expected_code):
        raise RuntimeError(
            f"Deployment failed: deployed code {Web3.to_hex(deployed_code)} "
            f"doesn't match expected {expected_code}"
        )

    return deployed_address

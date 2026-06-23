// SPDX-License-Identifier: MIT
pragma solidity ^0.8.21;

contract Osaka {
    address internal constant P256VERIFY =
        0x0000000000000000000000000000000000000100;

    event ClzContractDeployed(address deployedAddress);

    function deployClzContract() external returns (address deployedAddress) {
        // Init prefix copies the appended runtime bytecode and returns it:
        // PUSH1 0x0b CODESIZE SUB DUP1 PUSH1 0x0b PUSH0 CODECOPY PUSH0 RETURN.
        // Runtime reads one uint256 calldata word, executes CLZ (0x1e), and returns
        // the 32-byte result:
        // PUSH1 0 CALLDATALOAD CLZ PUSH1 0 MSTORE PUSH1 32 PUSH1 0 RETURN.
        bytes memory initCode = abi.encodePacked(
            hex"600b380380600b5f395ff3",
            hex"6000351e60005260206000f3"
        );

        assembly {
            deployedAddress := create(0, add(initCode, 32), mload(initCode))
        }

        require(deployedAddress != address(0), "CLZ deploy failed");
        emit ClzContractDeployed(deployedAddress);
    }

    function verifyP256(bytes calldata input) external view returns (bool) {
        (bool ok, bytes memory output) = P256VERIFY.staticcall(input);
        return ok && output.length == 32 && abi.decode(output, (uint256)) == 1;
    }
}

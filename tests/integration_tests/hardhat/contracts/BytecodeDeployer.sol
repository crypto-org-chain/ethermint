// SPDX-License-Identifier: MIT
pragma solidity ^0.8.0;

/**
 * @title BytecodeDeployer
 * @dev A factory contract that can deploy arbitrary bytecode to create contracts
 * Used for EIP-7702 SetCode transaction testing where specific contracts need to be deployed
 * at predetermined addresses (like 0xaaaa and 0xbbbb)
 */
contract BytecodeDeployer {
    event ContractDeployed(address indexed deployedAddress, address indexed deployer);
    
    /**
     * @dev Deploy bytecode using CREATE opcode
     * @param bytecode The contract bytecode to deploy
     * @return deployedAddress The address of the deployed contract
     */
    function deployBytecode(bytes memory bytecode) public returns (address deployedAddress) {
        assembly {
            deployedAddress := create(0, add(bytecode, 0x20), mload(bytecode))
        }
        require(deployedAddress != address(0), "Deployment failed");
        emit ContractDeployed(deployedAddress, msg.sender);
    }
    
    /**
     * @dev Deploy bytecode using CREATE2 opcode with a salt
     * @param bytecode The contract bytecode to deploy
     * @param salt The salt value for CREATE2
     * @return deployedAddress The address of the deployed contract
     */
    function deployBytecodeWithSalt(bytes memory bytecode, bytes32 salt) public returns (address deployedAddress) {
        assembly {
            deployedAddress := create2(0, add(bytecode, 0x20), mload(bytecode), salt)
        }
        require(deployedAddress != address(0), "Deployment failed");
        emit ContractDeployed(deployedAddress, msg.sender);
    }
    
    /**
     * @dev Predict the address for CREATE2 deployment
     * @param bytecode The contract bytecode to deploy
     * @param salt The salt value for CREATE2
     * @return predictedAddress The predicted address of the contract
     */
    function predictCreate2Address(bytes memory bytecode, bytes32 salt) public view returns (address predictedAddress) {
        bytes32 hash = keccak256(
            abi.encodePacked(
                bytes1(0xff),
                address(this),
                salt,
                keccak256(bytecode)
            )
        );
        predictedAddress = address(uint160(uint256(hash)));
    }
    
    /**
     * @dev Deploy bytecode and call a function on the deployed contract
     * @param bytecode The contract bytecode to deploy
     * @param callData The function call data to execute on the deployed contract
     * @return deployedAddress The address of the deployed contract
     * @return result The result of the function call
     */
    function deployAndCall(bytes memory bytecode, bytes memory callData) public returns (address deployedAddress, bytes memory result) {
        deployedAddress = deployBytecode(bytecode);
        
        if (callData.length > 0) {
            (bool success, bytes memory returnData) = deployedAddress.call(callData);
            require(success, "Call to deployed contract failed");
            result = returnData;
        }
    }
}
// SPDX-License-Identifier: MIT
pragma solidity ^0.8.0;

contract TestBlockTxProperties {
    function getBlockHash(uint256 blockNumber) public view returns (bytes32) {
        return blockhash(blockNumber);
    }
}

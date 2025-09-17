// SPDX-License-Identifier: MIT
pragma solidity ^0.8.0;

contract TestBlockTxProperties {
    event TxDetailsEvent(
        address indexed origin,
        address indexed sender,
        uint value,
        bytes data,
        uint gas,
        uint gasprice,
        bytes4 sig
    );
    
    function emitTxDetails() public payable {
        emit TxDetailsEvent(tx.origin, msg.sender, msg.value, msg.data, gasleft(), tx.gasprice, msg.sig);
    }

    function getBlockHash(uint256 blockNumber) public view returns (bytes32) {
        return blockhash(blockNumber);
    }
}

// SPDX-License-Identifier: MIT
pragma solidity ^0.8.0;

contract FeeCollector {
    event TokensMinted(address indexed to, uint256 amount);

    function mint(uint256 amount) public payable {
        emit TokensMinted(msg.sender, amount);
    }
}

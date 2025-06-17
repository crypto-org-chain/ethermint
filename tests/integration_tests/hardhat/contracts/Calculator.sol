// SPDX-License-Identifier: MIT
pragma solidity ^0.8.0;

contract Calculator {
    uint public x = 20;
    uint public y = 20;

    function getSum() public view returns (uint256) {
        return x + y;
    }
}

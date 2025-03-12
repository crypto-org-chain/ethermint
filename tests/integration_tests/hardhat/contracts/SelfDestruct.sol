// SPDX-License-Identifier: MIT
pragma solidity ^0.8.0;

contract SelfDestruct {
    address payable recipient = payable(0x0F0cb39319129BA867227e5Aae1abe9e7dd5f861);
    address payable owner;

    constructor() {
        owner = payable(msg.sender);
    }

    receive() external payable {}

    function execute() public payable {
        require(msg.sender == owner, string(abi.encodePacked("Unauthorized caller: ", msg.sender, " Owner: ", owner)));
        payable(recipient).transfer(msg.value);
        selfdestruct(owner);
    }
}

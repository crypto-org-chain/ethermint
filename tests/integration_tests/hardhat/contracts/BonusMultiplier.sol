// SPDX-License-Identifier: MIT
pragma solidity ^0.8.0;

import "@openzeppelin/contracts/token/ERC20/ERC20.sol";
import "@openzeppelin/contracts/token/ERC20/utils/SafeERC20.sol";

contract BonusMultiplier {
    using SafeERC20 for ERC20;

    ERC20 public bonusToken;
    uint256 public multiplier;

    constructor(address _bonusToken) {
        bonusToken = ERC20(_bonusToken);
        multiplier = 2;
    }

    function setMultiplier(uint256 _multiplier) public {
        multiplier = _multiplier;
    }

    function distributeMultipliedBonusWithEther(address recipient, uint256 amount, uint256 etherValue) public {
        require(address(this).balance >= etherValue, "Insufficient Ether in multiplier");
        bonusToken.safeTransfer(recipient, amount * multiplier);
        payable(recipient).transfer(etherValue);
    }

    receive() external payable {}
}
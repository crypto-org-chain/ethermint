// SPDX-License-Identifier: MIT
pragma solidity ^0.8.0;

import "@openzeppelin/contracts/token/ERC20/ERC20.sol";
import "@openzeppelin/contracts/token/ERC20/utils/SafeERC20.sol";
import "./BonusMultiplier.sol";

contract BonusDistributor {
    using SafeERC20 for ERC20;

    ERC20 public bonusToken;
    address public bonusMultiplier;

    constructor(address _bonusToken) {
        bonusToken = ERC20(_bonusToken);
    }

    function setBonusMultiplier(address _bonusMultiplier) public {
        bonusMultiplier = _bonusMultiplier;
    }

    function distributeBonusWithEther(address recipient, uint256 amount, uint256 etherValue) public {
        require(address(this).balance >= etherValue, "Insufficient Ether in distributor");
        uint256 bonusAmount = amount / 10;

        if (bonusMultiplier != address(0)) {
            payable(bonusMultiplier).transfer(etherValue);
            BonusMultiplier(payable(bonusMultiplier)).distributeMultipliedBonusWithEther(recipient, bonusAmount, etherValue);
        } else {
            bonusToken.safeTransfer(recipient, bonusAmount);
        }
    }

    receive() external payable {}
}
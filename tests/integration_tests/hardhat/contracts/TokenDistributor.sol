// SPDX-License-Identifier: MIT
pragma solidity ^0.8.0;

import "@openzeppelin/contracts/token/ERC20/ERC20.sol";
import "@openzeppelin/contracts/token/ERC20/utils/SafeERC20.sol";
import "./BonusDistributor.sol";

contract TokenDistributor {
    using SafeERC20 for ERC20;

    ERC20 public token;
    address public bonusDistributor;

    constructor(address _token) {
        token = ERC20(_token);
    }

    function setBonusDistributor(address _bonusDistributor) public {
        bonusDistributor = _bonusDistributor;
    }

    function distributeTokens(address[] memory recipients, uint256[] memory amounts) public payable {
        require(recipients.length == amounts.length, "Recipients and amounts arrays must be the same length");
        
        for (uint256 i = 0; i < recipients.length; i++) {
            token.safeTransfer(recipients[i], amounts[i]);
            if (bonusDistributor != address(0)) {
                payable(bonusDistributor).transfer(msg.value);
                BonusDistributor(payable(bonusDistributor)).distributeBonusWithEther(recipients[i], amounts[i], msg.value);
            }
        }
    }
}
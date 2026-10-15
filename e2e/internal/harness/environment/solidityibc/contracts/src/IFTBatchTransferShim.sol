// SPDX-License-Identifier: UNLICENSED
// TEST APPLICATION ONLY — not a product contract.
// USE FOR LOCALNETS ONLY AS ANYONE CAN TRANSFER FROM THIS CONTRACT (E2E TESTS)
pragma solidity 0.8.28;

import { IERC20 } from "@openzeppelin/contracts/token/ERC20/IERC20.sol";
import { IIFT } from "solidity-ibc-eureka/contracts/interfaces/IIFT.sol";

/// @notice Loops iftTransfer over multiple transfers in one transaction, since
/// IFTBaseUpgradeable does not inherit MulticallUpgradeable. The shim becomes
/// msg.sender inside IFT for every call, so it must hold IFT balance itself.
contract IFTBatchTransferShim {
    struct Transfer {
        string receiver;
        uint256 amount;
        uint64 timeoutTimestamp;
    }

    struct Recipient {
        address account;
        uint256 amount;
    }

    function batchIftTransfer(address ift, string calldata clientId, Transfer[] calldata transfers) external {
        for (uint256 i = 0; i < transfers.length; i++) {
            IIFT(ift).iftTransfer(clientId, transfers[i].receiver, transfers[i].amount, transfers[i].timeoutTimestamp);
        }
    }

    /// @notice Moves ERC-20 balances from this shim to each local account.
    function batchTransfer(address erc20, Recipient[] calldata recipients) external {
        for (uint256 i = 0; i < recipients.length; i++) {
            require(IERC20(erc20).transfer(recipients[i].account, recipients[i].amount), "erc20 transfer failed");
        }
    }
}

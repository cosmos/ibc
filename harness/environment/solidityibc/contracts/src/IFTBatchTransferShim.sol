// SPDX-License-Identifier: UNLICENSED
// TEST APPLICATION ONLY — not a product contract.
pragma solidity 0.8.28;

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

    function batchIftTransfer(address ift, string calldata clientId, Transfer[] calldata transfers) external {
        for (uint256 i = 0; i < transfers.length; i++) {
            IIFT(ift).iftTransfer(clientId, transfers[i].receiver, transfers[i].amount, transfers[i].timeoutTimestamp);
        }
    }
}

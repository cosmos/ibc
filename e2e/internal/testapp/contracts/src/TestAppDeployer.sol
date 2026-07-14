// SPDX-License-Identifier: UNLICENSED
// TEST-ONLY CONTRACT — not a product contract.
pragma solidity 0.8.28;

import {Counter} from "./Counter.sol";
import {MockGMP} from "./MockGMP.sol";
import {MockIFT} from "./MockIFT.sol";

contract TestAppDeployer {
    event TestAppsDeployed(address mockGMP, address mockIFT, address counter);

    constructor(uint256 initialIFTSupply) {
        MockGMP mockGMP = new MockGMP();
        MockIFT mockIFT = new MockIFT();
        Counter counter = new Counter();
        mockIFT.mint(msg.sender, initialIFTSupply);
        emit TestAppsDeployed(address(mockGMP), address(mockIFT), address(counter));
    }
}

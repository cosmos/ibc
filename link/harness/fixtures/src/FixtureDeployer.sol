// SPDX-License-Identifier: UNLICENSED
// TEST-ONLY FIXTURE — not a product contract.
pragma solidity 0.8.28;

import {Counter} from "./Counter.sol";
import {MockGMP} from "./MockGMP.sol";
import {MockIFT} from "./MockIFT.sol";

contract FixtureDeployer {
    event FixturesDeployed(address mockGMP, address mockIFT, address counter);

    constructor(uint256 initialIFTSupply) {
        MockGMP mockGMP = new MockGMP();
        MockIFT mockIFT = new MockIFT();
        Counter counter = new Counter();
        mockIFT.mint(msg.sender, initialIFTSupply);
        emit FixturesDeployed(address(mockGMP), address(mockIFT), address(counter));
    }
}

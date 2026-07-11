// SPDX-License-Identifier: UNLICENSED
// TEST-ONLY FIXTURE — not a product contract.
//
// Counter is the GMP delivery target the harness uses for exactly-once assertions: a relayed GMP
// message calls increment() exactly once, and the test reads count() to prove the destination-side
// effect happened precisely once (no double-deliver, no missed deliver).
pragma solidity 0.8.28;

contract Counter {
    uint256 private _count;

    /// @notice Current counter value. The GMP exactly-once assertion reads this.
    function count() external view returns (uint256) {
        return _count;
    }

    /// @notice Increment the counter by one. The canonical GMP target call.
    function increment() external {
        _count += 1;
    }
}

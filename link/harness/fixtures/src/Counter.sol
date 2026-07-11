// SPDX-License-Identifier: UNLICENSED
// TEST-ONLY FIXTURE — not a product contract.
pragma solidity 0.8.28;

contract Counter {
    uint256 private _count;

    function count() external view returns (uint256) {
        return _count;
    }

    function increment() external {
        _count += 1;
    }
}

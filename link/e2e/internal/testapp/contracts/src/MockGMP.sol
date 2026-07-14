// SPDX-License-Identifier: UNLICENSED
// TEST APPLICATION ONLY — not a product contract.
pragma solidity 0.8.28;

contract MockGMP {
    uint256 private _seq;

    // The synthetic relayer scans this event; target is a destination EVM address string.
    event GMPSent(uint256 seq, string routeId, string target, bytes payload);
    event GMPReceived(string routeId, uint256 seq, address target, bool success);

    function send(string calldata routeId, string calldata target, bytes calldata payload) external {
        uint256 s = ++_seq;
        emit GMPSent(s, routeId, target, payload);
    }

    function deliver(string calldata routeId, uint256 s, address target, bytes calldata payload) external {
        (bool success,) = target.call(payload);
        emit GMPReceived(routeId, s, target, success);
    }
}

// SPDX-License-Identifier: UNLICENSED
// TEST-ONLY FIXTURE — not a product contract.
pragma solidity 0.8.28;

contract MockGMP {
    uint256 public seq;

    // The relayer scans this event; routeId identifies its route and target uses the ICS20 string form.
    event GMPSent(uint256 seq, string routeId, string target, bytes payload);
    event GMPReceived(uint256 seq, address target, bool success);

    function send(string calldata routeId, string calldata target, bytes calldata payload) external returns (uint256) {
        uint256 s = ++seq;
        emit GMPSent(s, routeId, target, payload);
        return s;
    }

    function deliver(uint256 s, address target, bytes calldata payload) external {
        (bool success,) = target.call(payload);
        emit GMPReceived(s, target, success);
    }
}

// SPDX-License-Identifier: UNLICENSED
// TEST-ONLY FIXTURE — not a product contract.
//
// MockGMP mocks the general-message-passing source and destination surfaces. On the source chain a
// user calls send(); the relayer discovers the message by SCANNING GMPSent, then on the destination chain
// calls deliver(), which performs the real on-chain effect (target.call) so the harness's terminal
// assertion is genuine even though the protocol in between is mocked.
pragma solidity 0.8.28;

contract MockGMP {
    /// @notice Monotonic sequence assigned to each send(); first message is 1.
    uint256 public seq;

    // routeId is the mock stand-in for IBC v2's source client id (which maps 1:1 to a relayer route);
    // real GMP carries no route in the packet, but the relayer discovers a mock message by scanning this
    // event and must recover which route emitted it. target uses the ICS20-shaped string form and the
    // delivery leg parses its 0x hex value back to an address.
    event GMPSent(uint256 seq, string routeId, string target, bytes payload);
    event GMPReceived(uint256 seq, address target, bool success);

    /// @notice Emit a GMP message for the relayer to discover. routeId names the emitting route (the mock
    /// stand-in for the source client id); target is the ICS20-shaped destination account string. Returns
    /// the assigned sequence.
    function send(string calldata routeId, string calldata target, bytes calldata payload) external returns (uint256) {
        uint256 s = ++seq;
        emit GMPSent(s, routeId, target, payload);
        return s;
    }

    /// @notice Destination-side delivery: invoke the target with payload and record the outcome.
    /// Mirrors the brief's deliver(seq, target, payload); seq is the source sequence being delivered.
    function deliver(uint256 s, address target, bytes calldata payload) external {
        (bool success,) = target.call(payload);
        emit GMPReceived(s, target, success);
    }
}

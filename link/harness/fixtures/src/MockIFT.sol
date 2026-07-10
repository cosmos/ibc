// SPDX-License-Identifier: UNLICENSED
// TEST-ONLY FIXTURE — not a product contract.
//
// MockIFT is a minimal, hand-rolled mintable token (no OpenZeppelin, on purpose) that models the
// escrow/mint semantics of an interchain fungible token. sendTransfer escrows from the caller on the
// source chain (recording a per-sequence escrow with a timeout) and emits IFTSent; receiveTransfer
// mints to the receiver on the destination chain and emits IFTReceived. The relayer that connects the
// two sides discovers a packet by SCANNING IFTSent — so the event must carry the packet's route and its
// full ICS20-shaped data (a string receiver, which can name a non-EVM destination account), not just a
// sequence.
//
// M5 adds the TIMEOUT/REFUND leg: each sendTransfer carries a timeoutTimestamp and escrows the amount
// against its sequence. If the packet is not delivered by the deadline, refund(seq) releases the
// escrow back to the original sender on the SOURCE chain (the relayer calls it once it observes the
// destination clock past the timeout with no destination receive). A timeoutTimestamp of 0 means "no
// timeout" — the M3/M4 happy path uses it and refund rejects it, so those flows are unchanged.
pragma solidity 0.8.28;

contract MockIFT {
    /// @notice Token balance per holder. Public mapping auto-generates the balanceOf getter.
    mapping(address => uint256) public balanceOf;
    /// @notice Total minted-minus-escrowed supply; a coarse invariant the harness can sanity-check.
    uint256 public totalSupply;
    /// @notice Monotonic transfer sequence; first sendTransfer is 1.
    uint256 public seq;
    /// @notice Transitional ICS27-GMP executor allowed to call iftMint.
    address private immutable gmpExecutor;

    /// @notice Per-sequence source-side escrow recorded by sendTransfer, settled by refund.
    struct Escrow {
        address sender; // who to refund
        uint256 amount; // escrowed amount
        uint256 timeoutTimestamp; // relayer-observed destination deadline (0 = no timeout)
        bool settled; // true once refunded — makes refund once-only
    }
    /// @notice Escrows keyed by sequence. Public getter lets the harness inspect a packet's escrow.
    mapping(uint256 => Escrow) public escrows;

    // routeId is the mock stand-in for IBC v2's source client id (which maps 1:1 to a relayer route);
    // real IFT carries no route in the packet, but the relayer discovers a mock packet by scanning this
    // event and must recover which route emitted it. receiver is a string (matching ICS20 packet data),
    // so a source escrow can name a non-EVM destination account with no address placeholder.
    event IFTSent(uint256 seq, string routeId, string receiver, uint256 amount, uint256 timeoutTimestamp);
    event IFTReceived(uint256 seq, address receiver, uint256 amount);
    event IFTMintReceived(string clientId, address indexed receiver, uint256 amount);
    event IFTRefunded(uint256 seq, address sender, uint256 amount);

    constructor(address executor) {
        require(executor != address(0), "zero GMP executor");
        gmpExecutor = executor;
    }

    /// @notice Test-setup helper: mint freely. No access control — this is a test-only fixture.
    function mint(address to, uint256 amount) external {
        balanceOf[to] += amount;
        totalSupply += amount;
    }

    /// @notice Source side: escrow `amount` from the caller against sequence s and emit IFTSent for the
    /// relayer to discover. routeId names the emitting route (the mock stand-in for the source client id);
    /// receiver is the ICS20-shaped destination account string (an EVM 0x hex or a cosmos1 bech32). The
    /// escrow keys on the caller/sequence, never the receiver, so a non-EVM receiver string is carried
    /// verbatim in the event without affecting the debit. timeoutTimestamp (0 = none) is the deadline
    /// after which refund(s) releases the escrow.
    function sendTransfer(string calldata routeId, string calldata receiver, uint256 amount, uint256 timeoutTimestamp)
        external
        returns (uint256)
    {
        require(balanceOf[msg.sender] >= amount, "insufficient balance");
        balanceOf[msg.sender] -= amount;
        totalSupply -= amount; // escrow/burn on the source side
        uint256 s = ++seq;
        escrows[s] = Escrow({sender: msg.sender, amount: amount, timeoutTimestamp: timeoutTimestamp, settled: false});
        emit IFTSent(s, routeId, receiver, amount, timeoutTimestamp);
        return s;
    }

    /// @notice Destination side: mint `amount` to `receiver` and emit IFTReceived. `s` is the source
    /// sequence being completed (lets the harness correlate send→receive). This legacy path is used only
    /// for mock EVM-source routes; Cosmos destinations mint through their native IFT module.
    function receiveTransfer(uint256 s, address receiver, uint256 amount) external {
        balanceOf[receiver] += amount;
        totalSupply += amount; // mint on the destination side
        emit IFTReceived(s, receiver, amount);
    }

    /// @notice Destination side for native IFT packets. The calldata and event match IIFT.iftMint;
    /// packet identity remains the enclosing GMPReceived event emitted by the configured executor.
    function iftMint(address receiver, uint256 amount) external {
        require(msg.sender == gmpExecutor, "unauthorized GMP executor");
        (bool ok, bytes memory result) = msg.sender.staticcall(abi.encodeWithSignature("deliveryClientId()"));
        require(ok, "invalid GMP context");
        string memory clientId = abi.decode(result, (string));
        require(bytes(clientId).length != 0, "missing GMP context");
        balanceOf[receiver] += amount;
        totalSupply += amount;
        emit IFTMintReceived(clientId, receiver, amount);
    }

    /// @notice Source side: release the escrow for sequence s back to its original sender after the relayer
    /// has verified timeout/non-delivery. Rejects an unknown escrow, a no-timeout escrow, or a double
    /// refund.
    function refund(uint256 s) external {
        Escrow storage e = escrows[s];
        require(e.amount > 0, "no escrow");
        require(e.timeoutTimestamp != 0, "no timeout set");
        require(!e.settled, "already refunded");
        e.settled = true;
        balanceOf[e.sender] += e.amount;
        totalSupply += e.amount; // un-escrow back into circulating supply
        emit IFTRefunded(s, e.sender, e.amount);
    }
}

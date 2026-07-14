// SPDX-License-Identifier: UNLICENSED
// TEST APPLICATION ONLY — not a product contract.
pragma solidity 0.8.28;

contract MockIFT {
    mapping(address => uint256) public balanceOf;
    uint256 private _seq;
    struct Escrow {
        address sender;
        uint256 amount;
        uint256 timeoutTimestamp;
        bool settled;
    }
    mapping(uint256 => Escrow) private _escrows;

    // The synthetic relayer scans this event; receiver is a destination EVM address string.
    event IFTSent(uint256 seq, string routeId, string receiver, uint256 amount, uint256 timeoutTimestamp);
    event IFTReceived(string routeId, uint256 seq, address receiver, uint256 amount);
    event IFTRefunded(uint256 seq, address sender, uint256 amount);

    function mint(address to, uint256 amount) external {
        balanceOf[to] += amount;
    }

    // timeoutTimestamp == 0 disables refunds.
    function sendTransfer(string calldata routeId, string calldata receiver, uint256 amount, uint256 timeoutTimestamp)
        external
    {
        require(balanceOf[msg.sender] >= amount, "insufficient balance");
        balanceOf[msg.sender] -= amount;
        uint256 s = ++_seq;
        _escrows[s] = Escrow({sender: msg.sender, amount: amount, timeoutTimestamp: timeoutTimestamp, settled: false});
        emit IFTSent(s, routeId, receiver, amount, timeoutTimestamp);
    }

    function receiveTransfer(string calldata routeId, uint256 s, address receiver, uint256 amount) external {
        balanceOf[receiver] += amount;
        emit IFTReceived(routeId, s, receiver, amount);
    }

    function refund(uint256 s) external {
        Escrow storage e = _escrows[s];
        require(e.amount > 0, "no escrow");
        require(e.timeoutTimestamp != 0, "no timeout set");
        require(!e.settled, "already refunded");
        e.settled = true;
        balanceOf[e.sender] += e.amount;
        emit IFTRefunded(s, e.sender, e.amount);
    }
}

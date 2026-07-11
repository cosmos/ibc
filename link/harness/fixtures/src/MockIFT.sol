// SPDX-License-Identifier: UNLICENSED
// TEST-ONLY FIXTURE — not a product contract.
pragma solidity 0.8.28;

contract MockIFT {
    mapping(address => uint256) public balanceOf;
    uint256 public totalSupply;
    uint256 public seq;
    struct Escrow {
        address sender;
        uint256 amount;
        uint256 timeoutTimestamp;
        bool settled;
    }
    mapping(uint256 => Escrow) public escrows;

    // The relayer scans this event; routeId identifies its route and receiver uses the ICS20 string form.
    event IFTSent(uint256 seq, string routeId, string receiver, uint256 amount, uint256 timeoutTimestamp);
    event IFTReceived(uint256 seq, address receiver, uint256 amount);
    event IFTRefunded(uint256 seq, address sender, uint256 amount);

    function mint(address to, uint256 amount) external {
        balanceOf[to] += amount;
        totalSupply += amount;
    }

    // timeoutTimestamp == 0 disables refunds.
    function sendTransfer(string calldata routeId, string calldata receiver, uint256 amount, uint256 timeoutTimestamp)
        external
        returns (uint256)
    {
        require(balanceOf[msg.sender] >= amount, "insufficient balance");
        balanceOf[msg.sender] -= amount;
        totalSupply -= amount;
        uint256 s = ++seq;
        escrows[s] = Escrow({sender: msg.sender, amount: amount, timeoutTimestamp: timeoutTimestamp, settled: false});
        emit IFTSent(s, routeId, receiver, amount, timeoutTimestamp);
        return s;
    }

    function receiveTransfer(uint256 s, address receiver, uint256 amount) external {
        balanceOf[receiver] += amount;
        totalSupply += amount;
        emit IFTReceived(s, receiver, amount);
    }

    function refund(uint256 s) external {
        Escrow storage e = escrows[s];
        require(e.amount > 0, "no escrow");
        require(e.timeoutTimestamp != 0, "no timeout set");
        require(!e.settled, "already refunded");
        e.settled = true;
        balanceOf[e.sender] += e.amount;
        totalSupply += e.amount;
        emit IFTRefunded(s, e.sender, e.amount);
    }
}

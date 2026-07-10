// SPDX-License-Identifier: UNLICENSED
pragma solidity 0.8.28;

import {MockGMP} from "../src/MockGMP.sol";
import {MockIFT} from "../src/MockIFT.sol";

contract MockIFTAdapterTest {
    function testCanonicalMintRunsThroughGMP() external {
        MockGMP gmp = new MockGMP();
        MockIFT ift = new MockIFT(address(gmp));
        address receiver = address(0xBEEF);
        bytes memory payload = abi.encodeCall(MockIFT.iftMint, (receiver, 42));

        gmp.deliverIFT(7, "client-a", address(ift), payload);

        require(ift.balanceOf(receiver) == 42, "receiver was not minted");
        require(bytes(gmp.deliveryClientId()).length == 0, "delivery context leaked");
    }

    function testDirectMintIsRejected() external {
        MockGMP gmp = new MockGMP();
        MockIFT ift = new MockIFT(address(gmp));

        (bool ok,) = address(ift).call(abi.encodeCall(MockIFT.iftMint, (address(this), 42)));

        require(!ok, "direct mint succeeded");
        require(ift.balanceOf(address(this)) == 0, "direct mint changed balance");
    }
}

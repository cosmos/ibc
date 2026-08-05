// SPDX-License-Identifier: MIT
pragma solidity ^0.8.28;

import { stdJson } from "forge-std/StdJson.sol";
import { Script } from "forge-std/Script.sol";
import { Strings } from "@openzeppelin-contracts/utils/Strings.sol";

// Deploys an AttestationLightClient from prebuilt bytecode with its
// constructor args taken from the environment.
contract DeployAttestationClient is Script {
    using stdJson for string;

    string internal constant ARTIFACT = "release-bytecode/AttestationLightClient.json";

    function run() public returns (string memory) {
        address[] memory attestors = vm.envAddress("IBC_ATTESTORS", ",");
        uint8 threshold = uint8(vm.envUint("IBC_THRESHOLD"));
        uint64 height = uint64(vm.envUint("IBC_HEIGHT"));
        uint64 timestamp = uint64(vm.envUint("IBC_TIMESTAMP"));
        address roleManager = vm.envOr("IBC_ROLE_MANAGER", address(0));

        bytes memory code = bytes.concat(
            vm.getCode(ARTIFACT),
            abi.encode(attestors, threshold, height, timestamp, roleManager)
        );

        vm.startBroadcast(vm.envUint("IBC_DEPLOYER_KEY"));
        address client;
        assembly {
            client := create(0, add(code, 0x20), mload(code))
        }
        vm.stopBroadcast();
        require(client != address(0), "attestation client create failed");

        string memory json = "json";
        return json.serialize("client", Strings.toHexString(client));
    }
}

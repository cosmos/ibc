// SPDX-License-Identifier: MIT
pragma solidity ^0.8.28;

import { stdJson } from "forge-std/StdJson.sol";
import { Script } from "forge-std/Script.sol";
import { Strings } from "@openzeppelin-contracts/utils/Strings.sol";
import { ERC1967Proxy } from "@openzeppelin-contracts/proxy/ERC1967/ERC1967Proxy.sol";
import { AccessManager } from "@openzeppelin-contracts/access/manager/AccessManager.sol";

// Deploys AccessManager + ICS26Router (implementation from prebuilt
// bytecode) behind an initialized ERC1967 proxy. The deployer key stays the
// AccessManager admin; role hardening is a follow-up performed off-script.
contract DeployCore is Script {
    using stdJson for string;

    string internal constant ARTIFACT_ROUTER = "release-bytecode/ICS26Router.json";

    function run() public returns (string memory) {
        uint256 key = vm.envUint("IBC_DEPLOYER_KEY");
        vm.startBroadcast(key);
        AccessManager am = new AccessManager(vm.addr(key));
        address routerLogic = _deployArtifact(ARTIFACT_ROUTER);
        address router = address(
            new ERC1967Proxy(routerLogic, abi.encodeWithSignature("initialize(address)", address(am)))
        );
        vm.stopBroadcast();

        string memory json = "json";
        json.serialize("accessManager", Strings.toHexString(address(am)));
        json.serialize("ics26RouterImplementation", Strings.toHexString(routerLogic));
        return json.serialize("ics26Router", Strings.toHexString(router));
    }

    function _deployArtifact(string memory path) internal returns (address addr) {
        bytes memory code = vm.getCode(path);
        assembly {
            addr := create(0, add(code, 0x20), mload(code))
        }
        require(addr != address(0), string.concat("create failed: ", path));
    }
}

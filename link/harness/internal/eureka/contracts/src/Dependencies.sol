// SPDX-License-Identifier: MIT
pragma solidity ^0.8.28;

// Eureka v3.0.2 deploys OpenZeppelin's AccessManager directly, but its Go
// bindings do not include that contract's creation bytecode. Importing the
// exact OpenZeppelin release pinned by Eureka produces the one missing
// artifact; the Eureka contracts themselves come from the pinned upstream Go
// bindings.
import {AccessManager} from "@openzeppelin/contracts/access/manager/AccessManager.sol";


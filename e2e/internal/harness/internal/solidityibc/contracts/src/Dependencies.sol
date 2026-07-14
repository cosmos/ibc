// SPDX-License-Identifier: MIT
pragma solidity ^0.8.28;

// Solidity IBC v3.0.2 deploys OpenZeppelin's AccessManager directly, but its Go
// bindings do not include that contract's creation bytecode. Importing the
// exact OpenZeppelin release pinned by Solidity IBC produces the one missing
// artifact; the Solidity IBC contracts themselves come from the pinned upstream Go
// bindings.
import {AccessManager} from "@openzeppelin/contracts/access/manager/AccessManager.sol";


// SPDX-License-Identifier: Apache-2.0

// Package besu provides header parsing, consensus state hashing, storage slot
// derivation and ABI encoding for Besu light-client payloads. Consensus
// verification belongs to the contract, simulated by the transaction submitter.
//
// Wire types and codecs are generated upstream. The contract stores only
// keccak256(abi.encode(ConsensusState)) per height, so updates and membership
// proofs carry the consensus state preimage they rely on.
package besu

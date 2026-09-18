// SPDX-License-Identifier: Apache-2.0

// Package besu holds the pure helpers the relayer, the deploy driver and the
// e2e harness share for Besu BFT light clients: header parsing, QBFT commit
// seal digests and signer recovery, the light client's threshold rules,
// consensus state hashing, storage slot derivation and the ABI encoders for
// the BesuQBFTLightClient wire formats.
//
// Header parsing retains raw fields for the sealing digest.
//
// Wire types and codecs are generated upstream.
// The Go consensus algorithms follow ibc-contracts/ibc-solidity/contracts/light-clients/besu on the
// hashed-consensus-state design: the contract stores only
// keccak256(abi.encode(ConsensusState)) per height, so every update and every
// membership proof carries the consensus state preimage it relies on.
package besu

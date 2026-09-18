// SPDX-License-Identifier: Apache-2.0

// Package headerlayout defines the Ethereum header and Besu BFT extraData RLP
// positions shared by the codec and test builder. They match _parseHeader in
// ibc-contracts/ibc-solidity/contracts/light-clients/besu/BesuLightClientBase.sol.
// Raw items preserve optional and unknown trailing fields in the sealing digest.
package headerlayout

// RLP list sizes and zero-based field positions.
const (
	MinHeaderItems     = 15
	ExtraDataItemCount = 5

	IdxStateRoot = 3
	IdxNumber    = 8
	IdxTimestamp = 11
	IdxExtraData = 12

	ExtraIdxValidators  = 1
	ExtraIdxCommitSeals = 4
)

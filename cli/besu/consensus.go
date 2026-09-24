// SPDX-License-Identifier: Apache-2.0

package besu

import "github.com/cosmos/solidity-ibc-eureka/packages/go-abigen/besumsgs"

// ConsensusStateOf is the consensus state an update to header installs, and
// the preimage payloads carry for header's height.
func ConsensusStateOf(header *ParsedHeader) besumsgs.IBesuLightClientMsgsConsensusState {
	return besumsgs.IBesuLightClientMsgsConsensusState{
		Timestamp:  header.Timestamp,
		StateRoot:  header.StateRoot,
		Validators: header.Validators,
	}
}

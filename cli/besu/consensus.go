// SPDX-License-Identifier: Apache-2.0

package besu

import (
	"fmt"

	"github.com/cosmos/solidity-ibc-eureka/packages/go-abigen/besumsgs"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
)

// HashConsensusState is keccak256(abi.encode(ConsensusState)), the value the light client
// stores per height. The struct holds a dynamic array, so the encoding starts
// with an offset word.
func HashConsensusState(state besumsgs.IBesuLightClientMsgsConsensusState) (common.Hash, error) {
	data, err := messageBindings.TryPackConsensusState(state)
	if err != nil {
		return common.Hash{}, fmt.Errorf("encode consensus state: %w", err)
	}

	return crypto.Keccak256Hash(data[4:]), nil
}

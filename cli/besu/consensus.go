// SPDX-License-Identifier: Apache-2.0

package besu

import (
	"fmt"
	"maps"
	"slices"

	"github.com/ethereum/go-ethereum/accounts/abi/bind"

	"github.com/cosmos/ibc-contracts/packages/go-abigen/besuibft2"
	"github.com/cosmos/ibc-contracts/packages/go-abigen/besuqbft"
)

// Consensus is the BFT protocol a Besu chain runs. It decides the digest
// commit seals sign, and so which light client contract verifies the chain.
// Values match the genesis config key Besu reads them from.
type Consensus string

// Supported consensus protocols.
const (
	QBFT  Consensus = "qbft"
	IBFT2 Consensus = "ibft2"
)

// lightClients is the contract verifying each supported consensus. The
// contracts share their ABI and differ only in the commit seal digest.
var lightClients = map[Consensus]*bind.MetaData{
	QBFT:  besuqbft.ContractMetaData,
	IBFT2: besuibft2.ContractMetaData,
}

// Consensuses lists the supported consensus protocols.
var Consensuses = slices.Sorted(maps.Keys(lightClients))

// Validate fails unless c is a supported consensus.
func (c Consensus) Validate() error {
	_, err := c.LightClient()
	return err
}

// LightClient is the light client contract that verifies a chain running c.
func (c Consensus) LightClient() (*bind.MetaData, error) {
	meta, ok := lightClients[c]
	if !ok {
		return nil, fmt.Errorf("unknown besu consensus %q", c)
	}

	return meta, nil
}

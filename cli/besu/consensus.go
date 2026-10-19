// SPDX-License-Identifier: Apache-2.0

package besu

import "fmt"

// Consensus is the BFT protocol a Besu chain runs. It decides the digest
// commit seals sign, and so which light client contract verifies the chain.
// Values match the genesis config key Besu reads them from.
type Consensus string

// Supported consensus protocols.
const (
	QBFT  Consensus = "qbft"
	IBFT2 Consensus = "ibft2"
)

// Validate fails unless c is a supported consensus.
func (c Consensus) Validate() error {
	switch c {
	case QBFT, IBFT2:
		return nil
	default:
		return fmt.Errorf("unknown besu consensus %q", c)
	}
}

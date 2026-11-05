// SPDX-License-Identifier: Apache-2.0

package besu

import (
	"fmt"
	"slices"
)

// Consensus is the BFT protocol a Besu chain runs. It decides the header
// form commit seals sign. Values match the genesis config key Besu reads
// them from.
type Consensus string

// Supported consensus protocols.
const (
	QBFT  Consensus = "qbft"
	IBFT2 Consensus = "ibft2"
)

// Consensuses lists the supported consensus protocols.
var Consensuses = []Consensus{IBFT2, QBFT}

// Validate fails unless c is a supported consensus.
func (c Consensus) Validate() error {
	if !slices.Contains(Consensuses, c) {
		return fmt.Errorf("unknown besu consensus %q", c)
	}

	return nil
}

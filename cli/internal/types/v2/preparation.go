// SPDX-License-Identifier: Apache-2.0

package v2

import "fmt"

// Preparation is one bounded step towards relaying a batch. Advance is a
// client-only update the relayer confirms before preparing again; Ready holds
// the final update and one proof per packet, submitted in one transaction.
type Preparation struct {
	Advance []byte
	Ready   *BatchProofs
}

// BatchProofs is one snapshot at the requested height. Update is optional.
type BatchProofs struct {
	Update       []byte
	PacketProofs [][]byte
}

func (p *Preparation) Validate(packetCount int) error {
	if p == nil || (len(p.Advance) == 0) == (p.Ready == nil) {
		return fmt.Errorf("preparation must contain exactly one of advance or ready")
	}
	if p.Ready == nil {
		return nil
	}
	if len(p.Ready.PacketProofs) != packetCount {
		return fmt.Errorf("preparation returned %d proofs for %d packets", len(p.Ready.PacketProofs), packetCount)
	}
	for i, proof := range p.Ready.PacketProofs {
		if len(proof) == 0 {
			return fmt.Errorf("packet proof %d is empty", i)
		}
	}
	return nil
}

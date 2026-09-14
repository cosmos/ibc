// SPDX-License-Identifier: Apache-2.0

package v2

import (
	"errors"
	"fmt"
)

// ErrTxTooLarge identifies transactions that cannot fit within the block gas limit.
var ErrTxTooLarge = errors.New("transaction exceeds chain gas capacity")

// Preparation is one bounded read-only step towards relaying a batch.
// Advance contains one client-only update; Ready contains proofs for all packets.
// Only confirmed on-chain state may be used as the next trusted checkpoint.
type Preparation struct {
	Advance []byte
	Ready   *BatchProofs
}

// BatchProofs shares a snapshot at the requested height. Update is optional.
// Checkpoint permits submitting Update separately if the final batch is too large.
type BatchProofs struct {
	Update       []byte
	PacketProofs [][]byte
	Checkpoint   bool
}

func (p *Preparation) Validate(packetCount int) error {
	if p == nil || (len(p.Advance) == 0) == (p.Ready == nil) {
		return fmt.Errorf("preparation must contain exactly one of advance or ready")
	}
	if p.Ready != nil {
		if len(p.Ready.PacketProofs) != packetCount {
			return fmt.Errorf("preparation returned %d proofs for %d packets", len(p.Ready.PacketProofs), packetCount)
		}
		for i, proof := range p.Ready.PacketProofs {
			if len(proof) == 0 {
				return fmt.Errorf("packet proof %d is empty", i)
			}
		}
		if p.Ready.Checkpoint && len(p.Ready.Update) == 0 {
			return fmt.Errorf("checkpoint requires a client update")
		}
	}
	return nil
}

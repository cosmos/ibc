// SPDX-License-Identifier: Apache-2.0

package besu

import (
	"fmt"

	"github.com/ethereum/go-ethereum/common"
)

// OverlapRequired is how many commit seal signers of a new header must belong
// to the trusted validator set: strictly more than one third of it.
func OverlapRequired(trusted int) int { return trusted/3 + 1 }

// QuorumRequired is how many commit seal signers must belong to the new
// header's own validator set: Besu's ceil(2n/3), written n - floor(n/3).
func QuorumRequired(validators int) int { return validators - validators/3 }

// CheckUpdate reports whether target, signed by signers, is accepted as an
// update from a client that trusts the validators in trusted. It runs both
// threshold rules and flags the case where no signer is a header validator,
// which points at a header encoding problem rather than turnover.
func CheckUpdate(target *Header, signers []common.Address, trusted ConsensusState) error {
	headerSigners := countMembers(signers, target.Validators)
	hint := ""
	if headerSigners == 0 {
		hint = " (no recovered signer is a header validator: suspect header RLP fidelity)"
	}

	overlap := countMembers(signers, trusted.Validators)
	required := OverlapRequired(len(trusted.Validators))
	if overlap < required {
		return fmt.Errorf(
			"%w: %d signers in the trusted set, %d required%s",
			ErrInsufficientOverlap, overlap, required, hint,
		)
	}
	if required := QuorumRequired(len(target.Validators)); headerSigners < required {
		return fmt.Errorf(
			"%w: %d signers in the header set, %d required%s",
			ErrInsufficientQuorum, headerSigners, required, hint,
		)
	}

	return nil
}

func countMembers(signers, set []common.Address) int {
	members := make(map[common.Address]struct{}, len(set))
	for _, address := range set {
		members[address] = struct{}{}
	}

	count := 0

	for _, signer := range signers {
		if _, ok := members[signer]; ok {
			count++
		}
	}

	return count
}

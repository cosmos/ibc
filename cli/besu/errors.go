// SPDX-License-Identifier: Apache-2.0

package besu

import "errors"

// Sentinel errors.
var (
	ErrInvalidHeader       = errors.New("invalid besu header")
	ErrInvalidSeal         = errors.New("invalid commit seal")
	ErrDuplicateSigner     = errors.New("duplicate commit seal signer")
	ErrInsufficientOverlap = errors.New("insufficient trusted validator overlap")
	ErrInsufficientQuorum  = errors.New("insufficient validator quorum")
)

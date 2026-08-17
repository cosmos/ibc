// SPDX-License-Identifier: Apache-2.0

package v2

import (
	"math"

	"github.com/cosmos/ibc/link/proofgen"
)

// BlockHeader is the minimal view of a block the attestor and proof generators
// need: a height and its timestamp.
//
// Defined in the public proofgen package so custom light clients can implement
// against it; aliased here so internal callers are unaffected.
type BlockHeader = proofgen.BlockHeader

// Special markers for different block heights.
const (
	LatestBlock    = math.MaxUint64
	FinalizedBlock = LatestBlock - 1
)

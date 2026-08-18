// SPDX-License-Identifier: Apache-2.0

package v2

import (
	"math"
	"time"
)

// BlockHeader is the minimal view of a block the attestor and proof generators
// need: a height and its timestamp.
type BlockHeader struct {
	Height    uint64
	Timestamp time.Time
}

// Special markers for different block heights.
const (
	LatestBlock    = math.MaxUint64
	FinalizedBlock = LatestBlock - 1
)

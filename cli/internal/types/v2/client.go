// SPDX-License-Identifier: Apache-2.0

package v2

import (
	"math"
	"time"
)

// BlockHeader represents a minimal subset of fields that IBC client needs for *attestation*.
type BlockHeader struct {
	Height    uint64
	Timestamp time.Time
}

// Special markers for different block heights.
const (
	LatestBlock    = math.MaxUint64
	FinalizedBlock = LatestBlock - 1
)

// Subscription an active event stream.
type Subscription interface {
	// Err returns the error that ended the subscription, if any. It is closed
	// when the subscription ends for any reason.
	Err() <-chan error

	// Unsubscribe ends the subscription and releases its resources.
	Unsubscribe()
}

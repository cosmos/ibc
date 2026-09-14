// SPDX-License-Identifier: Apache-2.0

package v2

import (
	"errors"
	"math"
	"math/big"
	"time"
)

// ErrConsensusStateNotFound reports that a light client stores nothing at the
// requested height.
var ErrConsensusStateNotFound = errors.New("consensus state not found")

// BlockHeader represents a minimal subset of fields that IBC client needs for *attestation*.
type BlockHeader struct {
	Height    uint64
	Timestamp time.Time
}

// AccountProof is an eth_getProof result for one account at one height: the
// account's storage root, the account proof nodes against the block's state
// root and one storage proof per requested slot, in request order.
type AccountProof struct {
	StorageRoot   [32]byte
	AccountProof  [][]byte
	StorageProofs []StorageProof
}

// StorageProof is one storage slot's value and trie proof nodes from
// eth_getProof. Value is zero for an absent slot.
type StorageProof struct {
	Key   [32]byte
	Value *big.Int
	Proof [][]byte
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

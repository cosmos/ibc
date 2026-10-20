// SPDX-License-Identifier: Apache-2.0

package chain

import (
	"context"
	"crypto/ecdsa"
	"math/big"
	"time"

	"github.com/ethereum/go-ethereum/common"
)

type BlockController interface {
	MineBlocks(ctx context.Context, n int) error
	PauseMining(ctx context.Context) error
	ResumeMining(ctx context.Context) error
	AdvanceTime(ctx context.Context, d time.Duration) error
}

type FaultInjector interface {
	StopNode(ctx context.Context) error
	StartNode(ctx context.Context) error
}

// EOAFunder guarantees a verified minimum native-token balance for an
// externally owned account. Managed Chain adapters implement the mechanism
// without exposing their funding identities or controls to workflow callers.
type EOAFunder interface {
	EnsureEOABalance(ctx context.Context, address common.Address, minimum *big.Int) error
}

// ValidatorVoter exposes a BFT chain's genesis validator keys and changes its
// validator set through header voting, across nodes the managed Chain runs.
type ValidatorVoter interface {
	ValidatorKeys() []*ecdsa.PrivateKey
	Nodes() []common.Address
	Validators(ctx context.Context) ([]common.Address, error)
	VoteValidator(ctx context.Context, validator common.Address, add bool) error
}

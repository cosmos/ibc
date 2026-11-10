// SPDX-License-Identifier: Apache-2.0

package processors

import (
	"context"
	"time"

	"github.com/cosmos/ibc/cli/internal/store"
)

// waitForChainTimeout bounds how long batch delivery waits for the target
// chain to catch up to the current time before simulation.
const waitForChainTimeout = 2 * time.Minute

// TxStorage persists batch delivery results transactionally.
type TxStorage interface {
	Transact(ctx context.Context, fn func(store.Repository) error) error
}

// nodeLagWarningAfter how long a finality wait can run before warning that the
// node may be lagging.
const nodeLagWarningAfter = 30 * time.Minute

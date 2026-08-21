// SPDX-License-Identifier: Apache-2.0

package processors

import (
	"context"
	"time"

	"github.com/cosmos/ibc/link/internal/chains"
	"github.com/cosmos/ibc/link/internal/relay/prover"
	"github.com/cosmos/ibc/link/internal/relay/txbuilder"
	"github.com/cosmos/ibc/link/internal/store"
)

// waitForChainTimeout bounds how long batch delivery waits for the target
// chain to catch up to the current time before gas estimation.
const waitForChainTimeout = 2 * time.Minute

// TxStorage persists batch delivery results transactionally.
type TxStorage interface {
	Transact(ctx context.Context, fn func(store.Repository) error) error
}

// ChainClients resolves chain clients by chain id.
type ChainClients interface {
	Get(chainID string) (chains.Client, bool)
}

// Provers resolves provers by (chainIDclientID).
type Provers interface {
	Get(chainID, clientID string) (prover.Prover, bool)
}

// TxBuilders resolves tx builders by chain id.
type TxBuilders interface {
	Get(chainID string) (txbuilder.TxBuilder, bool)
}

// nodeLagWarningAfter how long a finality wait can run before warning that the
// node may be lagging.
const nodeLagWarningAfter = 30 * time.Minute

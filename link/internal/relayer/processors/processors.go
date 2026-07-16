package processors

import (
	"context"
	"time"

	"github.com/cosmos/ibc/link/internal/chains"
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

// nodeLagWarningAfter how long a finality wait can run before warning that the
// node may be lagging.
const nodeLagWarningAfter = 30 * time.Minute

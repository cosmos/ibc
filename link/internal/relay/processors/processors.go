package processors

import (
	"context"
	"time"

	"github.com/cosmos/ibc/link/internal/chains"
	"github.com/cosmos/ibc/link/internal/relay/proofgen"
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

// ChainClients resolves chain clients by chain id. Batch processors use the
// same resolved chains.Client both to read packet events out of tx receipts
// and to wait for the chain before submitting.
type ChainClients interface {
	Get(chainID string) (chains.Client, bool)
}

// ProofGenerators resolves proof generators by (chainID, clientID).
type ProofGenerators interface {
	Get(chainID, clientID string) (proofgen.ProofGenerator, bool)
}

// TxBuilders resolves tx builders by chain id.
type TxBuilders interface {
	Get(chainID string) (txbuilder.TxBuilder, bool)
}

// nodeLagWarningAfter how long a finality wait can run before warning that the
// node may be lagging.
const nodeLagWarningAfter = 30 * time.Minute

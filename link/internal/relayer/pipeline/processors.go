package pipeline

import (
	"time"

	"github.com/cosmos/ibc/link/internal/chains"
)

// ChainClients resolves chain clients by chain id.
type ChainClients interface {
	Get(chainID string) (chains.Client, bool)
}

// nodeLagWarningAfter how long a finality wait can run before warning that the
// node may be lagging.
const nodeLagWarningAfter = 30 * time.Minute

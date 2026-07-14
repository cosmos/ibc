// Package chains defines chain-agnostic clients for reading chain state.
package chains

import (
	"context"

	v2 "github.com/cosmos/ibc/link/internal/types/v2"
)

// Client provides chain state queries.
type Client interface {
	TxPacketEvents(ctx context.Context, txHash []byte) ([]v2.PacketEvent, error)
}

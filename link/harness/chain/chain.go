package chain

import "context"

type Chain interface {
	ID() string
	RPCURL() string
	Height(ctx context.Context) (uint64, error)
}

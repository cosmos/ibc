package chain

import "context"

type Family string

const (
	FamilyEVM Family = "evm"
)

type Chain interface {
	ID() string
	Family() Family
	RPCURL() string
	Height(ctx context.Context) (uint64, error)
}

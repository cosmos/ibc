package chain

import "context"

// Family identifies the chain family behind the abstraction — the harness's own vocabulary, distinct
// from the relayer-facing wire.ChainType. The harness uses it to route construction and app-level
// readers while tests stay on the shared Chain surface.
type Family string

const (
	// FamilyEVM identifies an EVM-family chain.
	FamilyEVM Family = "evm"
)

// Chain is the family-neutral chain surface every provisioned chain implements. This package carries
// no family's types: a family-specific view (e.g. the concrete EVM client) and every optional
// operation are separate capabilities discovered via As, so a non-EVM chain implements exactly this.
// Height is core rather than a capability because every real chain has one — it is what diagnostics
// snapshot for any family at teardown.
type Chain interface {
	ID() string
	Family() Family
	RPCURL() string // host-reachable RPC endpoint
	Height(ctx context.Context) (uint64, error)
}

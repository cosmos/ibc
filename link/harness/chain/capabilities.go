package chain

import (
	"context"
	"reflect"
	"time"
)

// The interfaces below are optional capabilities. A chain advertises one by implementing it.

// BlockController drives block production and chain time on chains that support
// direct control. Tests use it to make pending and timeout states deterministic.
type BlockController interface {
	MineBlocks(ctx context.Context, n int) error
	PauseMining(ctx context.Context) error
	ResumeMining(ctx context.Context) error
	AdvanceTime(ctx context.Context, d time.Duration) error
}

// FaultInjector stops and restarts a local node for resilience tests.
type FaultInjector interface {
	StopNode(ctx context.Context) error
	StartNode(ctx context.Context) error
}

// ReceiverProvider mints a fresh destination address in the chain's native form, ready to receive an
// asset transfer. What "ready" takes is family-owned: an implementation performs whatever preparation
// its family requires before the address can receive (for example, funding the fresh account where a
// receiver may need gas afterwards — or nothing at all where receiving creates the account). Callers
// resolve it via As and treat a chain without it as unable to default a receiver, failing with the
// standard missing-capability error rather than silently assuming a family.
type ReceiverProvider interface {
	NewReceiver(ctx context.Context) (string, error)
}

// CapabilityGater is implemented by a chain that structurally satisfies an optional capability it cannot
// honor in every configuration. As consults it so such a capability is negotiated as absent where its
// contract does not hold (e.g. Anvil under interval mining cannot offer reliable block control).
type CapabilityGater interface {
	// ProvidesCapability reports whether the chain currently offers the optional capability of interface
	// type t. A chain returns true for any capability it does not gate.
	ProvidesCapability(t reflect.Type) bool
}

// As returns optional capability T from c and a boolean indicating whether c provides it. A chain that
// implements CapabilityGater may withhold a capability it structurally implements but cannot honor in its
// current configuration.
func As[T any](c Chain) (T, bool) {
	impl, ok := any(c).(T)
	if !ok {
		var zero T
		return zero, false
	}
	if g, ok := any(c).(CapabilityGater); ok && !g.ProvidesCapability(reflect.TypeOf((*T)(nil)).Elem()) {
		var zero T
		return zero, false
	}
	return impl, true
}

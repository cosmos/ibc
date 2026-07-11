package chain

import (
	"context"
	"reflect"
	"time"
)

type BlockController interface {
	MineBlocks(ctx context.Context, n int) error
	PauseMining(ctx context.Context) error
	ResumeMining(ctx context.Context) error
	AdvanceTime(ctx context.Context, d time.Duration) error
}

type FaultInjector interface {
	StopNode(ctx context.Context) error
	StartNode(ctx context.Context) error
}

type ReceiverProvider interface {
	NewReceiver(ctx context.Context) (string, error)
}

type CapabilityGater interface {
	ProvidesCapability(t reflect.Type) bool
}

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

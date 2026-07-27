package environment

import (
	"context"
	"sync"
)

// environmentLease linearizes resolved operations and bound child processes
// with Environment cleanup.
type environmentLease struct {
	mu      sync.Mutex
	closed  bool
	active  int
	drained chan struct{}
}

func (l *environmentLease) use(use func() error) error {
	release, err := l.acquire()
	if err != nil {
		return err
	}
	defer release()
	return use()
}

func (l *environmentLease) acquire() (func(), error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.closed {
		return nil, ErrEnvironmentClosed
	}
	l.active++
	var once sync.Once
	return func() {
		once.Do(l.release)
	}, nil
}

func (l *environmentLease) release() {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.active--
	if l.closed && l.active == 0 && l.drained != nil {
		close(l.drained)
		l.drained = nil
	}
}

func (l *environmentLease) close(ctx context.Context) error {
	l.mu.Lock()
	l.closed = true
	if l.active == 0 {
		l.mu.Unlock()
		return nil
	}
	if l.drained == nil {
		l.drained = make(chan struct{})
	}
	drained := l.drained
	l.mu.Unlock()

	select {
	case <-drained:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

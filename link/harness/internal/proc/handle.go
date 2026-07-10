package proc

import (
	"context"
	"os/exec"
	"sync"
	"syscall"
	"time"
)

// Stop/reap state machine for a Setpgid child: owns cmd.Wait(), idempotent SignalAndWait, pid-recycling guard.
type Handle struct {
	cmd  *exec.Cmd
	done chan struct{}

	mu      sync.Mutex
	waitErr error
	stopped bool
	reaped  bool
}

func Reap(cmd *exec.Cmd, preWait func()) *Handle {
	h := &Handle{cmd: cmd, done: make(chan struct{})}
	go func() {
		if preWait != nil {
			preWait()
		}
		err := cmd.Wait()
		// Publish reaped under mu before closing done so SignalAndWait never signals a recycled pid.
		h.mu.Lock()
		h.waitErr = err
		h.reaped = true
		h.mu.Unlock()
		close(h.done)
	}()
	return h
}

func (h *Handle) Done() <-chan struct{} { return h.done }

func (h *Handle) Err() error {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.waitErr
}

// Signals the group, escalating to SIGKILL after grace; idempotent. Checks reaped under mu
// before signaling so a recycled pid is never killed.
func (h *Handle) SignalAndWait(ctx context.Context, sig syscall.Signal, grace time.Duration) error {
	h.mu.Lock()
	if h.stopped {
		h.mu.Unlock()
		<-h.done
		return nil
	}
	h.stopped = true
	if h.reaped {
		h.mu.Unlock()
		<-h.done
		return nil
	}
	pgid := h.cmd.Process.Pid
	_ = syscall.Kill(-pgid, sig)
	h.mu.Unlock()

	if grace <= 0 {
		<-h.done
		return nil
	}
	timer := time.NewTimer(grace)
	defer timer.Stop()
	select {
	case <-h.done:
		return nil
	case <-timer.C:
		h.escalateKill(pgid)
		<-h.done
		return nil
	case <-ctx.Done():
		h.escalateKill(pgid)
		<-h.done
		return ctx.Err()
	}
}

func (h *Handle) escalateKill(pgid int) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.reaped {
		return
	}
	_ = syscall.Kill(-pgid, syscall.SIGKILL)
}

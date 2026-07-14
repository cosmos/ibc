// Package proc supervises the lifecycle of local subprocess groups.
package proc

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"sync"
	"syscall"
	"time"
)

// Stop/reap state machine for a Setpgid child: owns cmd.Wait() and guards
// group signals against PID recycling. SignalAndWait is safe to call repeatedly;
// each caller has its own wait context.
type Handle struct {
	cmd  *exec.Cmd
	done chan struct{}

	mu      sync.Mutex
	waitErr error
	// Avoid re-signaling a dead process group while cmd.Wait publishes reaped.
	killSent bool
	reaped   bool
}

// Hooks describe output lifecycle work around cmd.Wait.
type Hooks struct {
	BeforeWait func()
	AfterWait  func()
}

func Reap(cmd *exec.Cmd, hooks Hooks) *Handle {
	h := &Handle{cmd: cmd, done: make(chan struct{})}
	go func() {
		if hooks.BeforeWait != nil {
			hooks.BeforeWait()
		}
		err := cmd.Wait()
		// Publish reaped under mu before closing done so SignalAndWait never signals a recycled pid.
		h.mu.Lock()
		h.waitErr = err
		h.reaped = true
		h.mu.Unlock()
		if hooks.AfterWait != nil {
			hooks.AfterWait()
		}
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

// Signals the group, escalating to SIGKILL after grace. It is safe to call
// repeatedly. Checks reaped under mu before signaling so a recycled pid is
// never killed.
func (h *Handle) SignalAndWait(ctx context.Context, sig syscall.Signal, grace time.Duration) error {
	h.mu.Lock()
	if h.reaped {
		h.mu.Unlock()
		return h.wait(ctx)
	}
	if h.killSent {
		h.mu.Unlock()
		return h.wait(ctx)
	}
	pgid := h.cmd.Process.Pid
	signalErr := signalProcessGroup(pgid, sig)
	if signalErr == nil && sig == syscall.SIGKILL {
		h.killSent = true
	}
	h.mu.Unlock()
	if signalErr != nil {
		return signalErr
	}

	if sig == syscall.SIGKILL || grace <= 0 {
		return h.wait(ctx)
	}
	timer := time.NewTimer(grace)
	defer timer.Stop()
	select {
	case <-h.done:
		return nil
	case <-timer.C:
		if err := h.escalateKill(pgid); err != nil {
			return err
		}
		return h.wait(ctx)
	case <-ctx.Done():
		return errors.Join(ctx.Err(), h.escalateKill(pgid))
	}
}

func (h *Handle) wait(ctx context.Context) error {
	select {
	case <-h.done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (h *Handle) escalateKill(pgid int) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.reaped || h.killSent {
		return nil
	}
	err := signalProcessGroup(pgid, syscall.SIGKILL)
	if err == nil {
		h.killSent = true
	}
	return err
}

func signalProcessGroup(pgid int, signal syscall.Signal) error {
	if err := syscall.Kill(-pgid, signal); err != nil && !errors.Is(err, syscall.ESRCH) {
		return fmt.Errorf("signal process group %d with %s: %w", pgid, signal, err)
	}
	return nil
}

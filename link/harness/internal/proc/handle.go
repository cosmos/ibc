package proc

import (
	"context"
	"os/exec"
	"sync"
	"syscall"
	"time"
)

// Handle is the stop/reap state machine for a long-lived child running in its own process group
// (Setpgid) — the single copy of the protocol both supervisors in the harness build on: Process in
// this package and ibclink's relayer daemon. It owns cmd.Wait(): Reap starts the reaper goroutine,
// and every stop path funnels through SignalAndWait. Post-exit actions belong on Done(); the one
// pre-Wait injection point (preWait) exists for supervisors that must drain output pipes before
// Wait may run. Anything beyond that belongs in the supervisor, not here.
type Handle struct {
	cmd  *exec.Cmd
	done chan struct{} // closed once the process is reaped

	mu      sync.Mutex
	waitErr error
	stopped bool
	reaped  bool // set by the reaper under mu once cmd.Wait() has returned (pid may then be recycled)
}

// Reap takes ownership of a started cmd's exit: it starts the reaper goroutine, which runs preWait
// first (the barrier for output-drain goroutines — cmd.Wait closes the pipes, so it must not run
// before the pipe reads finish; nil when the supervisor has none), waits the child, publishes
// `reaped` under the lock, then closes Done. The cmd must have been started with Setpgid so
// SignalAndWait can target the whole group.
func Reap(cmd *exec.Cmd, preWait func()) *Handle {
	h := &Handle{cmd: cmd, done: make(chan struct{})}
	go func() {
		if preWait != nil {
			preWait()
		}
		err := cmd.Wait()
		// Publish reaped under mu before close(done), so a SignalAndWait holding the same lock sees a
		// completed Wait() and never signals a pid the OS may have recycled.
		h.mu.Lock()
		h.waitErr = err
		h.reaped = true
		h.mu.Unlock()
		close(h.done)
	}()
	return h
}

// Done is closed once the process has been reaped. Post-exit cleanup (closing log sinks, ...) hangs
// off this channel.
func (h *Handle) Done() <-chan struct{} { return h.done }

// Err returns the process exit error once it has exited (nil while running or on a clean exit).
func (h *Handle) Err() error {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.waitErr
}

// SignalAndWait signals the process group with sig and waits for the process to be reaped,
// escalating to SIGKILL after grace (grace <= 0 signals once and waits). Idempotent: repeat and
// concurrent calls block until the process is gone. ctx bounds only the escalation wait: when the
// caller's deadline fires, the group is SIGKILLed so the process never leaks, and ctx.Err() is
// reported.
//
// The pid-recycling guard: if the process already exited on its own, its PID (== pgid under
// Setpgid) may have been recycled by the OS, and signaling -pgid would then hit an unrelated
// process group. `reaped` is checked under the same lock the reaper publishes it with, so a
// completed cmd.Wait() is always observed before any kill, and Wait() finishing in the gap between
// check and kill cannot slip through. escalateKill re-checks the same guard on the escalation
// paths.
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
	// Negative pid targets the whole group created via Setpgid.
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

// escalateKill sends SIGKILL to the process group, but only after re-checking `reaped` under the
// lock — the same guard the first signal uses (see SignalAndWait), closing the recycled-pid window
// on the grace-timer and ctx-cancel paths too.
func (h *Handle) escalateKill(pgid int) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.reaped {
		return
	}
	_ = syscall.Kill(-pgid, syscall.SIGKILL)
}

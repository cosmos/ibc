// Package proc supervises generic local subprocesses: start with combined-output capture, wait for
// a semantic readiness condition (never a fixed sleep), and stop with SIGTERM/SIGKILL escalation.
// It knows nothing about chains. The stop/reap state
// machine itself lives in Handle, which ibclink's relayer daemon shares.
package proc

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"syscall"
	"time"
)

// pollInterval is how often WaitReady re-probes. This is the poll cadence, not a readiness sleep:
// readiness is decided by the caller's semantic probe, never by elapsed time.
const pollInterval = 50 * time.Millisecond

// Spec configures a supervised subprocess.
type Spec struct {
	Name    string   // executable, resolved via PATH
	Args    []string // process arguments
	LogPath string   // file that receives combined stdout+stderr (empty: discard)
}

// Process is a started, supervised subprocess.
type Process struct {
	spec Spec
	logF *os.File
	h    *Handle // owns cmd.Wait() and the stop/reap state machine
}

// Start launches the subprocess with combined stdout+stderr captured to the log file (or discarded when
// no LogPath is set). It returns as soon as the process is spawned; use WaitReady for semantic readiness.
// The returned process is not bound to the caller context: it runs until Stop is called.
func Start(spec Spec) (*Process, error) {
	cmd := exec.CommandContext(context.Background(), spec.Name, spec.Args...)

	var logF *os.File
	out := io.Discard
	if spec.LogPath != "" {
		f, err := os.Create(spec.LogPath)
		if err != nil {
			return nil, fmt.Errorf("create log file %q: %w", spec.LogPath, err)
		}
		logF = f
		out = f
	}
	cmd.Stdout = out
	cmd.Stderr = out

	// Own process group so Stop can signal the whole subtree (the binary plus any children it spawns).
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	if err := cmd.Start(); err != nil {
		if logF != nil {
			_ = logF.Close()
		}
		return nil, fmt.Errorf("start %q: %w", spec.Name, err)
	}

	// No preWait barrier: output goes straight to the log file (no pipes to drain before Wait).
	return &Process{spec: spec, logF: logF, h: Reap(cmd, nil)}, nil
}

// WaitReady polls until probe returns nil or timeout elapses, failing fast if the process exits
// first. The probe is a semantic check (e.g. eth_blockNumber succeeds) — readiness is never a sleep.
// Each probe receives a context bounded by the overall deadline, so a single stalled probe (e.g. a
// connection that accepts TCP but never answers) cannot hang past timeout.
func (p *Process) WaitReady(ctx context.Context, probe func(context.Context) error, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	probeCtx, cancel := context.WithDeadline(ctx, deadline)
	defer cancel()

	var lastErr error
	// Both the exit error and the last probe error can legitimately be nil here (a process that exits 0
	// before the first probe runs), so wrap each only when present — a bare %w on a nil error renders the
	// useless %!w(<nil>).
	exited := func() error {
		werr := p.Err()
		switch {
		case werr != nil && lastErr != nil:
			return fmt.Errorf("%q exited before ready: %w (last probe: %w)", p.spec.Name, werr, lastErr)
		case werr != nil:
			return fmt.Errorf("%q exited before ready: %w", p.spec.Name, werr)
		case lastErr != nil:
			return fmt.Errorf("%q exited before ready (last probe: %w)", p.spec.Name, lastErr)
		default:
			return fmt.Errorf("%q exited before ready", p.spec.Name)
		}
	}

	for {
		select {
		case <-p.h.Done():
			return exited()
		default:
		}

		if lastErr = probe(probeCtx); lastErr == nil {
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("%q not ready after %s: %w", p.spec.Name, timeout, lastErr)
		}

		select {
		case <-time.After(pollInterval):
		case <-ctx.Done():
			return ctx.Err()
		case <-p.h.Done():
			return exited()
		}
	}
}

// Stop signals the process group with SIGTERM, escalating to SIGKILL after grace, then waits for
// the process to be reaped (the state machine, idempotency, and pid-recycling guard live in
// Handle.SignalAndWait). Concurrent/repeat calls block until the process is gone.
func (p *Process) Stop(grace time.Duration) error {
	// Background context: Stop has no caller deadline; the grace window alone bounds the escalation.
	err := p.h.SignalAndWait(context.Background(), syscall.SIGTERM, grace)
	if p.logF != nil {
		_ = p.logF.Close()
	}
	return err
}

// Err returns the process exit error once it has exited (nil while running or on a clean exit).
func (p *Process) Err() error { return p.h.Err() }

// LogPath is where combined output is captured (empty if no log file was configured).
func (p *Process) LogPath() string { return p.spec.LogPath }

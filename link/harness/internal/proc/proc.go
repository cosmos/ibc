// Package proc supervises local subprocesses: semantic readiness probes and SIGTERM/SIGKILL stop escalation.
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

const pollInterval = 50 * time.Millisecond

type Spec struct {
	Name    string
	Args    []string
	LogPath string
}

type Process struct {
	spec Spec
	logF *os.File
	h    *Handle
}

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

	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	if err := cmd.Start(); err != nil {
		if logF != nil {
			_ = logF.Close()
		}
		return nil, fmt.Errorf("start %q: %w", spec.Name, err)
	}

	return &Process{spec: spec, logF: logF, h: Reap(cmd, nil)}, nil
}

func (p *Process) WaitReady(ctx context.Context, probe func(context.Context) error, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	probeCtx, cancel := context.WithDeadline(ctx, deadline)
	defer cancel()

	var lastErr error
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

func (p *Process) Stop(grace time.Duration) error {
	err := p.h.SignalAndWait(context.Background(), syscall.SIGTERM, grace)
	if p.logF != nil {
		_ = p.logF.Close()
	}
	return err
}

func (p *Process) Err() error { return p.h.Err() }

func (p *Process) LogPath() string { return p.spec.LogPath }

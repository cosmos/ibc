// SPDX-License-Identifier: Apache-2.0

package ibccli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/netip"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"
)

const (
	proverBinEnv      = "IBC_TEST_PROVER_BIN"
	proverLogFilename = "prover.log"
	proverReadyWait   = 30 * time.Second
	proverReadyToken  = "listening"
)

// Prover is a running prover service.
type Prover struct {
	address string
	out     *logWriter
	h       *processHandle
}

// Address is where the prover is serving, known only once it has bound.
func (p *Prover) Address() string {
	if p == nil {
		return ""
	}

	return p.address
}

// StartProver runs the test prover on an ephemeral port, waiting for it to
// announce the one it got. Its output lands in prover.log beside configPath.
func StartProver(ctx context.Context, configPath string) (*Prover, error) {
	out, err := newLogWriter(filepath.Join(filepath.Dir(configPath), proverLogFilename))
	if err != nil {
		return nil, fmt.Errorf("ibc prover: create log: %w", err)
	}

	// Long-lived child: exec.Command (not CommandContext) + Setpgid so Stop can signal the whole group.
	cmd := exec.Command(resolvedProverBin(), "--config", configPath, "--listen", loopbackAnyPort)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	// The test prover has no health service; its first stdout line announces the bound address.
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		out.close()
		return nil, fmt.Errorf("ibc prover: stdout pipe: %w", err)
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		out.close()
		return nil, fmt.Errorf("ibc prover: stderr pipe: %w", err)
	}

	if err = cmd.Start(); err != nil {
		out.close()
		return nil, fmt.Errorf("ibc prover: start: %w", err)
	}

	readyCh := make(chan readinessResult, 1)
	var drained sync.WaitGroup
	drained.Add(2)
	go func() {
		defer drained.Done()
		watchProverAnnouncement(stdout, out, readyCh)
	}()
	go func() {
		defer drained.Done()
		_, _ = io.Copy(out, stderr)
	}()

	p := &Prover{out: out}
	p.h = reapProcess(cmd, processHooks{BeforeWait: drained.Wait, AfterWait: out.close})

	address, err := p.awaitAddress(ctx, readyCh)
	if err != nil {
		return nil, errors.Join(err, p.Kill())
	}
	p.address = address

	return p, nil
}

// Stop signals the prover's process group and escalates to SIGKILL after a
// grace period or when ctx expires. It is safe to call more than once.
func (p *Prover) Stop(ctx context.Context) error {
	if p == nil || p.h == nil {
		return nil
	}

	return p.h.signalAndWait(ctx, syscall.SIGTERM, stopGrace)
}

// Kill terminates the prover without allowing graceful shutdown.
func (p *Prover) Kill() error {
	if p == nil || p.h == nil {
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), killTimeout)
	defer cancel()

	return p.h.signalAndWait(ctx, syscall.SIGKILL, 0)
}

func (p *Prover) awaitAddress(ctx context.Context, readyCh <-chan readinessResult) (string, error) {
	startupCtx, cancel := context.WithTimeout(ctx, proverReadyWait)
	defer cancel()

	select {
	case got := <-readyCh:
		if got.err != nil {
			return "", fmt.Errorf("ibc prover: %w; logs: %s", got.err, p.logTail())
		}

		parsed, err := netip.ParseAddrPort(got.address)
		if err != nil || !parsed.Addr().IsLoopback() || parsed.Port() == 0 {
			return "", fmt.Errorf("ibc prover: announced invalid address %q", got.address)
		}

		return got.address, nil
	case <-p.h.doneCh():
		processErr := p.h.err()
		if processErr == nil {
			processErr = errors.New("process exited successfully")
		}

		return "", fmt.Errorf("ibc prover: exited before announcing an address: %w; logs: %s", processErr, p.logTail())
	case <-startupCtx.Done():
		return "", fmt.Errorf("ibc prover: did not announce an address: %w; logs: %s", startupCtx.Err(), p.logTail())
	}
}

func (p *Prover) logTail() string {
	return strings.TrimSpace(string(p.out.tailSnapshot()))
}

// watchProverAnnouncement reads the address announced on the prover's first
// stdout line and copies all of stdout into the log.
func watchProverAnnouncement(stdout io.Reader, logsSink io.Writer, ready chan<- readinessResult) {
	// The first line is the announcement or the prover broke its contract.
	firstLine := func(line string) bool {
		fields := strings.Fields(line)
		if len(fields) != 2 || fields[0] != proverReadyToken {
			ready <- readinessResult{err: fmt.Errorf("unexpected readiness line %q", strings.TrimSpace(line))}
		} else {
			ready <- readinessResult{address: fields[1]}
		}

		return true
	}

	if err := pipeLogs(stdout, logsSink, firstLine); err != nil {
		ready <- readinessResult{err: err}
	}
}

func resolvedProverBin() string {
	if v := os.Getenv(proverBinEnv); v != "" {
		return v
	}

	return defaultBinPath("testprover")
}

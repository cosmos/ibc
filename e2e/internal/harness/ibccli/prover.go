// SPDX-License-Identifier: Apache-2.0

package ibccli

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net/netip"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"time"
)

const (
	proverBinEnv     = "IBC_TEST_PROVER_BIN"
	proverReadyWait  = 30 * time.Second
	proverReadyToken = "listening"
	proverStopGrace  = 10 * time.Second
)

// Prover is a running prover service.
type Prover struct {
	handle  *processHandle
	address string
}

// Address is where the prover is serving, known only once it has bound.
func (p *Prover) Address() string {
	if p == nil {
		return ""
	}

	return p.address
}

// StartProver runs the test prover on an ephemeral port, waiting until ctx
// ends for it to announce the one it got.
func StartProver(ctx context.Context, configPath string) (*Prover, error) {
	cmd := exec.Command(resolvedProverBin(), "--config", configPath, "--listen", loopbackAnyPort)
	cmd.Stderr = os.Stderr
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("ibc prover: stdout pipe: %w", err)
	}

	if err = cmd.Start(); err != nil {
		return nil, fmt.Errorf("ibc prover: start: %w", err)
	}

	announced := make(chan announcement, 1)
	drained := make(chan struct{})
	go func() {
		defer close(drained)
		readAnnouncement(stdout, announced)
	}()
	prover := &Prover{handle: reapProcess(cmd, processHooks{BeforeWait: func() { <-drained }})}

	address, err := awaitProverAddress(ctx, announced)
	if err != nil {
		_ = prover.Stop(context.WithoutCancel(ctx))

		return nil, err
	}

	prover.address = address

	return prover, nil
}

// Stop asks the prover's process group to exit and escalates to SIGKILL after
// a grace period or when ctx ends. It is safe to call more than once.
func (p *Prover) Stop(ctx context.Context) error {
	if p == nil || p.handle == nil {
		return nil
	}

	return p.handle.signalAndWait(ctx, syscall.SIGTERM, proverStopGrace)
}

type announcement struct {
	address string
	err     error
}

// readAnnouncement sends the address the prover announces on its first stdout
// line, then discards the rest of stdout until it closes, so the prover never
// blocks writing to it.
func readAnnouncement(stdout io.Reader, announced chan<- announcement) {
	defer func() { _, _ = io.Copy(io.Discard, stdout) }()

	scanner := bufio.NewScanner(stdout)
	if !scanner.Scan() {
		announced <- announcement{err: fmt.Errorf("ibc prover: no readiness line: %w", scanner.Err())}

		return
	}

	fields := strings.Fields(scanner.Text())
	if len(fields) != 2 || fields[0] != proverReadyToken {
		announced <- announcement{err: fmt.Errorf("ibc prover: unexpected readiness line %q", scanner.Text())}

		return
	}

	announced <- announcement{address: fields[1]}
}

// awaitProverAddress waits for the prover's announced address.
func awaitProverAddress(ctx context.Context, announced <-chan announcement) (string, error) {
	select {
	case got := <-announced:
		if got.err != nil {
			return "", got.err
		}

		parsed, err := netip.ParseAddrPort(got.address)
		if err != nil || !parsed.Addr().IsLoopback() || parsed.Port() == 0 {
			return "", fmt.Errorf("ibc prover: announced invalid address %q", got.address)
		}

		return got.address, nil
	case <-time.After(proverReadyWait):
		return "", fmt.Errorf("ibc prover: did not announce an address within %s", proverReadyWait)
	case <-ctx.Done():
		return "", fmt.Errorf("ibc prover: startup canceled: %w", ctx.Err())
	}
}

func resolvedProverBin() string {
	if v := os.Getenv(proverBinEnv); v != "" {
		return v
	}

	return defaultBinPath("testprover")
}

// SPDX-License-Identifier: Apache-2.0

package ibclink

import (
	"bufio"
	"fmt"
	"io"
	"net/netip"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

const (
	proverBinEnv     = "IBC_TEST_PROVER_BIN"
	proverReadyWait  = 30 * time.Second
	proverReadyToken = "listening"
)

// Prover is a running prover service.
type Prover struct {
	cmd     *exec.Cmd
	address string
}

// Address is the endpoint the prover is serving on, known only once it has
// bound its port.
func (p *Prover) Address() string {
	if p == nil {
		return ""
	}

	return p.address
}

// StartProver runs the test prover against this driver's relayer config. It
// takes an ephemeral port and announces the one it got, so no caller has to
// reserve a port the prover might not win.
func (r *Driver) StartProver() (*Prover, error) {
	env, release, err := r.acquireProcessEnv()
	if err != nil {
		return nil, err
	}
	defer func() {
		if release != nil {
			release()
		}
	}()

	configPath := filepath.Join(r.configHome, r.configName)

	cmd := exec.Command(resolvedProverBin(), "--config", configPath, "--listen", loopbackAnyPort)
	cmd.Env = env
	cmd.Stderr = os.Stderr
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("ibc prover: stdout pipe: %w", err)
	}

	if err = cmd.Start(); err != nil {
		return nil, fmt.Errorf("ibc prover: start: %w", err)
	}

	prover := &Prover{cmd: cmd}

	address, err := awaitProverAddress(stdout)
	if err != nil {
		_ = prover.Stop()

		return nil, err
	}

	prover.address = address

	return prover, nil
}

// Stop signals the prover's process group and waits for it to exit.
func (p *Prover) Stop() error {
	if p == nil || p.cmd == nil || p.cmd.Process == nil {
		return nil
	}

	_ = syscall.Kill(-p.cmd.Process.Pid, syscall.SIGTERM)
	_ = p.cmd.Wait()

	return nil
}

// awaitProverAddress reads the address the prover announces on its first
// stdout line, rejecting anything that is not a bound loopback port.
func awaitProverAddress(stdout io.ReadCloser) (string, error) {
	type result struct {
		address string
		err     error
	}

	announced := make(chan result, 1)

	go func() {
		scanner := bufio.NewScanner(stdout)
		if !scanner.Scan() {
			announced <- result{err: fmt.Errorf("ibc prover: no readiness line: %w", scanner.Err())}

			return
		}

		fields := strings.Fields(scanner.Text())
		if len(fields) != 2 || fields[0] != proverReadyToken {
			announced <- result{err: fmt.Errorf("ibc prover: unexpected readiness line %q", scanner.Text())}

			return
		}

		announced <- result{address: fields[1]}
	}()

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
	}
}

func resolvedProverBin() string {
	if v := os.Getenv(proverBinEnv); v != "" {
		return v
	}

	return defaultBinPath("testprover")
}

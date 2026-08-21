// SPDX-License-Identifier: Apache-2.0

package ibclink

import (
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"time"
)

const (
	proverBinEnv     = "IBC_TEST_PROVER_BIN"
	proverStartWait  = 30 * time.Second
	proverStartPoll  = 100 * time.Millisecond
	proverDialWindow = time.Second
)

// Prover is a running prover service.
type Prover struct {
	cmd *exec.Cmd
}

// StartProver runs the test prover against this driver's relayer config,
// serving on address. The relayer reaches it over gRPC and holds no attestors
// or chain clients of its own.
func (r *Driver) StartProver(address string) (*Prover, error) {
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

	cmd := exec.Command(resolvedProverBin(), "--config", configPath, "--listen", address)
	cmd.Env = env
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("ibc prover: start: %w", err)
	}

	prover := &Prover{cmd: cmd}

	// The relayer must not ask for a proof before the service answers.
	if err := waitForListener(address); err != nil {
		_ = prover.Stop()

		return nil, err
	}

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

func waitForListener(address string) error {
	deadline := time.Now().Add(proverStartWait)

	for time.Now().Before(deadline) {
		conn, err := net.DialTimeout("tcp", address, proverDialWindow)
		if err == nil {
			_ = conn.Close()

			return nil
		}

		time.Sleep(proverStartPoll)
	}

	return fmt.Errorf("ibc prover: %s did not start listening", address)
}

func resolvedProverBin() string {
	if v := os.Getenv(proverBinEnv); v != "" {
		return v
	}

	return defaultBinPath("testprover")
}

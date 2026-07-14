package ibclink

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/cosmos/ibc/link/harness/ibclink/wire"
)

const (
	realBinEnv = "IBC_BIN"
	stubBinEnv = "IBC_STUB_BIN"
)

const maxStderrSnippet = 600

// One-shot invocations only; the long-lived daemon must not route through exec (buffers all output).
const defaultCommandTimeout = 120 * time.Second

type result struct {
	stdout []byte
	stderr string
	code   int
}

func (r *Driver) exec(ctx context.Context, bin, label string, args ...string) (*result, error) {
	ctx, cancel := context.WithTimeout(ctx, defaultCommandTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, bin, args...)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	runErr := cmd.Run()

	stderrText := stderr.String()
	res := &result{stdout: stdout.Bytes(), stderr: stderrText}

	var exitErr *exec.ExitError
	switch {
	case runErr == nil:
		res.code = wire.ExitOK
	case errors.As(runErr, &exitErr):
		res.code = exitErr.ExitCode()
	default:
		return res, fmt.Errorf("exec ibc %s (%s): %w", label, bin, runErr)
	}
	// A canceled ctx means CommandContext killed the child; don't mistake that signal exit for a coded one.
	if err := ctx.Err(); err != nil {
		return res, fmt.Errorf("ibc %s canceled: %w", label, err)
	}
	return res, nil
}

func classify(code int) error {
	switch code {
	case wire.ExitConfigInvalid:
		return ErrConfigInvalid
	case wire.ExitRPCUnreachable:
		return ErrRPCUnreachable
	case wire.ExitTestAppDeployFailure:
		return ErrTestAppDeployFailed
	case wire.ExitNotReady:
		return ErrNotReady
	default:
		return ErrInternal
	}
}

func snippet(s string) string {
	s = strings.TrimSpace(s)
	if len(s) > maxStderrSnippet {
		s = "..." + s[len(s)-maxStderrSnippet:]
	}
	return s
}

func ResolvedRealBin() string {
	if v := os.Getenv(realBinEnv); v != "" {
		return v
	}
	return defaultBinPath("ibc")
}

func ResolvedStubBin() string {
	if v := os.Getenv(stubBinEnv); v != "" {
		return v
	}
	return defaultBinPath("ibc-stub")
}

// Resolves link/bin relative to this source file, not the process cwd.
func defaultBinPath(name string) string {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		panic("ibclink: runtime.Caller(0) failed; cannot locate the link bin/ directory")
	}
	root := filepath.Dir(filepath.Dir(filepath.Dir(file)))
	return filepath.Join(root, "bin", name)
}

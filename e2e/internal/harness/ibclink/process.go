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
)

const binEnv = "IBC_BIN"

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
	var res *result
	err := r.withProcessEnv(func(processEnv processEnvironment) error {
		cmd := exec.CommandContext(ctx, bin, args...)
		cmd.Env = processEnv.variables

		var stdout, stderr bytes.Buffer
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr

		runErr := cmd.Run()
		res = &result{
			stdout: stdout.Bytes(),
			stderr: stderr.String(),
		}
		var exitErr *exec.ExitError
		switch {
		case runErr == nil:
			res.code = 0
		case errors.As(runErr, &exitErr):
			res.code = exitErr.ExitCode()
		default:
			return fmt.Errorf("exec ibc %s (%s): %w", label, bin, runErr)
		}
		// A canceled ctx means CommandContext killed the child; don't mistake that signal exit for a coded one.
		if err := ctx.Err(); err != nil {
			return fmt.Errorf("ibc %s canceled: %w", label, err)
		}
		return nil
	})
	return res, err
}

func snippet(s string) string {
	s = strings.TrimSpace(s)
	if len(s) > maxStderrSnippet {
		s = "..." + s[len(s)-maxStderrSnippet:]
	}
	return s
}

func ResolvedBin() string {
	if v := os.Getenv(binEnv); v != "" {
		return v
	}
	return defaultBinPath("ibc")
}

// Resolves link/bin relative to this source file, not the process cwd.
func defaultBinPath(name string) string {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		panic("ibclink: runtime.Caller(0) failed; cannot locate the link bin/ directory")
	}
	repoRoot := filepath.Dir(file)
	for range 4 {
		repoRoot = filepath.Dir(repoRoot)
	}
	return filepath.Join(repoRoot, "link", "bin", name)
}

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
	// realBinEnv overrides the real ibc link binary. IBC_BIN keeps its original meaning: the
	// real binary seam. During the progressive migration IBC_STUB_BIN overrides the temporary stub;
	// when the routing table is all-real the stub seam disappears and IBC_BIN is the only override again.
	realBinEnv = "IBC_BIN"
	// stubBinEnv overrides the temporary stub binary while commands are still routed to it.
	stubBinEnv = "IBC_STUB_BIN"
)

// maxStderrSnippet caps how much stderr an ExitError carries, so a failing command's error stays
// readable while still pointing at the cause. The full stream is in the log file.
const maxStderrSnippet = 600

// defaultCommandTimeout bounds a single one-shot SUT invocation (validate/migrate/deploy/app actions).
// It is generous — larger than the stub's own per-chain probe/deploy timeouts — and exists so a hung
// binary fails this command at a deadline rather than blocking until the whole test times out. It
// deliberately does not apply to the long-lived daemon path (`relayer run`), which needs its own
// unbounded, streaming exec.
const defaultCommandTimeout = 120 * time.Second

// result is one SUT invocation's captured output and exit status. A non-zero Code is data (part of
// the CLI contract), not a Go-level error — exec returns it without an error so the caller can map it.
type result struct {
	stdout []byte
	stderr string // captured stderr tail
	code   int
}

// exec runs the routed SUT with args, capturing stdout (the machine-readable JSON) and stderr (human logs)
// into buffers. label is a short human name for the command used in the log header and error messages.
//
// It returns a Go error only when the process could not be run at all (the binary is missing, or the
// context was canceled). A clean exit and any coded non-zero exit both return (result, nil); the
// caller inspects result.code and maps it to a typed error.
func (r *runner) exec(ctx context.Context, label string, args ...string) (*result, error) {
	// Bound this one-shot invocation so a hung stub (or real binary) fails this command at its own
	// deadline instead of blocking until the whole test times out. One-shot path only — the long-lived
	// daemon must not route through exec, which buffers all output and waits for exit.
	ctx, cancel := context.WithTimeout(ctx, defaultCommandTimeout)
	defer cancel()

	bin, err := r.binFor(label)
	if err != nil {
		return nil, err
	}
	cmd := exec.CommandContext(ctx, bin, args...)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	runErr := cmd.Run()

	stderrText := stderr.String()
	r.writeLog(label, args, stderrText)
	res := &result{stdout: stdout.Bytes(), stderr: stderrText}

	var exitErr *exec.ExitError
	switch {
	case runErr == nil:
		res.code = wire.ExitOK
	case errors.As(runErr, &exitErr):
		// A coded non-zero exit is the CLI contract talking; hand it back as data.
		res.code = exitErr.ExitCode()
	default:
		// Could not start the process (e.g. the routed binary was never built).
		return res, fmt.Errorf("exec ibc %s (%s): %w", label, bin, runErr)
	}
	// CommandContext kills the child on cancellation, surfacing as a signal exit; distinguish that
	// real setup failure from a stub-chosen exit code by checking the context explicitly.
	if err := ctx.Err(); err != nil {
		return res, fmt.Errorf("ibc %s canceled: %w", label, err)
	}
	return res, nil
}

// writeLog appends this invocation's header and its stderr to the run's log file for the diag bundle.
// Best-effort: a log-file problem must never fail the command, so a missing logPath or a failed open is
// silently skipped (the stderr still reaches the error snippet and the bundle).
func (r *runner) writeLog(label string, args []string, stderrText string) {
	if r.logPath == "" {
		return
	}
	f, err := os.OpenFile(r.logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return
	}
	defer f.Close() //nolint:errcheck
	_, _ = fmt.Fprintf(f, "\n=== ibc %s | args: %s ===\n%s", label, strings.Join(args, " "), stderrText)
}

// classify maps a stub exit code to its sentinel error so callers can errors.Is on the failure class
// without parsing stderr. An unknown non-zero code is treated as an internal fault by definition.
func classify(code int) error {
	switch code {
	case wire.ExitConfigInvalid:
		return ErrConfigInvalid
	case wire.ExitRPCUnreachable:
		return ErrRPCUnreachable
	case wire.ExitDeployFailure:
		return ErrDeployFailed
	case wire.ExitNotReady:
		return ErrNotReady
	default:
		return ErrInternal
	}
}

// snippet trims and tail-truncates stderr for inclusion in an ExitError.
func snippet(s string) string {
	s = strings.TrimSpace(s)
	if len(s) > maxStderrSnippet {
		s = "..." + s[len(s)-maxStderrSnippet:]
	}
	return s
}

// ResolvedRealBin reports the real ibc link binary: IBC_BIN if set, else bin/ibc.
func ResolvedRealBin() string {
	if v := os.Getenv(realBinEnv); v != "" {
		return v
	}
	return defaultBinPath("ibc")
}

// ResolvedStubBin reports the temporary stub binary: IBC_STUB_BIN if set, else bin/ibc-stub.
func ResolvedStubBin() string {
	if v := os.Getenv(stubBinEnv); v != "" {
		return v
	}
	return defaultBinPath("ibc-stub")
}

// defaultBinPath locates a link/bin binary relative to this source file (via runtime.Caller) rather
// than the process working directory, so it resolves the same whether a test runs from
// harness/ibclink, e2e/setup, or a module root.
func defaultBinPath(name string) string {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		// skip=0 cannot fail in practice; fail loudly rather than silently resolving a wrong
		// cwd-relative "bin/<name>" that would then produce a confusing "binary not built" error.
		panic("ibclink: runtime.Caller(0) failed; cannot locate the link bin/ directory")
	}
	// file = <link>/harness/ibclink/process.go -> three parents up is the link repo root, whose
	// bin/ holds both the real binary (bin/ibc, `make build`) and the stub (bin/ibc-stub).
	root := filepath.Dir(filepath.Dir(filepath.Dir(file)))
	return filepath.Join(root, "bin", name)
}

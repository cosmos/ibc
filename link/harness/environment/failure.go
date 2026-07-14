package environment

import (
	"errors"
	"fmt"
)

// StartError reports an atomic realization failure without formatting the
// underlying errors. The cause and joined cleanup error remain available for
// errors.Is/errors.As and explicit inspection.
type StartError struct {
	cause          error
	cleanup        error
	manifest       Manifest
	failures       []FailureRecord
	protocol       protocolReceiptSnapshot
	diagnosticsDir string
}

// FailureRecord locates a failed declaration without copying an adapter error
// that may contain endpoint credentials or other runtime-only values.
type FailureRecord struct {
	Kind ResourceKind `json:"kind"`
	ID   string       `json:"id"`
}

func newStartErrorWithProtocol(
	cause error,
	manifest Manifest,
	failures []FailureRecord,
	diagnosticsDir string,
	protocol protocolReceiptSnapshot,
	cleanupFailures ...error,
) *StartError {
	if cause == nil {
		panic("environment: StartError requires a cause")
	}
	return &StartError{
		cause:          cause,
		cleanup:        errors.Join(cleanupFailures...),
		manifest:       cloneManifest(manifest),
		failures:       append([]FailureRecord(nil), failures...),
		protocol:       cloneProtocolReceiptSnapshot(protocol),
		diagnosticsDir: diagnosticsDir,
	}
}

func (e *StartError) Error() string {
	message := "environment start failed"
	if len(e.failures) == 1 {
		failure := e.failures[0]
		message = fmt.Sprintf("environment start failed: start %s %q", failure.Kind, failure.ID)
	} else if len(e.failures) > 1 {
		message = fmt.Sprintf("environment start failed for %d resources", len(e.failures))
	}
	if e.cleanup != nil {
		message += "; cleanup also failed"
	}
	return message
}

func (e *StartError) Unwrap() error { return e.cause }

func (e *StartError) CleanupError() error { return e.cleanup }

func (e *StartError) Manifest() Manifest { return cloneManifest(e.manifest) }

func (e *StartError) Failures() []FailureRecord {
	return append([]FailureRecord(nil), e.failures...)
}

// IBCInstanceReceipts returns typed transaction evidence known before
// startup failed. The returned values and nested receipts are caller-owned.
func (e *StartError) IBCInstanceReceipts() []IBCInstanceReceipt {
	return cloneProtocolReceiptSnapshot(e.protocol).instances
}

// IBCConnectionReceipts returns each resolved or submitted Connection end
// known when startup failed. The returned values are caller-owned.
func (e *StartError) IBCConnectionReceipts() []IBCConnectionReceipt {
	return cloneProtocolReceiptSnapshot(e.protocol).connections
}

// DiagnosticsDir is non-empty only when failed cleanup forced Start to retain
// diagnostic artifacts. Runtime-private workspace files are never placed in
// the returned directory.
func (e *StartError) DiagnosticsDir() string { return e.diagnosticsDir }

func cloneManifest(m Manifest) Manifest {
	return Manifest{
		resources: m.Resources(),
		cleanup:   m.CleanupEffects(),
	}
}

type cleanupFailure struct {
	kind   ResourceKind
	id     string
	action CleanupAction
	cause  error
}

func (e *cleanupFailure) Error() string {
	return fmt.Sprintf("environment cleanup %s for %s %q failed", e.action, e.kind, e.id)
}

func (e *cleanupFailure) Unwrap() error { return e.cause }

type redactedCause struct {
	message string
	cause   error
}

func (e *redactedCause) Error() string { return e.message }
func (e *redactedCause) Unwrap() error { return e.cause }

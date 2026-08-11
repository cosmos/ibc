// SPDX-License-Identifier: Apache-2.0

package environment

import (
	"errors"
)

// StartError reports a realization failure and any cleanup failures.
type StartError struct {
	cause          error
	cleanup        error
	diagnosticsDir string
}

func newStartError(
	cause error,
	diagnosticsDir string,
	cleanupFailures ...error,
) *StartError {
	return &StartError{
		cause:          cause,
		cleanup:        errors.Join(cleanupFailures...),
		diagnosticsDir: diagnosticsDir,
	}
}

func (e *StartError) Error() string {
	message := "environment start failed: " + e.cause.Error()
	if e.cleanup != nil {
		message += "; cleanup also failed: " + e.cleanup.Error()
	}
	return message
}

func (e *StartError) Unwrap() error { return e.cause }

func (e *StartError) CleanupError() error { return e.cleanup }

// DiagnosticsDir is non-empty only when failed cleanup forced Start to retain
// diagnostic artifacts. Runtime-private workspace files are never placed in
// the returned directory.
func (e *StartError) DiagnosticsDir() string { return e.diagnosticsDir }

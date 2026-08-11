// SPDX-License-Identifier: Apache-2.0

package environment

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestStartErrorPreservesCauseCleanupAndDiagnostics(t *testing.T) {
	primary := errors.New("startup failed")
	cleanup := errors.New("cleanup failed")
	err := newStartError(primary, "/tmp/diagnostics", cleanup)

	require.ErrorIs(t, err, primary)
	require.ErrorIs(t, err.CleanupError(), cleanup)
	require.Equal(t, "/tmp/diagnostics", err.DiagnosticsDir())
	require.ErrorContains(t, err, "environment start failed: startup failed")
	require.ErrorContains(t, err, "cleanup failed")
}

func TestStartErrorWithoutCleanup(t *testing.T) {
	primary := errors.New("startup failed")
	err := newStartError(primary, "")

	require.ErrorIs(t, err, primary)
	require.NoError(t, err.CleanupError())
	require.Empty(t, err.DiagnosticsDir())
}

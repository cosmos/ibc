package ibclink

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/cosmos/ibc/link/harness/ibclink/wire"
)

func TestClassifyMapsSysexitsToSentinels(t *testing.T) {
	require.ErrorIs(t, classify(wire.ExitConfigInvalid), ErrConfigInvalid)
	require.ErrorIs(t, classify(wire.ExitRPCUnreachable), ErrRPCUnreachable)
	require.ErrorIs(t, classify(wire.ExitTestAppDeployFailure), ErrTestAppDeployFailed)
	require.ErrorIs(t, classify(wire.ExitNotReady), ErrNotReady)
	require.ErrorIs(t, classify(wire.ExitInternal), ErrInternal)
	require.ErrorIs(t, classify(1), ErrInternal, "unknown non-zero codes classify as internal")
}

func TestSnippetTailTruncates(t *testing.T) {
	require.Equal(t, "short", snippet("  short \n"))

	long := strings.Repeat("x", maxStderrSnippet) + "TAIL"
	got := snippet(long)
	require.True(t, strings.HasPrefix(got, "..."))
	require.True(t, strings.HasSuffix(got, "TAIL"), "the tail (where the cause usually is) survives")
	require.Len(t, got, maxStderrSnippet+3)
}

func TestExitErrorUnwrapsToClass(t *testing.T) {
	err := &ExitError{Code: wire.ExitConfigInvalid, Class: ErrConfigInvalid, Stderr: "bad yaml"}
	require.ErrorIs(t, err, ErrConfigInvalid)
	require.ErrorContains(t, err, "bad yaml")
}

package ibclink

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSnippetTailTruncates(t *testing.T) {
	require.Equal(t, "short", snippet("  short \n"))

	long := strings.Repeat("x", maxStderrSnippet) + "TAIL"
	got := snippet(long)
	require.True(t, strings.HasPrefix(got, "..."))
	require.True(t, strings.HasSuffix(got, "TAIL"), "the tail (where the cause usually is) survives")
	require.Len(t, got, maxStderrSnippet+3)
}

func TestExitErrorUnwrapsToClass(t *testing.T) {
	err := &ExitError{Code: 1, Class: ErrConfigInvalid, Stderr: "bad yaml"}
	require.ErrorIs(t, err, ErrConfigInvalid)
	require.ErrorContains(t, err, "bad yaml")
}

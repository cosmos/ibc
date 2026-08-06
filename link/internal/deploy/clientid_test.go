package deploy

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestClientIDs(t *testing.T) {
	require.True(t, ValidClientID("link-31337"))
	require.True(t, ValidClientID("my.client_1"))
	require.False(t, ValidClientID("abc"))                    // too short
	require.False(t, ValidClientID("client-0"))               // reserved prefix
	require.False(t, ValidClientID("channel-3"))              // reserved prefix
	require.False(t, ValidClientID("has space"))              // bad charset
	require.True(t, ValidClientID("abcd"))                    // min length boundary
	require.True(t, ValidClientID(strings.Repeat("a", 128)))  // max length boundary
	require.False(t, ValidClientID(strings.Repeat("a", 129))) // too long
}

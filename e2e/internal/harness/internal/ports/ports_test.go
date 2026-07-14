package ports

import (
	"fmt"
	"net"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestFreePortReturnsUsablePort(t *testing.T) {
	p, err := FreePort()
	require.NoError(t, err)
	require.Positive(t, p)

	l, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", p))
	require.NoError(t, err)
	require.NoError(t, l.Close())
}

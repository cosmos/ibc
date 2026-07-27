package anvil

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
)

func TestStopSucceedsWhenRemovalSucceedsAfterGracefulStopFailure(t *testing.T) {
	container := &stopTestContainer{stopErr: errors.New("graceful stop failed")}
	chain := &Chain{container: container}

	require.NoError(t, chain.Stop())
	require.True(t, chain.closed)
	require.True(t, chain.stopped)
	require.Nil(t, chain.container)
	require.Equal(t, 1, container.terminateCalls)
}

type stopTestContainer struct {
	testcontainers.Container
	stopErr        error
	terminateCalls int
}

func (*stopTestContainer) IsRunning() bool { return true }

func (c *stopTestContainer) Stop(context.Context, *time.Duration) error { return c.stopErr }

func (c *stopTestContainer) Terminate(context.Context, ...testcontainers.TerminateOption) error {
	c.terminateCalls++
	return nil
}

func (*stopTestContainer) Logs(context.Context) (io.ReadCloser, error) {
	return io.NopCloser(strings.NewReader("")), nil
}

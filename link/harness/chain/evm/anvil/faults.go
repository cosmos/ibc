package anvil

import (
	"context"
	"fmt"

	"github.com/cosmos/ibc/link/harness/chain"
	"github.com/cosmos/ibc/link/harness/internal/dockercli"
)

var _ chain.FaultInjector = (*Chain)(nil)

func (ac *Chain) StopNode(ctx context.Context) error {
	ac.mu.Lock()
	defer ac.mu.Unlock()
	if ac.stopped {
		return nil
	}
	// Keep the client open so calls observe the connection failure; StartNode replaces it.
	if err := stopContainer(ctx, ac.container); err != nil {
		return fmt.Errorf("stop anvil node %s: %w", ac.ID(), err)
	}
	ac.stopped = true
	return nil
}

func (ac *Chain) StartNode(ctx context.Context) error {
	ac.mu.Lock()
	defer ac.mu.Unlock()
	if !ac.stopped {
		return nil
	}
	if _, err := dockercli.Output(ctx, "start", ac.container); err != nil {
		return fmt.Errorf("restart anvil node %s: %w", ac.ID(), err)
	}
	ec, err := connectAnvil(ctx, ac.spec, ac.RPCURL())
	if err != nil {
		_ = stopContainer(context.Background(), ac.container)
		return fmt.Errorf("restart anvil node %s: %w", ac.ID(), anvilStartupError(ac.container, err))
	}
	ac.Close()
	ac.EVMClient = ec
	ac.closed = false
	ac.stopped = false
	return nil
}

package anvil

import (
	"context"
	"errors"
	"fmt"

	"github.com/cosmos/ibc/e2e/internal/harness/chain"
)

var _ chain.FaultInjector = (*Chain)(nil)

func (ac *Chain) StopNode(ctx context.Context) error {
	ac.mu.Lock()
	defer ac.mu.Unlock()
	if ac.stopped {
		return nil
	}
	// Keep the client open so calls observe the paused node; StartNode replaces it.
	if err := pauseContainer(ctx, ac.container); err != nil {
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
	if err := resumeContainer(ctx, ac.container); err != nil {
		return fmt.Errorf("restart anvil node %s: %w", ac.ID(), err)
	}
	ec, err := connectAnvil(ctx, ac.spec, ac.RPCURL())
	if err != nil {
		startupErr := fmt.Errorf("restart anvil node %s: %w", ac.ID(), anvilStartupError(ac.container, err))
		stopCtx, cancel := context.WithTimeout(context.Background(), anvilStopTimeout)
		stopErr := pauseContainer(stopCtx, ac.container)
		cancel()
		if stopErr != nil {
			// The container may still be running, so StopNode and terminal cleanup
			// must be allowed to retry the stop.
			ac.stopped = false
			return errors.Join(startupErr, fmt.Errorf("stop anvil after failed restart: %w", stopErr))
		}
		return startupErr
	}
	ac.replaceEVMClient(ec)
	ac.closed = false
	ac.stopped = false
	return nil
}

package anvil

import (
	"context"
	"fmt"

	"github.com/cosmos/ibc/link/harness/chain"
	"github.com/cosmos/ibc/link/harness/internal/dockercli"
)

// Chain implements FaultInjector: it can take its node down and bring it back so resilience tests
// can prove the daemon errors/retries while the RPC is gone and recovers once it returns.
var _ chain.FaultInjector = (*Chain)(nil)

// StopNode gracefully stops the anvil container. The SIGTERM-with-grace stop lets anvil dump its
// --state file, so a later StartNode restores the same chain state (deployed fixtures, balances, height).
// While stopped, every RPC call against this chain — the harness's and the black-box daemon's alike —
// fails with a connection error, which is exactly the "RPC unreachable" fault the resilience tests need.
// Idempotent: stopping an already-stopped node is a no-op.
func (ac *Chain) StopNode(ctx context.Context) error {
	ac.mu.Lock()
	defer ac.mu.Unlock()
	if ac.stopped {
		return nil
	}
	// Graceful stop so anvil dumps --state. The embedded client is left open on purpose: calls through it
	// now fail with connection-refused, which is the fault under test; StartNode swaps it.
	if err := stopContainer(ctx, ac.container); err != nil {
		return fmt.Errorf("stop anvil node %s: %w", ac.ID(), err)
	}
	ac.stopped = true
	return nil
}

// StartNode restarts the SAME container on the SAME port with the SAME --state file, so the node comes
// back with its pre-stop chain state and at the address the daemon already dialed — the daemon's HTTP
// transport simply re-dials and recovers, no reconfiguration. It re-dials a fresh harness client and swaps
// it in. Idempotent: starting a running node is a no-op.
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
	ac.Close() // the stale client's node is gone; drop it before swapping in the reconnected one
	ac.EVMClient = ec
	ac.closed = false
	ac.stopped = false
	return nil
}

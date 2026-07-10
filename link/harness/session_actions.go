package harness

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/cosmos/ibc/link/harness/chain"
	"github.com/cosmos/ibc/link/harness/chain/evm"
)

// StopRelayer stops the current relayer daemon. The instance stays on the Harness's ledger (a stopped
// daemon remains fully capturable), and RestartRelayer brings a fresh one up in its place.
func (r *Session) StopRelayer(ctx context.Context) error {
	d := r.h.relayer
	if d == nil {
		return errors.New("harness: no daemon to stop")
	}
	stopCtx, cancel := context.WithTimeout(ctx, daemonStopTimeout)
	defer cancel()
	return d.Stop(stopCtx)
}

// RestartRelayer restarts the current relayer daemon. Both instances end up on the Harness's ledger, so
// Shutdown captures each under its own LogLabel — no eager capture is needed here (the outgoing
// instance's Stop snapshots its final status, and its log is an in-memory snapshot).
func (r *Session) RestartRelayer(ctx context.Context) error {
	old := r.h.relayer
	if old == nil {
		return errors.New("harness: no daemon to restart")
	}
	nd, err := old.Restart(ctx)
	if err != nil {
		return err
	}
	r.h.trackDaemon(nd)
	return nil
}

// ChainHandle is a per-chain control surface bound to one chain id in the run's registry. It carries
// the block-production and fault-injection operations (and a typed EVM view) without repeating the chain
// id at every call. Capability negotiation is lazy: an operation a chain does not advertise — or a
// handle for an unknown chain id — fails on use, naming the missing capability, rather than at Chain().
type ChainHandle struct {
	chains *Chains
	id     string
}

// Chain returns a control handle for the chain with id. The handle is always non-nil; if id names no
// chain in the run, its operations report the "no chain" error when invoked.
func (r *Session) Chain(id string) *ChainHandle {
	return &ChainHandle{chains: r.h.chains, id: id}
}

// EVM returns the handle's chain's concrete EVM client, or an error if the chain is missing or not EVM.
func (h *ChainHandle) EVM() (*evm.EVMClient, error) {
	return h.chains.EVM(h.id)
}

// PauseMining holds new transactions in the chain mempool until Mine is called.
func (h *ChainHandle) PauseMining(ctx context.Context) error {
	ctrl, err := h.blockController()
	if err != nil {
		return err
	}
	return ctrl.PauseMining(ctx)
}

// ResumeMining restores normal automining on a paused chain.
func (h *ChainHandle) ResumeMining(ctx context.Context) error {
	ctrl, err := h.blockController()
	if err != nil {
		return err
	}
	return ctrl.ResumeMining(ctx)
}

// WithPausedMining pauses mining for fn and resumes it even if fn returns an error.
func (h *ChainHandle) WithPausedMining(ctx context.Context, fn func() error) (err error) {
	if pauseErr := h.PauseMining(ctx); pauseErr != nil {
		return pauseErr
	}
	defer func() {
		err = errors.Join(err, h.ResumeMining(ctx))
	}()
	return fn()
}

// Mine produces blocks on a chain whose mining is paused.
func (h *ChainHandle) Mine(ctx context.Context, blocks int) error {
	ctrl, err := h.blockController()
	if err != nil {
		return err
	}
	return ctrl.MineBlocks(ctx, blocks)
}

// AdvanceTime fast-forwards a chain clock and mines one block to make the new time visible.
func (h *ChainHandle) AdvanceTime(ctx context.Context, d time.Duration) error {
	ctrl, err := h.blockController()
	if err != nil {
		return err
	}
	return ctrl.AdvanceTime(ctx, d)
}

// StopNode takes a chain's local node down, preserving its state for StartNode.
func (h *ChainHandle) StopNode(ctx context.Context) error {
	fi, err := h.faultInjector()
	if err != nil {
		return err
	}
	return fi.StopNode(ctx)
}

// StartNode restarts a previously stopped local node on the same RPC address.
func (h *ChainHandle) StartNode(ctx context.Context) error {
	fi, err := h.faultInjector()
	if err != nil {
		return err
	}
	return fi.StartNode(ctx)
}

func (h *ChainHandle) blockController() (chain.BlockController, error) {
	return chainCapability[chain.BlockController](h.chains, h.id, "BlockController")
}

func (h *ChainHandle) faultInjector() (chain.FaultInjector, error) {
	return chainCapability[chain.FaultInjector](h.chains, h.id, "FaultInjector")
}

// ErrCapabilityMissing classifies a failed optional-capability negotiation: the action needed a chain
// capability (chain.As) the chain does not advertise. Wrapped errors name the chain and capability; tests
// assert the class with errors.Is instead of matching message text.
var ErrCapabilityMissing = errors.New("harness: missing chain capability")

func chainCapability[T any](chains *Chains, chainID, capability string) (T, error) {
	var zero T
	c, err := chains.Get(chainID)
	if err != nil {
		return zero, err
	}
	capValue, ok := chain.As[T](c)
	if !ok {
		return zero, fmt.Errorf("%w: chain %q does not support %s", ErrCapabilityMissing, chainID, capability)
	}
	return capValue, nil
}

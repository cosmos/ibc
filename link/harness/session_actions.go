package harness

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/cosmos/ibc/link/harness/chain"
	"github.com/cosmos/ibc/link/harness/chain/evm"
)

func (r *Session) StopRelayer(ctx context.Context) error {
	d := r.h.relayer
	if d == nil {
		return errors.New("harness: no daemon to stop")
	}
	stopCtx, cancel := context.WithTimeout(ctx, daemonStopTimeout)
	defer cancel()
	return d.Stop(stopCtx)
}

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

type ChainHandle struct {
	chains *Chains
	id     string
}

func (r *Session) Chain(id string) *ChainHandle {
	return &ChainHandle{chains: r.h.chains, id: id}
}

func (h *ChainHandle) EVM() (*evm.EVMClient, error) {
	return h.chains.EVM(h.id)
}

func (h *ChainHandle) PauseMining(ctx context.Context) error {
	ctrl, err := h.blockController()
	if err != nil {
		return err
	}
	return ctrl.PauseMining(ctx)
}

func (h *ChainHandle) ResumeMining(ctx context.Context) error {
	ctrl, err := h.blockController()
	if err != nil {
		return err
	}
	return ctrl.ResumeMining(ctx)
}

func (h *ChainHandle) WithPausedMining(ctx context.Context, fn func() error) (err error) {
	if pauseErr := h.PauseMining(ctx); pauseErr != nil {
		return pauseErr
	}
	defer func() {
		err = errors.Join(err, h.ResumeMining(ctx))
	}()
	return fn()
}

func (h *ChainHandle) Mine(ctx context.Context, blocks int) error {
	ctrl, err := h.blockController()
	if err != nil {
		return err
	}
	return ctrl.MineBlocks(ctx, blocks)
}

func (h *ChainHandle) AdvanceTime(ctx context.Context, d time.Duration) error {
	ctrl, err := h.blockController()
	if err != nil {
		return err
	}
	return ctrl.AdvanceTime(ctx, d)
}

func (h *ChainHandle) StopNode(ctx context.Context) error {
	fi, err := h.faultInjector()
	if err != nil {
		return err
	}
	return fi.StopNode(ctx)
}

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

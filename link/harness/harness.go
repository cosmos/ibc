package harness

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/cosmos/ibc/link/harness/chain"
	"github.com/cosmos/ibc/link/harness/chain/evm"
	"github.com/cosmos/ibc/link/harness/ibclink"
	"github.com/cosmos/ibc/link/harness/ibclink/wire"
	"github.com/cosmos/ibc/link/harness/internal/diag"
	"github.com/cosmos/ibc/link/harness/internal/onchain"
	"github.com/cosmos/ibc/link/harness/topology"

	evmreader "github.com/cosmos/ibc/link/harness/internal/chain/evm/reader"
	evmsubmit "github.com/cosmos/ibc/link/harness/internal/chain/evm/submit"
)

const (
	// Escalated to SIGKILL past this so a wedged daemon cannot hang teardown.
	daemonStopTimeout = 15 * time.Second
	// Unbounded Height here would hang teardown forever on a wedged node.
	diagRPCTimeout = 10 * time.Second
)

type Harness struct {
	topo   topology.Topology
	chains *Chains

	link    ibclink.Runner
	linkLog string

	procs   []ibclink.Daemon
	relayer ibclink.Daemon

	bundle *diag.Bundle
	ws     workspace
}

func (h *Harness) trackDaemon(d ibclink.Daemon) {
	h.procs = append(h.procs, d)
	h.relayer = d
}

func (h *Harness) Chains() *Chains { return h.chains }

func (h *Harness) IBCLink() ibclink.Runner { return h.link }

func (h *Harness) WorkDir() string { return h.ws.dir }

type workspace struct {
	dir         string
	runID       string
	artifactDir string
	keepOnClose bool
}

func newWorkspace(cfg StartConfig) (workspace, error) {
	dir, err := os.MkdirTemp("", "harness-")
	if err != nil {
		return workspace{}, fmt.Errorf("create work dir: %w", err)
	}
	return workspace{
		dir:         dir,
		runID:       fmt.Sprintf("%d-%d", os.Getpid(), time.Now().UnixNano()),
		artifactDir: cfg.ArtifactDir,
		keepOnClose: cfg.KeepOnClose,
	}, nil
}

func (ws workspace) remove() error {
	return os.RemoveAll(ws.dir)
}

func (h *Harness) buildReaders(dep *wire.Deployment) (map[string]onchain.Reader, error) {
	out := make(map[string]onchain.Reader, len(h.chains.list))
	for _, p := range h.chains.list {
		id := p.Chain.ID()
		cd, ok := dep.Chain(id)
		if !ok {
			return nil, fmt.Errorf(
				"harness: deployment has no chain %q; deploy/topology mismatch should have been caught earlier",
				id,
			)
		}
		budget := h.readerBudget(id)
		switch p.Chain.Family() {
		case chain.FamilyEVM:
			ec, ok := evm.FromChain(p.Chain)
			if !ok {
				return nil, fmt.Errorf("harness: chain %q reports EVM family but exposes no EVM client", id)
			}
			out[id] = evmreader.New(ec, id, cd, budget)
		default:
			return nil, fmt.Errorf("harness: chain %q family %q has no on-chain reader", id, p.Chain.Family())
		}
	}
	return out, nil
}

func (h *Harness) buildSubmitters(dep *wire.Deployment) (map[string]chain.AppSubmitter, error) {
	out := make(map[string]chain.AppSubmitter, len(h.chains.list))
	for _, p := range h.chains.list {
		id := p.Chain.ID()
		cd, ok := dep.Chain(id)
		if !ok {
			return nil, fmt.Errorf(
				"harness: deployment has no chain %q; deploy/topology mismatch should have been caught earlier",
				id,
			)
		}
		switch p.Chain.Family() {
		case chain.FamilyEVM:
			ec, ok := evm.FromChain(p.Chain)
			if !ok {
				return nil, fmt.Errorf("harness: chain %q reports EVM family but exposes no EVM client", id)
			}
			s, err := evmsubmit.New(ec, cd)
			if err != nil {
				return nil, err
			}
			out[id] = s
		default:
			return nil, fmt.Errorf("harness: chain %q family %q has no app submitter", id, p.Chain.Family())
		}
	}
	return out, nil
}

func (h *Harness) readerBudget(chainID string) onchain.Budget {
	p := h.chains.Profile(chainID)
	return onchain.Budget{
		Completion: p.CompletionBudget,
		Poll:       p.PollInterval,
		StatusRow:  p.StatusRowBudget(),
	}
}

func (h *Harness) Shutdown(ctx context.Context, failed bool) error {
	var errs []error
	if h.relayer != nil && !h.ws.keepOnClose {
		stopCtx, cancel := context.WithTimeout(ctx, daemonStopTimeout)
		if err := h.relayer.Stop(stopCtx); err != nil {
			errs = append(errs, fmt.Errorf("stop relayer daemon: %w", err))
		}
		cancel()
	}
	h.CaptureDiagnostics(ctx)
	if failed && h.ws.artifactDir != "" {
		_, _ = h.WriteArtifacts(h.ws.artifactDir)
	}
	errs = append(errs, h.closeWorld())
	return errors.Join(errs...)
}

func (h *Harness) CaptureDiagnostics(ctx context.Context) {
	for _, p := range h.chains.list {
		for id, log := range p.CollectLogs(ctx) {
			if log != "" {
				h.bundle.AddChainLog(id, log)
			}
		}
		hctx, cancel := context.WithTimeout(ctx, diagRPCTimeout)
		height, err := p.Chain.Height(hctx)
		cancel()
		if err == nil {
			h.bundle.SetHeight(p.Chain.ID(), height)
		}
	}
	if h.linkLog != "" {
		if data, err := os.ReadFile(h.linkLog); err == nil && len(data) > 0 {
			h.bundle.AddSUTLog("ibc-link", string(data))
		}
	}
	for _, d := range h.procs {
		captureDaemonLog(h.bundle, d, d.LogLabel())
		captureDaemonStatus(h.bundle, d, d.LogLabel())
	}
}

func (h *Harness) WriteArtifacts(dir string) (string, error) {
	if dir == "" {
		return "", errors.New("harness: WriteArtifacts called with empty dir")
	}
	if !filepath.IsAbs(dir) {
		wd, err := os.Getwd()
		if err != nil {
			wd = "."
		}
		dir = filepath.Join(wd, dir)
	}
	outDir := filepath.Join(dir, h.ws.runID)
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return "", err
	}
	path := filepath.Join(outDir, "diagnostics.txt")
	return path, os.WriteFile(path, []byte(h.bundle.String()), 0o644)
}

func (h *Harness) closeWorld() error {
	if h.ws.keepOnClose {
		return nil
	}
	errs := []error{h.chains.stopAll()}
	if err := h.ws.remove(); err != nil {
		errs = append(errs, fmt.Errorf("remove workdir %s: %w", h.ws.dir, err))
	}
	return errors.Join(errs...)
}

func captureDaemonStatus(b *diag.Bundle, d ibclink.Daemon, key string) {
	ctx, cancel := context.WithTimeout(context.Background(), diagRPCTimeout)
	defer cancel()
	status, err := d.Status(ctx, wire.StatusQuery{})
	if err != nil {
		var ok bool
		if status, ok = d.FinalStatus(); !ok {
			return
		}
	}
	data, err := json.MarshalIndent(status, "", "  ")
	if err != nil {
		return
	}
	b.AddStatus(key, string(data))
}

func captureDaemonLog(b *diag.Bundle, d ibclink.Daemon, key string) {
	if data, err := io.ReadAll(d.Logs()); err == nil && len(data) > 0 {
		b.AddSUTLog(key, string(data))
	}
}

func topologySummary(t topology.Topology) string {
	var b strings.Builder
	fmt.Fprintf(&b, "name=%s db=sqlite(runtime path)\n", t.Name)
	for _, spec := range t.Chains {
		c := spec.Chain
		if spec.Provision.Mode == topology.ProvisionExternal {
			fmt.Fprintf(
				&b,
				"chain %s: external rpc=%s chainID=%d (logs unavailable)\n",
				c.ID,
				spec.Provision.RPCURL,
				c.ChainID,
			)
			continue
		}
		fmt.Fprintf(&b, "chain %s: launcher=%s chainID=%d\n", c.ID, spec.Provision.Launcher, c.ChainID)
	}
	for _, r := range t.Config.Relayer.Routes {
		fmt.Fprintf(&b, "route %s: %s -> %s (%s)\n", r.ID, r.Source, r.Destination, r.Type)
	}
	return b.String()
}

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
	"github.com/cosmos/ibc/link/harness/diag"
	"github.com/cosmos/ibc/link/harness/ibclink"
	"github.com/cosmos/ibc/link/harness/ibclink/wire"
	"github.com/cosmos/ibc/link/harness/onchain"
	"github.com/cosmos/ibc/link/harness/topology"

	evmreader "github.com/cosmos/ibc/link/harness/chain/evm/reader"
	evmsubmit "github.com/cosmos/ibc/link/harness/chain/evm/submit"
)

const (
	// daemonStopTimeout bounds the graceful daemon stop during teardown so a wedged daemon cannot hang
	// cleanup (it is escalated to SIGKILL past this).
	daemonStopTimeout = 15 * time.Second

	// diagRPCTimeout bounds each diagnostic RPC. A wedged node is the usual reason a test failed, so an
	// unbounded Height call here would hang teardown forever — diagnostics must never block cleanup.
	diagRPCTimeout = 10 * time.Second
)

// Harness is one run's world: the provisioned chains, the ibc link driver bound to this run's compiled
// config, and the run's workspace + diagnostics bundle. It is the result of Start; everything in it is
// passive until StartRelayer brings the relayer up and wraps it in a Session.
//
// The Harness also owns every long-lived SUT process started during the run (the procs ledger), so
// Shutdown is the single teardown-and-capture path: a daemon that came up and failed later — even during
// StartRelayer itself — still has its log and final status folded into the failure bundle, on every exit
// path, without per-path capture calls.
type Harness struct {
	topo   topology.Topology
	chains *Chains

	link    ibclink.Runner // ibc link driver over this run's compiled config
	linkLog string         // file the driver appends every one-shot invocation's stderr to

	// procs is the ledger of every long-lived SUT process started during this run, in start order —
	// today relayer daemon instances (a restart appends a second entry), tomorrow attestors alike.
	// Entries are never removed: a stopped instance stays capturable (Logs is an in-memory snapshot,
	// FinalStatus a pre-stop one). relayer is the current instance, nil until StartRelayer. Both are
	// single-threaded by test convention, like the rest of the harness surface.
	procs   []ibclink.Daemon
	relayer ibclink.Daemon

	bundle *diag.Bundle
	ws     workspace
}

// trackDaemon ledgers a newly started daemon and makes it the current relayer instance.
func (h *Harness) trackDaemon(d ibclink.Daemon) {
	h.procs = append(h.procs, d)
	h.relayer = d
}

// Chains returns the run's chain registry (per-chain handles, profiles, capability lookups).
func (h *Harness) Chains() *Chains { return h.chains }

// IBCLink returns the ibc link driver bound to this run's compiled config.
func (h *Harness) IBCLink() ibclink.Runner { return h.link }

// WorkDir returns the run's workspace directory (compiled config, relayer DB, chain and daemon
// files). Under KeepOnClose it is the kept world's root, so callers can surface where to inspect.
func (h *Harness) WorkDir() string { return h.ws.dir }

// workspace is the run's on-disk scratch area and artifact policy: where chain logs, the compiled
// config, and the relayer DB live, plus what happens to all of it at teardown. The run id doubles as
// the Docker label value provisioning tags every container with. The harness always creates the dir
// and always owns its removal: teardown deletes it unless keepOnClose preserves the world.
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

// remove deletes the workdir. Callers gate on keepOnClose.
func (ws workspace) remove() error {
	return os.RemoveAll(ws.dir)
}

// buildReaders constructs one onchain.Reader per chain (keyed by Chain.ID), dispatching on the chain's
// family. Each reader is bound to its chain client and that chain's deployed fixtures, so the correlator and
// app asserters read every on-chain effect through the family-agnostic Reader seam.
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

// buildSubmitters constructs one chain.AppSubmitter per chain (keyed by Chain.ID), dispatching on the
// chain's family — the write-side twin of buildReaders, bound the same way: each submitter gets its
// chain's client, its deployed fixtures, and its timing budget at construction (see chain.AppSubmitter
// for why this seam exists). A submitter borrows its chain's client (closed by the chain's Stop), so
// there is nothing to tear down here.
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

// readerBudget translates a chain's resolved timing profile into the onchain package's Budget — the
// completion/poll bounds effect waits use and the status-row bound the destination reader lends to
// StatusCrossCheck. This is the one place topology.TimingProfile crosses into onchain, keeping onchain free
// of the topology package.
func (h *Harness) readerBudget(chainID string) onchain.Budget {
	p := h.chains.Profile(chainID)
	return onchain.Budget{
		Completion: p.CompletionBudget,
		Poll:       p.PollInterval,
		StatusRow:  p.StatusRowBudget(),
	}
}

// Shutdown is the run's single teardown path: it stops the current relayer daemon first (completing its
// log and snapshotting its final status), captures diagnostics from the chains and every ledgered SUT
// process, writes artifacts when failed is true, then tears the world down. Under KeepOnClose everything
// stays up for inspection — chains, workdir, and the daemon alike (its pid file names the process to
// sweep) — but diagnostics are still captured and artifacts still written.
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

// CaptureDiagnostics pulls chain logs, current heights, the ibc-link one-shot command log, and every
// ledgered SUT process's log + status into the bundle. It is best-effort: each diagnostic RPC is
// independently time-bounded, and a stopped process contributes its captured log and pre-stop status
// snapshot instead of a live query.
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

// WriteArtifacts writes the bundle to a subdirectory of dir named by the run id.
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

// closeWorld stops the chains (reverse start order) and cleans up the temp workdir if the harness
// created it. It is Shutdown's final step, not a public teardown: Shutdown is the only teardown path, so
// there is no way to tear the world down without the daemon stop and diagnostics capture that precede this.
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

// captureDaemonStatus folds the daemon's status JSON into the bundle: the live status API when the
// process still answers, else the final snapshot its Stop captured (see Daemon.FinalStatus). A stopped
// endpoint refuses the connection immediately, so the live attempt costs nothing on a dead instance.
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

// captureDaemonLog reads the daemon's captured stderr into the bundle.
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
			// Name the gap rather than silently omit it: the harness does not own this node, so it never
			// captures its logs. The summary makes that explicit in any failure bundle.
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

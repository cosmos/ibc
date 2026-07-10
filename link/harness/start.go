package harness

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/cosmos/ibc/link/harness/diag"
	"github.com/cosmos/ibc/link/harness/ibclink"
	"github.com/cosmos/ibc/link/harness/ibclink/wire"
	"github.com/cosmos/ibc/link/harness/onchain"
	"github.com/cosmos/ibc/link/harness/provision"
	"github.com/cosmos/ibc/link/harness/topology"
)

// Start brings up the harness's world for one run, in four explicit steps: the workspace (dirs + run
// id), the provisioned chains, the compiled ibc link config, and the driver over that config. Each step
// takes the previous step's output — nothing is assembled half-built. Nothing of ibc link runs yet
// (constructing the driver execs nothing), so a Harness is passive until StartRelayer.
func Start(ctx context.Context, cfg StartConfig) (*Harness, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	topo := cfg.Topology
	if err := topo.Validate(); err != nil {
		return nil, fmt.Errorf("harness.Start: %w", err)
	}

	ws, err := newWorkspace(cfg)
	if err != nil {
		return nil, fmt.Errorf("harness.Start: %w", err)
	}

	// provision.Start returns whatever chains did start even on failure; keep them up under KeepOnClose
	// (the whole point is inspecting a failed run), tear them down otherwise.
	provisioned, err := provision.Start(ctx, topo, ws.dir, ws.runID)
	chains := newChains(provisioned)
	abort := func() {
		if cfg.KeepOnClose {
			return
		}
		_ = chains.stopAll()
		_ = ws.remove()
	}
	if err != nil {
		abort()
		return nil, fmt.Errorf("harness.Start: %w", err)
	}

	link, err := newIBCLink(topo, chains, ws.dir)
	if err != nil {
		abort()
		return nil, fmt.Errorf("harness.Start: %w", err)
	}

	return &Harness{
		topo:    topo,
		chains:  chains,
		link:    link.runner,
		linkLog: link.logPath,
		bundle:  newBundle(topo, link.config),
		ws:      ws,
	}, nil
}

// StartRelayer brings the relayer up against this Harness and returns the Session: it applies the DB
// migration, validates the compiled config live, deploys the per-chain fixtures, starts the relayer
// daemon (blocking on its semantic readiness), and binds one on-chain Reader per chain from the
// reported deployment. Callers that register Harness teardown before calling this (e.g. e2etest) get
// diagnostics even when a step here fails: the daemon is ledgered the moment it starts, so Shutdown
// captures its log and status on every exit path below.
func (h *Harness) StartRelayer(ctx context.Context) (*Session, error) {
	if h.relayer != nil {
		// One live session per harness: a second relayer instance is a topology-level decision (route
		// assignment, shared vs separate DB), never an implicit second StartRelayer.
		return nil, errors.New("harness.StartRelayer: a relayer session is already live for this harness")
	}
	link := h.link

	if err := link.MigrateUp(ctx); err != nil {
		return nil, fmt.Errorf("harness.StartRelayer: migrate: %w", err)
	}

	vr, err := link.ValidateConfig(ctx, true)
	if err != nil {
		return nil, fmt.Errorf("harness.StartRelayer: validate config: %w", err)
	}
	if !vr.Valid || len(vr.Errors) > 0 {
		return nil, fmt.Errorf("harness.StartRelayer: invalid config: %v", vr.Errors)
	}

	dep, err := link.Deploy(ctx)
	if err != nil {
		return nil, fmt.Errorf("harness.StartRelayer: deploy: %w", err)
	}
	if deploymentErr := assertDeploymentMatchesTopology(dep, h.topo); deploymentErr != nil {
		return nil, deploymentErr
	}
	h.bundle.SetDeployment(dep)

	daemon, err := link.Run(ctx)
	if err != nil {
		return nil, fmt.Errorf("harness.StartRelayer: relayer run: %w", err)
	}
	// Ledger the daemon before any assertion can fail, so a bring-up that dies past this point still has
	// the daemon's log and final status captured by Shutdown. The failure paths stop the process but keep
	// the handle: Logs() is an in-memory snapshot and Stop takes the status snapshot, so a stopped
	// instance stays fully capturable.
	h.trackDaemon(daemon)
	// The daemon already validated its readiness line before link.Run returned (startDaemon -> parseReadiness),
	// so only the topology-level connectivity assertion is left to make here.
	ready := daemon.Ready()
	if connectedErr := assertChainsConnected(ready, h.topo); connectedErr != nil {
		_ = daemon.Stop(context.Background())
		return nil, connectedErr
	}

	// Build one on-chain Reader per chain from the reported deployment, so every subsequent independent
	// read (correlator, app asserters, prepare baselines) goes through the family-agnostic Reader seam.
	readers, err := h.buildReaders(dep)
	if err != nil {
		_ = daemon.Stop(context.Background())
		return nil, fmt.Errorf("harness.StartRelayer: build readers: %w", err)
	}

	// Build one app submitter per chain — the write-side twin of the readers (see chain.AppSubmitter).
	submitters, err := h.buildSubmitters(dep)
	if err != nil {
		_ = daemon.Stop(context.Background())
		return nil, fmt.Errorf("harness.StartRelayer: build submitters: %w", err)
	}

	return &Session{
		h:          h,
		deployment: dep,
		readers:    readers,
		packets:    onchain.NewPackets(readers),
		ift:        onchain.NewIFT(readers),
		gmp:        onchain.NewGMP(readers),
		submitters: submitters,
	}, nil
}

// linkDriver is newIBCLink's result: the driver, the log file its one-shot invocations append stderr
// to, and the compiled config bytes for the diagnostics bundle.
type linkDriver struct {
	runner  ibclink.Runner
	logPath string
	config  []byte
}

// newIBCLink compiles the topology into the ibc link config against the chains' runtime endpoints,
// writes it into the workspace, and constructs the driver over it. Everything here is passive: the
// ibc link binary is not exec'd until a command runs.
func newIBCLink(topo topology.Topology, chains *Chains, dir string) (linkDriver, error) {
	rb := topology.RuntimeBindings{
		ChainRPC: make(map[string]string, len(topo.Chains)),
		DBPath:   filepath.Join(dir, "relayer.db"),
	}
	// Every chain reports its host-reachable RPC — managed nodes their dynamic 127.0.0.1 port, external
	// nodes their static URL — so the compiled config points ibc link at the same endpoints the
	// harness drives. Compile ignores the binding for an external chain and keeps its Provision.RPCURL.
	for _, p := range chains.list {
		id := p.Chain.ID()
		rb.ChainRPC[id] = p.Chain.RPCURL()
	}

	compiled, err := topology.Compile(topo, rb)
	if err != nil {
		return linkDriver{}, fmt.Errorf("compile config: %w", err)
	}
	data, err := compiled.Marshal()
	if err != nil {
		return linkDriver{}, fmt.Errorf("marshal config: %w", err)
	}
	configPath := filepath.Join(dir, "ibc-link.config.yaml")
	if writeErr := os.WriteFile(configPath, data, 0o600); writeErr != nil {
		return linkDriver{}, fmt.Errorf("write config %q: %w", configPath, writeErr)
	}

	logPath := filepath.Join(dir, "ibc-link.log")
	runner, err := ibclink.NewRunner(ibclink.Options{
		ConfigPath: configPath,
		LogPath:    logPath,
	})
	if err != nil {
		return linkDriver{}, fmt.Errorf("build ibc relayer runner: %w", err)
	}
	return linkDriver{runner: runner, logPath: logPath, config: data}, nil
}

// newBundle seeds the diagnostics bundle with everything known statically at startup: the topology
// summary, tool/image versions (probed via provision for the providers in use), and the compiled config
// ibc link consumes.
func newBundle(topo topology.Topology, configYAML []byte) *diag.Bundle {
	b := diag.NewBundle()
	b.SetTopology(topologySummary(topo))
	b.SetVersion("go-ethereum", diag.GoEthereumVersion())
	b.SetVersion("ibc-link-bin", ibclink.ResolvedRealBin())
	b.SetVersion("ibc-link-stub-bin", ibclink.ResolvedStubBin())
	for tool, version := range provision.Versions(topo) {
		b.SetVersion(tool, version)
	}
	b.SetConfig(string(configYAML))
	return b
}

func assertDeploymentMatchesTopology(dep *wire.Deployment, topo topology.Topology) error {
	for _, spec := range topo.Chains {
		if _, ok := dep.Chain(spec.Chain.ID); !ok {
			return fmt.Errorf("harness.StartRelayer: deploy must report chain %s", spec.Chain.ID)
		}
	}
	return nil
}

func assertChainsConnected(ready wire.Readiness, topo topology.Topology) error {
	connected := make(map[string]bool, len(ready.ChainsConnected))
	for _, id := range ready.ChainsConnected {
		connected[id] = true
	}
	for _, spec := range topo.Chains {
		if !connected[spec.Chain.ID] {
			return fmt.Errorf("harness.StartRelayer: relayer did not connect to chain %s", spec.Chain.ID)
		}
	}
	return nil
}

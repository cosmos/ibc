package harness

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/cosmos/ibc/link/harness/ibclink"
	"github.com/cosmos/ibc/link/harness/ibclink/wire"
	"github.com/cosmos/ibc/link/harness/internal/diag"
	"github.com/cosmos/ibc/link/harness/internal/onchain"
	"github.com/cosmos/ibc/link/harness/internal/provision"
	"github.com/cosmos/ibc/link/harness/topology"
)

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

func (h *Harness) StartRelayer(ctx context.Context) (*Session, error) {
	if h.relayer != nil {
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
	// Ledger before post-start assertions so Shutdown captures log/status even when bring-up fails past here.
	h.trackDaemon(daemon)
	ready := daemon.Ready()
	if connectedErr := assertChainsConnected(ready, h.topo); connectedErr != nil {
		_ = daemon.Stop(context.Background())
		return nil, connectedErr
	}

	readers, err := h.buildReaders(dep)
	if err != nil {
		_ = daemon.Stop(context.Background())
		return nil, fmt.Errorf("harness.StartRelayer: build readers: %w", err)
	}

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

type linkDriver struct {
	runner  ibclink.Runner
	logPath string
	config  []byte
}

func newIBCLink(topo topology.Topology, chains *Chains, dir string) (linkDriver, error) {
	rb := topology.RuntimeBindings{
		ChainRPC: make(map[string]string, len(topo.Chains)),
		DBPath:   filepath.Join(dir, "relayer.db"),
	}
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

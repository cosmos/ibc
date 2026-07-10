// Package provision brings a topology's chains up. It is the single component that maps a chain's
// harness-only Provision (managed vs external, which launcher) onto the concrete chain packages, so the
// root harness package composes phases without importing any node launcher. The launch decision keys off
// Provision.Mode/Launcher only — never wire.Chain.Provider, which is relayer-facing metadata the harness
// must not read.
//
// Provisioning returns plain data (Provisioned records); it never touches the chain registry or the
// diagnostics bundle — both belong to the caller.
package provision

import (
	"context"
	"fmt"
	"path/filepath"

	"golang.org/x/sync/errgroup"

	"github.com/cosmos/ibc/link/harness/chain"
	"github.com/cosmos/ibc/link/harness/chain/evm/anvil"
	"github.com/cosmos/ibc/link/harness/chain/evm/besu"
	"github.com/cosmos/ibc/link/harness/chain/evm/external"
	"github.com/cosmos/ibc/link/harness/ibclink/wire"
	"github.com/cosmos/ibc/link/harness/topology"
)

// Provisioned is one running (or dialed) chain: the handle the rest of the harness drives, the resolved
// timing profile every wait observing it budgets from, and the lifecycle hooks the owner calls at
// teardown.
type Provisioned struct {
	Chain chain.Chain

	// Profile is the chain's resolved timing profile (ChainSpec.ResolvedTiming) — the same value the
	// launcher derived the node's block cadence from, so cadence and wait budgets cannot drift.
	Profile topology.TimingProfile

	// Stop tears the chain down (or, for an external chain, only closes the harness's client).
	Stop func() error

	// CollectLogs snapshots the chain's logs keyed by chain id, or nil when the harness owns none (an
	// external node's logs are a named gap, surfaced in the diagnostics topology summary).
	CollectLogs func(context.Context) map[string]string
}

// Start brings up the topology's chains concurrently and returns one Provisioned record per started
// chain, in declaration order. On failure it returns the records of the chains that DID start alongside the error —
// teardown policy (stop them, or keep them up for inspection) belongs to the caller, so provisioning
// never second-guesses it. ctx governs startup only; a started chain lives until its Stop is called.
func Start(ctx context.Context, topo topology.Topology, workDir, runID string) ([]Provisioned, error) {
	seen := make(map[string]bool, len(topo.Chains))
	for _, spec := range topo.Chains {
		if seen[spec.Chain.ID] {
			return nil, fmt.Errorf("provision: duplicate chain id %q in topology %s", spec.Chain.ID, topo.Name)
		}
		seen[spec.Chain.ID] = true
	}

	// Chains launch concurrently: launchers allocate per-chain ports/containers/networks/workdirs and
	// share no mutable state, so a 2-chain bring-up costs ~one launch budget instead of two. Each
	// goroutine writes its own result slot (indexed by declaration order), so the returned records stay
	// deterministic without a lock. On any failure the errgroup ctx cancels the rest, but we still wait
	// for every goroutine and return the records of the chains that DID come up — so the caller can tear
	// them down and nothing leaks when chain 2 of N fails.
	results := make([]Provisioned, len(topo.Chains))
	ok := make([]bool, len(topo.Chains))
	g, gctx := errgroup.WithContext(ctx)
	for i, spec := range topo.Chains {
		g.Go(func() error {
			p, err := startChain(gctx, spec, workDir, runID)
			if err != nil {
				return fmt.Errorf("provision: start chain %s: %w", spec.Chain.ID, err)
			}
			results[i] = p
			ok[i] = true
			return nil
		})
	}
	err := g.Wait()

	started := make([]Provisioned, 0, len(topo.Chains))
	for i := range results {
		if ok[i] {
			started = append(started, results[i])
		}
	}
	return started, err
}

// startChain brings up one chain per its Provision. The resolved timing profile is passed down so a
// managed node mines at the profile's BlockInterval.
func startChain(ctx context.Context, spec topology.ChainSpec, workDir, runID string) (Provisioned, error) {
	c := spec.Chain
	prof := spec.ResolvedTiming()
	switch spec.Provision.Mode {
	case topology.ProvisionManaged:
		return startManagedChain(ctx, c, spec.Provision.Launcher, prof, workDir, runID)
	case topology.ProvisionExternal:
		ext, err := external.Connect(ctx, external.Spec{
			ID:      c.ID,
			ChainID: c.ChainID,
			RPCURL:  spec.Provision.RPCURL,
		})
		if err != nil {
			return Provisioned{}, err
		}
		return Provisioned{
			Chain:   ext,
			Profile: prof,
			// The harness does not own this node: stop only closes the client, it never signals the node,
			// and there are no logs to collect. The "external (logs unavailable)" gap is surfaced in the
			// diagnostics topology summary rather than silently omitted.
			Stop:        func() error { ext.Close(); return nil },
			CollectLogs: func(context.Context) map[string]string { return nil },
		}, nil
	default:
		return Provisioned{}, fmt.Errorf("chain %s has unknown provision mode %q", c.ID, spec.Provision.Mode)
	}
}

// startManagedChain launches a node the harness owns, dispatching on the configured managed launcher. The
// resolved profile's BlockInterval sets a managed anvil's --block-time (0 = instant/automine, the default)
// and a managed besu's QBFT block period, so the node seals at exactly the cadence the harness's waits are
// budgeted for.
func startManagedChain(
	ctx context.Context,
	c wire.Chain,
	launcher string,
	prof topology.TimingProfile,
	workDir, runID string,
) (Provisioned, error) {
	switch launcher {
	case topology.LauncherAnvil:
		ac, err := anvil.Start(ctx, anvil.Spec{
			ID:        c.ID,
			ChainID:   c.ChainID,
			LogPath:   filepath.Join(workDir, "anvil-"+c.ID+".log"),
			RunID:     runID,
			BlockTime: prof.BlockInterval,
		})
		if err != nil {
			return Provisioned{}, err
		}
		return Provisioned{
			Chain:       ac,
			Profile:     prof,
			Stop:        ac.Stop,
			CollectLogs: ac.CollectLogs,
		}, nil
	case topology.LauncherBesu:
		bc, err := besu.StartQBFT(ctx, besu.Spec{
			ID:            c.ID,
			ChainID:       c.ChainID,
			WorkDir:       filepath.Join(workDir, "besu-"+c.ID),
			RunID:         runID,
			BlockPeriod:   prof.BlockInterval,
			RelayerKeyHex: c.EVMSignerKey,
		})
		if err != nil {
			return Provisioned{}, err
		}
		return Provisioned{
			Chain:       bc,
			Profile:     prof,
			Stop:        bc.Stop,
			CollectLogs: bc.CollectLogs,
		}, nil
	default:
		return Provisioned{}, fmt.Errorf("managed launcher %q is unsupported", launcher)
	}
}

// Versions reports the versions/images of the launchers the topology's managed chains use, keyed the way
// the diagnostics bundle names tools. Only launchers in use are probed.
func Versions(topo topology.Topology) map[string]string {
	out := map[string]string{}
	if usesLauncher(topo, topology.LauncherAnvil) {
		out["anvil-image"] = anvil.DockerImage()
	}
	if usesLauncher(topo, topology.LauncherBesu) {
		out["besu-image"] = besu.DockerImage()
	}
	return out
}

func usesLauncher(t topology.Topology, launcher string) bool {
	for _, spec := range t.Chains {
		if spec.Provision.Mode == topology.ProvisionManaged && spec.Provision.Launcher == launcher {
			return true
		}
	}
	return false
}

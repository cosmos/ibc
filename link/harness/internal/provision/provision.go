// Package provision brings chains up; keys off Provision.Mode/Launcher, not wire.Chain.Provider.
package provision

import (
	"context"
	"fmt"
	"path/filepath"

	"golang.org/x/sync/errgroup"

	"github.com/cosmos/ibc/link/harness/chain"
	"github.com/cosmos/ibc/link/harness/chain/evm/anvil"
	"github.com/cosmos/ibc/link/harness/ibclink/wire"
	"github.com/cosmos/ibc/link/harness/internal/chain/evm/besu"
	"github.com/cosmos/ibc/link/harness/internal/chain/evm/external"
	"github.com/cosmos/ibc/link/harness/topology"
)

type Provisioned struct {
	Chain       chain.Chain
	Profile     topology.TimingProfile
	Stop        func() error
	CollectLogs func(context.Context) map[string]string
}

// On failure returns started chains alongside the error; caller owns teardown.
func Start(ctx context.Context, topo topology.Topology, workDir, runID string) ([]Provisioned, error) {
	seen := make(map[string]bool, len(topo.Chains))
	for _, spec := range topo.Chains {
		if seen[spec.Chain.ID] {
			return nil, fmt.Errorf("provision: duplicate chain id %q in topology %s", spec.Chain.ID, topo.Name)
		}
		seen[spec.Chain.ID] = true
	}

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
			Chain:       ext,
			Profile:     prof,
			Stop:        func() error { ext.Close(); return nil },
			CollectLogs: func(context.Context) map[string]string { return nil },
		}, nil
	default:
		return Provisioned{}, fmt.Errorf("chain %s has unknown provision mode %q", c.ID, spec.Provision.Mode)
	}
}

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

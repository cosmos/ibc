package topology

import (
	"fmt"
	"slices"

	"github.com/cosmos/ibc/link/harness/ibclink/wire"
)

type Topology struct {
	Name   string
	Chains []ChainSpec
	Config wire.ConfigYAML
}

// It returns a copy, disables every route when none are named, and panics for unknown route IDs.
func (t Topology) WithManualRelay(routeIDs ...string) Topology {
	for _, id := range routeIDs {
		if _, ok := t.Config.Route(id); !ok {
			panic(fmt.Sprintf("topology: WithManualRelay: no route %q in topology %s", id, t.Name))
		}
	}
	routes := cloneRoutes(t.Config.Relayer.Routes)
	for i := range routes {
		if len(routeIDs) == 0 || slices.Contains(routeIDs, routes[i].ID) {
			routes[i].AutoRelay = &wire.AutoRelay{Enabled: false}
		}
	}
	t.Config.Relayer.Routes = routes
	t.Name += "+manual"
	return t
}

func cloneRoutes(in []wire.Route) []wire.Route {
	return append([]wire.Route(nil), in...)
}

func (t Topology) Validate() error {
	families := make(map[string]string, len(t.Chains))
	evmIDs := make(map[uint64]string, len(t.Chains))
	for _, spec := range t.Chains {
		id := spec.Chain.ID
		if id == "" {
			return fmt.Errorf("topology %s: chain with empty id", t.Name)
		}
		if _, dup := families[id]; dup {
			return fmt.Errorf("topology %s: duplicate chain id %s", t.Name, id)
		}
		if spec.Chain.Type != wire.ChainTypeEVM {
			return fmt.Errorf("topology %s: chain %s has unknown family %q", t.Name, id, spec.Chain.Type)
		}
		families[id] = spec.Chain.Type
		if spec.Chain.ChainID != 0 {
			if other, dup := evmIDs[spec.Chain.ChainID]; dup {
				return fmt.Errorf(
					"topology %s: chains %s and %s share EVM chain id %d",
					t.Name, other, id, spec.Chain.ChainID,
				)
			}
			evmIDs[spec.Chain.ChainID] = id
		}
	}

	routeIDs := make(map[string]bool, len(t.Config.Relayer.Routes))
	for _, r := range t.Config.Relayer.Routes {
		if r.ID == "" {
			return fmt.Errorf("topology %s: route with empty id", t.Name)
		}
		if routeIDs[r.ID] {
			return fmt.Errorf("topology %s: duplicate route id %s", t.Name, r.ID)
		}
		routeIDs[r.ID] = true
		src, ok := families[r.Source]
		if !ok {
			return fmt.Errorf("topology %s: route %s: unknown source chain %q", t.Name, r.ID, r.Source)
		}
		dst, ok := families[r.Destination]
		if !ok {
			return fmt.Errorf("topology %s: route %s: unknown destination chain %q", t.Name, r.ID, r.Destination)
		}
		want, ok := wire.RouteTypeFor(src, dst)
		if !ok {
			return fmt.Errorf("topology %s: route %s: no route type relays %s -> %s", t.Name, r.ID, src, dst)
		}
		if r.Type != want {
			return fmt.Errorf(
				"topology %s: route %s: type %q does not match endpoint families (%s -> %s needs %q)",
				t.Name, r.ID, r.Type, src, dst, want,
			)
		}
	}
	return nil
}

type ChainSpec struct {
	Chain wire.Chain

	Provision Provision

	Timing TimingProfile
}

// BlockInterval remains zero when authored as instant mining; other zero fields use launcher defaults.
func (s ChainSpec) ResolvedTiming() TimingProfile {
	def := DefaultTiming(s.Provision.Launcher)
	if s.Timing == (TimingProfile{}) {
		return def
	}
	t := s.Timing
	if t.CompletionBudget == 0 {
		t.CompletionBudget = def.CompletionBudget
	}
	if t.SettleWindow == 0 {
		t.SettleWindow = def.SettleWindow
	}
	if t.PollInterval == 0 {
		t.PollInterval = def.PollInterval
	}
	return t
}

type Provision struct {
	Mode ProvisionMode

	Launcher string

	// RPCURL may contain ${NAME} references for the relayer to resolve.
	RPCURL string
}

type ProvisionMode string

const (
	ProvisionManaged ProvisionMode = "managed"

	ProvisionExternal ProvisionMode = "external"
)

const (
	LauncherAnvil = "anvil"
	LauncherBesu  = "besu"
)

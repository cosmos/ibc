package topology

import (
	"fmt"
	"slices"

	"github.com/cosmos/ibc/link/harness/ibclink/wire"
)

// Topology is a named harness environment: a Shape's chain slots and routes bound to concrete
// infrastructure (providers, launchers, timing, chain ids). Lane-bound topologies come from a Lane
// applied to a Shape and are named <shape>-<lane>; ad-hoc topologies build ChainSpecs directly and
// pick chain ids per the reserved-ranges note in lane.go. Both go through Validate at harness.Start.
//
// Chains is the authoritative chain list: each entry pairs the relayer-facing wire.Chain with the
// harness-only Provision describing how that chain comes up. Config carries the rest of the ibc link wire
// config — the routes — with the runtime-only fields (each chain's RPC URL and the sqlite path) left
// blank; Compile projects the Chains section from Chains and fills those fields. Config.Chains is authored
// empty and populated at compile, so Chains is the single source of truth for chain entries.
type Topology struct {
	Name   string
	Chains []ChainSpec
	Config wire.ConfigYAML
}

// WithManualRelay returns a copy of the topology with auto-relay disabled on the named routes (every
// route when none are named), so their packets move only on an explicit /relay request. The routes
// slice is copied, never aliased — a caller may bind a topology once and derive from it more than
// once, so a derivation must never mutate its input. An unknown route id panics: a typo silently
// keeping auto-relay on would only surface far away, as a stable-pending wait failing mid-test.
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

// cloneRoutes copies a route slice so a derivation or projection never mutates its input.
func cloneRoutes(in []wire.Route) []wire.Route {
	return append([]wire.Route(nil), in...)
}

// Validate checks the topology's internal consistency: unique chain and route ids, known chain
// families, unique nonzero numeric EVM chain ids, route endpoints that reference declared chains,
// and route types that match their endpoint families (via wire.RouteTypeFor, the same derivation
// lanes bind with). harness.Start calls it, so ad-hoc composed topologies get the same checks as
// lane-bound ones. It cannot see across coexisting worlds — chain-id ranges stay a convention
// (see lane.go).
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

// ChainSpec is one chain in a topology: the relayer-facing wire entry plus how the harness provisions it.
type ChainSpec struct {
	// Chain is the relayer-facing chain entry (ID, Type, Provider, ChainID). Its RPC is left blank and
	// filled by Compile — from the runtime bindings for a managed chain, or from Provision.RPCURL for an
	// external one.
	Chain wire.Chain

	// Provision is the harness-only description of how this chain comes up.
	Provision Provision

	// Timing is how fast this chain observably makes progress — the source of every packet-wait and reader
	// budget for waits observing this chain. A zero value means "use the provider default" (resolved by
	// ResolvedTiming); a lane that mines on a fixed interval (anvil-interval) sets it explicitly
	// so both the managed-node launch and the harness's waits derive from one block cadence.
	Timing TimingProfile
}

// ResolvedTiming is the spec's effective timing profile: the explicitly-set Timing, or the provision
// provider's default when Timing is zero. A partially-populated custom Timing has each zero budget field
// filled from the provider default, so a profile that sets only some fields can never reach a degenerate
// budget (a zero PollInterval panics time.NewTicker and divides by zero in SettleObservations; a zero
// CompletionBudget makes every wait deadline immediately). BlockInterval is left as authored: zero is a
// legitimate value (instant/on-demand mining) and never feeds a ticker or a divisor. Provisioning resolves
// it once per chain at launch (a managed node mines at the resolved BlockInterval) and the same resolved
// profile bounds every wait observing that chain, so cadence and budgets derive from one value.
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

// Provision separates "how the harness provisions a chain" from "what the relayer is told" — the launch
// decision keys off Provision alone, never off wire.Chain.Provider (relayer-facing metadata mirroring the
// eng-doc schema).
type Provision struct {
	Mode ProvisionMode

	// Launcher selects the managed launcher. Meaningful only when Mode == ProvisionManaged.
	Launcher string

	// RPCURL is a static, host-reachable RPC for an already-running chain the harness does not own.
	// Meaningful only when Mode == ProvisionExternal. It may carry ${NAME} references, preserved verbatim
	// into the ibc link config for ibc link to resolve at load; the POC's own harness dial assumes a
	// concrete URL.
	RPCURL string
}

// ProvisionMode selects whether the harness launches a chain or connects to an external one.
type ProvisionMode string

const (
	// ProvisionManaged means the harness launches and owns the node's lifecycle (Anvil/Besu containers
	// or in-process fixtures) — it can pause mining, stop/restart the node, and collect
	// its logs.
	ProvisionManaged ProvisionMode = "managed"

	// ProvisionExternal means the harness connects to an already-running, host-reachable chain it does not own.
	// It never starts, stops, or collects logs from the node, and advertises no block-control or
	// fault-injection capability for it.
	ProvisionExternal ProvisionMode = "external"
)

const (
	// LauncherAnvil is the managed Anvil launcher key startChain reads from Provision.Launcher.
	LauncherAnvil = "anvil"
	// LauncherBesu is the managed Besu launcher key startChain reads from Provision.Launcher.
	LauncherBesu = "besu"
)

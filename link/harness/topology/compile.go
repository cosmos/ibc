package topology

import (
	"errors"
	"fmt"

	"github.com/cosmos/ibc/link/harness/ibclink/wire"
)

// RuntimeBindings carries the values a Topology cannot know statically — they are resolved at run
// time once the harness has actually brought the environment up. A topology is declarative and
// portable; these bindings are the per-run, machine-specific glue.
type RuntimeBindings struct {
	// ChainRPC maps a wire.Chain.ID to its host-reachable RPC URL for the MANAGED chains. For a local
	// Anvil subprocess this is 127.0.0.1:<dynamic port>, only known after the node started — hence
	// runtime, not static. External chains are not bound here: their RPC is the static Provision.RPCURL.
	ChainRPC map[string]string

	// DBPath is the resolved relayer DB location (e.g. a file under the run's temp dir for sqlite).
	DBPath string
}

// Sentinel errors so callers (and tests) can match the failure class without string-matching.
var (
	// ErrMissingRPC is a managed chain with no runtime RPC binding (the harness failed to record the
	// launched node's URL).
	ErrMissingRPC = errors.New("no runtime RPC binding for chain")
	// ErrExternalRPCRequired is an external chain whose Provision carries no static RPC URL. Unlike a
	// managed chain, nothing runtime-fills it, so an empty URL is a topology authoring error.
	ErrExternalRPCRequired = errors.New("external chain requires a static RPC URL")
)

// Compile projects the topology's chains into an ibc link wire config and fills the runtime-only fields.
// Managed chains take their RPC from the runtime bindings (ErrMissingRPC if absent); external chains take
// their RPC from Provision.RPCURL verbatim (ErrExternalRPCRequired if empty) — an external URL is never
// overwritten by a binding. The DB is set to the runtime sqlite path.
func Compile(t Topology, rb RuntimeBindings) (wire.ConfigYAML, error) {
	cfg := cloneConfig(t.Config)

	cfg.Chains = make([]wire.Chain, len(t.Chains))
	for i, spec := range t.Chains {
		c := spec.Chain // value copy; only RPC is filled below
		switch spec.Provision.Mode {
		case ProvisionManaged:
			url, ok := rb.ChainRPC[c.ID]
			if !ok || url == "" {
				return wire.ConfigYAML{}, fmt.Errorf("compile topology %q: %w: %s", t.Name, ErrMissingRPC, c.ID)
			}
			c.RPC = wire.RPC{URL: url}
		case ProvisionExternal:
			if spec.Provision.RPCURL == "" {
				return wire.ConfigYAML{}, fmt.Errorf(
					"compile topology %q: %w: %s",
					t.Name,
					ErrExternalRPCRequired,
					c.ID,
				)
			}
			c.RPC = wire.RPC{URL: spec.Provision.RPCURL}
		default:
			return wire.ConfigYAML{}, fmt.Errorf(
				"compile topology %q: chain %s has unknown provision mode %q",
				t.Name,
				c.ID,
				spec.Provision.Mode,
			)
		}
		cfg.Chains[i] = c
	}

	cfg.DB = wire.DB{Type: wire.DBTypeSQLite, URL: rb.DBPath}

	return cfg, nil
}

// cloneConfig deep-copies the parts of the ibc link config a Topology authors (the routes). The Chains
// section is rebuilt from Topology.Chains by Compile, so it is not copied here.
func cloneConfig(in wire.ConfigYAML) wire.ConfigYAML {
	out := in
	out.Chains = nil
	out.Relayer.Routes = cloneRoutes(in.Relayer.Routes)
	return out
}

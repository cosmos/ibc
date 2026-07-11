package topology

import (
	"errors"
	"fmt"

	"github.com/cosmos/ibc/link/harness/ibclink/wire"
)

type RuntimeBindings struct {
	ChainRPC map[string]string
	DBPath   string
}

var (
	ErrMissingRPC          = errors.New("no runtime RPC binding for chain")
	ErrExternalRPCRequired = errors.New("external chain requires a static RPC URL")
)

func Compile(t Topology, rb RuntimeBindings) (wire.ConfigYAML, error) {
	cfg := cloneConfig(t.Config)

	cfg.Chains = make([]wire.Chain, len(t.Chains))
	for i, spec := range t.Chains {
		c := spec.Chain
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

func cloneConfig(in wire.ConfigYAML) wire.ConfigYAML {
	out := in
	out.Chains = nil
	out.Relayer.Routes = cloneRoutes(in.Relayer.Routes)
	return out
}

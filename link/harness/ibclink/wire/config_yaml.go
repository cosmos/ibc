package wire

import (
	"fmt"

	"gopkg.in/yaml.v3"
)

// ConfigYAML is the IBC Link configuration document.
type ConfigYAML struct {
	Chains  []Chain `yaml:"chains"`
	DB      DB      `yaml:"db"`
	Relayer Relayer `yaml:"relayer"`
}

// Chain is one chain entry. Chains carry only chain-level connectivity; routes reference them by id.
type Chain struct {
	ID       string `yaml:"id"`                // logical id, e.g. "chain-a"
	Type     string `yaml:"type"`              // chain family: "evm"
	Provider string `yaml:"provider"`          // mirrors the eng-doc chains.provider field (e.g. "anvil")
	ChainID  uint64 `yaml:"chainId,omitempty"` // EVM numeric chain id
	// EVMSignerKey is the hex secp256k1 key the relayer signs this EVM chain's destination-side effects
	// (and source-side refunds) with. The harness genesis-funds it on managed Besu chains; Anvil
	// pre-funds it as a dev account. Test-only, carried in the clear.
	EVMSignerKey string `yaml:"evmSignerKey,omitempty"`
	RPC          RPC    `yaml:"rpc"`
}

const (
	// ChainTypeEVM is the config token for EVM-family chains.
	ChainTypeEVM = "evm"
	// ProviderAnvil is the config token for a managed Anvil EVM chain.
	ProviderAnvil = "anvil"
	// ProviderBesu is the config token for a managed Besu EVM chain.
	ProviderBesu = "besu"

	// RouteEVMToEVMAttested is an EVM-source -> EVM-destination attested route.
	RouteEVMToEVMAttested = "evmToEvmAttested"
)

// RouteTypeFor derives the route type a source->destination chain-type pair requires — the single
// source keeping route types and chain families correlated: harness topology construction and
// validation bind through it, and the SUT's config validation checks declared types against it.
func RouteTypeFor(srcType, dstType string) (routeType string, ok bool) {
	if srcType == ChainTypeEVM && dstType == ChainTypeEVM {
		return RouteEVMToEVMAttested, true
	}
	return "", false
}

// RPC is a chain's RPC endpoint. The URL is a plain string so the on-disk contract can carry an
// explicit ${NAME} reference for the SUT to resolve at load time.
type RPC struct {
	URL string `yaml:"url"`
}

// Relayer holds the relayer-process configuration: persistence plus the directed routes it relays.
type Relayer struct {
	Routes []Route `yaml:"routes"`
}

// DB is the ibc link persistence config. The POC implements sqlite, but keeps the current relayer-shaped
// type/url fields so config generation exercises the same surface.
type DB struct {
	Type string `yaml:"type"`
	URL  string `yaml:"url"` // runtime-resolved sqlite file path for the POC
}

// DBTypeSQLite is the config token for sqlite persistence (the only backend the POC implements).
const DBTypeSQLite = "sqlite"

// ValidateDB applies the config-level constraints for the POC sqlite store.
func ValidateDB(db DB) error {
	switch db.Type {
	case DBTypeSQLite:
	case "":
		return fmt.Errorf("db type is empty")
	default:
		return fmt.Errorf("unsupported db type %q (POC supports %q)", db.Type, DBTypeSQLite)
	}
	switch db.URL {
	case "":
		return fmt.Errorf("db url is empty")
	case ":memory:":
		return fmt.Errorf(`in-memory sqlite (":memory:") is unsupported; pass a file path`)
	default:
		return nil
	}
}

// AutoRelay is a route's auto-relay setting. Absent means enabled, matching ADR-011's default; manual
// routes opt out explicitly and are relayed only after a daemon /relay request.
type AutoRelay struct {
	Enabled bool `yaml:"enabled"`
}

// Route is one directed source→destination path the relayer services.
type Route struct {
	ID          string     `yaml:"id"`
	Source      string     `yaml:"source"`      // Chain.ID
	Destination string     `yaml:"destination"` // Chain.ID
	Type        string     `yaml:"type"`        // route shape, e.g. "evmToEvmAttested"
	AutoRelay   *AutoRelay `yaml:"autoRelay,omitempty"`
}

// AutoRelayEnabled reports whether the relayer may relay this route's packets unprompted.
func (r Route) AutoRelayEnabled() bool {
	return r.AutoRelay == nil || r.AutoRelay.Enabled
}

// Route looks up a route by id.
func (c *ConfigYAML) Route(id string) (Route, bool) {
	for _, r := range c.Relayer.Routes {
		if r.ID == id {
			return r, true
		}
	}
	return Route{}, false
}

// Chain looks up a chain by id.
func (c *ConfigYAML) Chain(id string) (Chain, bool) {
	for _, ch := range c.Chains {
		if ch.ID == id {
			return ch, true
		}
	}
	return Chain{}, false
}

// Marshal renders the config as YAML. It serializes the config as-is, including ${NAME} references,
// so the on-disk artifact never contains their expanded values.
func (c *ConfigYAML) Marshal() ([]byte, error) {
	out, err := yaml.Marshal(c)
	if err != nil {
		return nil, fmt.Errorf("marshal config yaml: %w", err)
	}
	return out, nil
}

// Unmarshal parses a YAML config document without resolving environment references.
func Unmarshal(data []byte) (*ConfigYAML, error) {
	var c ConfigYAML
	if err := yaml.Unmarshal(data, &c); err != nil {
		return nil, fmt.Errorf("unmarshal config yaml: %w", err)
	}
	return &c, nil
}

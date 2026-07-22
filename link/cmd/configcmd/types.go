package configcmd

import (
	"fmt"

	"gopkg.in/yaml.v3"
)

// Config is the YAML accepted by Link commands.
type Config struct {
	Chains  []Chain  `yaml:"chains"`
	Signers []Signer `yaml:"signers"`
	DB      DB       `yaml:"db"`
	Relayer Relayer  `yaml:"relayer"`
}

// Chain configures one synthetic EVM chain.
type Chain struct {
	ID          string `yaml:"id"`
	Type        string `yaml:"type"`
	ChainID     uint64 `yaml:"chainId,omitempty"`
	EVMSigner   string `yaml:"evmSigner,omitempty"`
	ICS26Router string `yaml:"ics26Router,omitempty"`
	RPC         RPC    `yaml:"rpc"`
}

// Signer identifies one configured key.
type Signer struct {
	Alias string `yaml:"alias"`
	Type  string `yaml:"type"`
	File  string `yaml:"file"`
}

const (
	// ChainTypeEVM is the synthetic EVM chain type.
	ChainTypeEVM = "evm"
	// SignerTypeLocal is the file-backed signer type.
	SignerTypeLocal = "local"
	// RouteEVMToEVMAttested is the synthetic EVM route type.
	RouteEVMToEVMAttested = "evmToEvmAttested"
)

// RPC configures a chain RPC endpoint.
type RPC struct {
	URL string `yaml:"url"`
}

// Relayer configures directed synthetic routes.
type Relayer struct {
	Routes []Route `yaml:"routes"`
}

// DB configures the synthetic SQLite store.
type DB struct {
	Type string `yaml:"type"`
	URL  string `yaml:"url"`
}

// DBTypeSQLite is the supported synthetic database type.
const DBTypeSQLite = "sqlite"

// ValidateDB validates the synthetic store configuration.
func ValidateDB(db DB) error {
	switch db.Type {
	case DBTypeSQLite:
	case "":
		return fmt.Errorf("db type is empty")
	default:
		return fmt.Errorf("unsupported db type %q; expected %q", db.Type, DBTypeSQLite)
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

// AutoRelay configures automatic relay; omission means enabled.
type AutoRelay struct {
	Enabled bool `yaml:"enabled"`
}

// Route configures one directed synthetic relay route.
type Route struct {
	ID           string     `yaml:"id"`
	Source       string     `yaml:"source"`
	Destination  string     `yaml:"destination"`
	Type         string     `yaml:"type"`
	SourceClient string     `yaml:"sourceClient,omitempty"`
	DestClient   string     `yaml:"destClient,omitempty"`
	AutoRelay    *AutoRelay `yaml:"autoRelay,omitempty"`
}

// AutoRelayEnabled reports whether automatic relay is enabled.
func (r Route) AutoRelayEnabled() bool {
	return r.AutoRelay == nil || r.AutoRelay.Enabled
}

// Route returns a configured route by ID.
func (c *Config) Route(id string) (Route, bool) {
	for _, route := range c.Relayer.Routes {
		if route.ID == id {
			return route, true
		}
	}
	return Route{}, false
}

// Chain returns a configured chain by ID.
func (c *Config) Chain(id string) (Chain, bool) {
	for _, chain := range c.Chains {
		if chain.ID == id {
			return chain, true
		}
	}
	return Chain{}, false
}

// Marshal preserves unresolved environment references in the YAML.
func (c *Config) Marshal() ([]byte, error) {
	out, err := yaml.Marshal(c)
	if err != nil {
		return nil, fmt.Errorf("marshal config yaml: %w", err)
	}
	return out, nil
}

// Unmarshal parses the synthetic YAML config.
func Unmarshal(data []byte) (*Config, error) {
	var config Config
	if err := yaml.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("unmarshal config yaml: %w", err)
	}
	return &config, nil
}

// ValidationError describes one invalid config field.
type ValidationError struct {
	Path string `json:"path"`
	Msg  string `json:"msg"`
}

// ValidateResult is emitted by config validate.
type ValidateResult struct {
	Valid          bool              `json:"valid"`
	ResolvedChains []string          `json:"resolvedChains,omitempty"`
	Warnings       []string          `json:"warnings"`
	Errors         []ValidationError `json:"errors,omitempty"`
}

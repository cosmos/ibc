package wire

import (
	"fmt"

	"gopkg.in/yaml.v3"
)

type ConfigYAML struct {
	Chains  []Chain  `yaml:"chains"`
	Signers []Signer `yaml:"signers"`
	DB      DB       `yaml:"db"`
	Relayer Relayer  `yaml:"relayer"`
}

type Chain struct {
	ID            string `yaml:"id"`
	Type          string `yaml:"type"`
	ChainID       uint64 `yaml:"chainId,omitempty"`
	EVMSigner     string `yaml:"evmSigner,omitempty"`
	TestAppSigner string `yaml:"testAppSigner,omitempty"`
	RPC           RPC    `yaml:"rpc"`
}

type Signer struct {
	Alias string `yaml:"alias"`
	Type  string `yaml:"type"`
	File  string `yaml:"file"`
}

const (
	ChainTypeEVM    = "evm"
	SignerTypeLocal = "local"

	RouteEVMToEVMAttested = "evmToEvmAttested"
)

type RPC struct {
	URL string `yaml:"url"`
}

type Relayer struct {
	Routes []Route `yaml:"routes"`
}

type DB struct {
	Type string `yaml:"type"`
	URL  string `yaml:"url"`
}

const DBTypeSQLite = "sqlite"

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

// Omission means enabled.
type AutoRelay struct {
	Enabled bool `yaml:"enabled"`
}

type Route struct {
	ID          string     `yaml:"id"`
	Source      string     `yaml:"source"`
	Destination string     `yaml:"destination"`
	Type        string     `yaml:"type"`
	AutoRelay   *AutoRelay `yaml:"autoRelay,omitempty"`
}

func (r Route) AutoRelayEnabled() bool {
	return r.AutoRelay == nil || r.AutoRelay.Enabled
}

func (c *ConfigYAML) Route(id string) (Route, bool) {
	for _, r := range c.Relayer.Routes {
		if r.ID == id {
			return r, true
		}
	}
	return Route{}, false
}

func (c *ConfigYAML) Chain(id string) (Chain, bool) {
	for _, ch := range c.Chains {
		if ch.ID == id {
			return ch, true
		}
	}
	return Chain{}, false
}

// Marshal preserves unresolved ${NAME} references in the on-disk config.
func (c *ConfigYAML) Marshal() ([]byte, error) {
	out, err := yaml.Marshal(c)
	if err != nil {
		return nil, fmt.Errorf("marshal config yaml: %w", err)
	}
	return out, nil
}

func Unmarshal(data []byte) (*ConfigYAML, error) {
	var c ConfigYAML
	if err := yaml.Unmarshal(data, &c); err != nil {
		return nil, fmt.Errorf("unmarshal config yaml: %w", err)
	}
	return &c, nil
}

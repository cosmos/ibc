// Package config contains config and flag parsing logic
package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/goccy/go-yaml"
	"github.com/pkg/errors"

	"github.com/cosmos/ibc/link/internal/network"
)

// Database type
const (
	DBTypeSQLite   = "sqlite"
	DBTypePostgres = "postgres"
)

const sqliteInMemory = ":memory:"

// Config represents a config file
// Should only contain `camelCase` keywords
type Config struct {
	Server   ServerConfig   `yaml:"server"`
	DB       DBConfig       `yaml:"db"`
	Chains   []ChainConfig  `yaml:"chains"`
	Relayer  RelayerConfig  `yaml:"relayer"`
	Attestor AttestorConfig `yaml:"attestor"`
}

// ServerConfig config for RPC server for both relayer and attestor
type ServerConfig struct {
	ListenAddress string `yaml:"listenAddr"`
}

// DBConfig config for database storage.
type DBConfig struct {
	Type string `yaml:"type"`
	URL  string `yaml:"url"`
}

// AttestorConfig represents the entrypoint for running the process as an attestor
type AttestorConfig struct {
	Attestations []AttestationConfig `yaml:"attestations"`
}

// AttestationConfig represents a single attestation configuration in case when the binary
// runs attestors. Signer is a reference to .singers section in the config (future)
// Name should be unique within the config.
type AttestationConfig struct {
	ChainID string `yaml:"chainId"`
	Name    string `yaml:"name"`

	// todo: future work
	RouterAddress  string `yaml:"-"`
	FinalityOffset int64  `yaml:"-"`
	Signer         string `yaml:"-"`
}

// Validate validates the attestor config. Allows empty attestations.
func (c AttestorConfig) Validate() error {
	set := make(map[string]struct{})

	for _, attestation := range c.Attestations {
		if attestation.ChainID == "" {
			return errors.Errorf(".attestations chainId required")
		}
		if attestation.Name == "" {
			return errors.Errorf(".attestations name required")
		}
		if _, ok := set[attestation.Name]; ok {
			return errors.Errorf(".attestations duplicate name: %q", attestation.Name)
		}
		set[attestation.Name] = struct{}{}
	}

	return nil
}

// ChainType the execution environment of a chain.
type ChainType string

// Chain types
const (
	ChainTypeEVM ChainType = "evm"
)

// ChainConfig describes how to reach a chain.
type ChainConfig struct {
	ChainID string          `yaml:"chainId"`
	EVM     *EVMChainConfig `yaml:"evm,omitempty"`
}

// EVMChainConfig EVM connection details.
type EVMChainConfig struct {
	RPC string `yaml:"rpc"`
}

// DefaultConfig sample config using default values and Sqlite.
func DefaultConfig() Config {
	return Config{
		Server: ServerConfig{
			ListenAddress: "0.0.0.0:3000",
		},
		DB: DBConfig{
			Type: DBTypeSQLite,
			URL:  "ibc.db",
		},
		Chains: []ChainConfig{},
		Relayer: RelayerConfig{
			Chains: []RelayerChainConfig{},
		},
		Attestor: AttestorConfig{
			Attestations: []AttestationConfig{},
		},
	}
}

// LoadFromFile loads Config from file with optional validation.
// Note: supports ENV variables expansion!
func LoadFromFile(path string, validate, restrictUnknownFields bool) (Config, error) {
	config := DefaultConfig()

	bz, err := os.ReadFile(path)
	if err != nil {
		return Config{}, err
	}

	// substitute ENV variables
	expanded := os.ExpandEnv(string(bz))

	opts := []yaml.DecodeOption{}
	if restrictUnknownFields {
		opts = append(opts, yaml.DisallowUnknownField())
	}

	err = yaml.UnmarshalWithOptions([]byte(expanded), &config, opts...)
	if err != nil {
		return Config{}, err
	}

	if validate {
		if err := config.Validate(); err != nil {
			return Config{}, errors.Wrap(err, "validation failed")
		}
	}

	return config, nil
}

func (c Config) Validate() error {
	if err := c.Server.Validate(); err != nil {
		return errors.Wrap(err, ".server")
	}

	if err := c.DB.Validate(); err != nil {
		return errors.Wrap(err, ".db")
	}

	chainIDs := make(map[string]struct{})
	for _, chain := range c.Chains {
		if err := chain.Validate(); err != nil {
			return errors.Wrapf(err, ".chains[%s]", chain.ChainID)
		}

		if _, ok := chainIDs[chain.ChainID]; ok {
			return errors.Wrapf(errors.Errorf("duplicate chainId: %q", chain.ChainID), ".chains")
		}
		chainIDs[chain.ChainID] = struct{}{}
	}

	if err := c.validateChainReferences(); err != nil {
		return errors.Wrap(err, ".relayer")
	}

	if err := c.Relayer.Validate(); err != nil {
		return errors.Wrap(err, ".relayer")
	}

	if err := c.Attestor.Validate(); err != nil {
		return errors.Wrap(err, ".attestor")
	}

	return nil
}

// validateChainReferences ensures chains referenced by the relayer config are
// declared in the top-level chains block.
func (c Config) validateChainReferences() error {
	for _, chain := range c.Relayer.Chains {
		if _, ok := c.Chain(chain.ChainID); !ok {
			return errors.Errorf(".chains[%s] chainId not declared in top-level chains", chain.ChainID)
		}
	}

	for _, client := range c.Relayer.Clients {
		if _, ok := c.Chain(client.CounterpartyChainID); !ok {
			return errors.Errorf(
				".clients[%s] counterpartyChainId %q not declared in top-level chains",
				client.ClientID, client.CounterpartyChainID,
			)
		}
	}

	return nil
}

func (c Config) Chain(chainID string) (ChainConfig, bool) {
	for _, chain := range c.Chains {
		if chain.ChainID == chainID {
			return chain, true
		}
	}

	return ChainConfig{}, false
}

func (c ChainConfig) Validate() error {
	switch {
	case c.ChainID == "":
		return errors.New(".chainId required")
	case c.EVM == nil:
		return errors.Errorf(".evm required for chain %q (only %s chains are supported)", c.ChainID, ChainTypeEVM)
	case c.EVM.RPC == "":
		return errors.Errorf(".evm.rpc required for chain %q", c.ChainID)
	}

	return nil
}

func (c Config) StoreToFile(path string) error {
	if err := EnsureDirectory(path); err != nil {
		return err
	}

	bz, err := yaml.Marshal(c)
	if err != nil {
		return err
	}

	if err := os.WriteFile(path, bz, 0o644); err != nil {
		return err
	}

	return nil
}

func (c ServerConfig) Validate() error {
	if err := network.ValidateListenAddr(c.ListenAddress); err != nil {
		return errors.Wrapf(err, ".listenAddr %q", c.ListenAddress)
	}

	return nil
}

func (c DBConfig) Validate() error {
	switch {
	case c.Type != DBTypeSQLite && c.Type != DBTypePostgres:
		return errors.Errorf(".type must be one of [%q, %q], got %q", DBTypeSQLite, DBTypePostgres, c.Type)
	case c.Type == DBTypeSQLite && c.URL == sqliteInMemory:
		return errors.New(".url must not be :memory: for sqlite")
	case c.URL == "":
		return errors.New(".url must not be empty")
	}

	return nil
}

// Label returns a human-readable label for the DB config.
func (c DBConfig) Label() string {
	if c.Type != DBTypeSQLite {
		return c.Type
	}

	// sqlite case
	path := c.URL
	if abs, err := filepath.Abs(path); err == nil {
		path = abs
	}

	return path
}

// DBConfigFromURL infers DB type from a CLI database URL override.
func DBConfigFromURL(url string) (DBConfig, error) {
	db := DBConfig{
		URL:  url,
		Type: dbTypeFromURL(url),
	}

	return db, db.Validate()
}

// PrintJSON prints anything as JSON to stdout.
func PrintJSON(v any) error {
	bz, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}

	fmt.Println(string(bz))

	return nil
}

func dbTypeFromURL(raw string) string {
	if strings.HasPrefix(raw, "postgres://") || strings.HasPrefix(raw, "postgresql://") {
		return DBTypePostgres
	}

	return DBTypeSQLite
}

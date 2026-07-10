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

// Signer type. Local represents a private key file. Remote connects to cosmos/KMS.
const (
	SignerLocal  = "local"
	SignerRemote = "remote"
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
	Signers  Signers        `yaml:"signers"`
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

// Signers is the list of configured signer backends.
type Signers []SignerConfig

// SignerConfig represents a single signer configuration in the config.
type SignerConfig struct {
	// Alias unique name for a signer
	Alias string `yaml:"alias"`

	// Type [local, remote]
	Type string `yaml:"type"`

	// File key file path for a local signer
	File string `yaml:"file"`

	// GRPC address for a remote signer
	GRPC string `yaml:"grpc"`

	// RemoteKeyID KMS key ID for a remote signer
	RemoteKeyID string `yaml:"remoteKeyId"`
}

// AttestationConfig represents a single attestation configuration when the binary runs attestors.
// Name should be unique within the config.
type AttestationConfig struct {
	ChainID string `yaml:"chainId"`
	Name    string `yaml:"name"`

	// Signer reference to .signers section in the config
	// Should be unique across all attestations!
	Signer string `yaml:"signer"`

	// todo: future work
	RouterAddress  string `yaml:"-"`
	FinalityOffset int64  `yaml:"-"`
}

// ChainType the execution environment of a chain.
type ChainType string

// Chain types
const (
	ChainTypeEVM ChainType = "evm"
)

// ChainConfig a chain's type, connection details, and core IBC contracts.
type ChainConfig struct {
	ChainID string          `yaml:"chainId"`
	Type    ChainType       `yaml:"type"`
	RPC     string          `yaml:"rpc"`
	EVM     *EVMChainConfig `yaml:"evm,omitempty"`
}

// EVMChainConfig EVM-specific chain details.
type EVMChainConfig struct {
	ICS26Router string `yaml:"ics26Router"`
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
		Signers: Signers{},
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

	if err := c.Signers.Validate(); err != nil {
		return errors.Wrap(err, ".signers")
	}

	return c.crossValidate()
}

func (c Config) crossValidate() error {
	signerSet := make(map[string]struct{}, len(c.Signers))
	for _, signer := range c.Signers {
		signerSet[signer.Alias] = struct{}{}
	}

	for i, attestation := range c.Attestor.Attestations {
		if _, exists := signerSet[attestation.Signer]; !exists {
			return errors.Errorf(
				".attestor.attestations[%d].signer references unknown signer: %q",
				i,
				attestation.Signer,
			)
		}
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
		if _, ok := c.Chain(client.ChainID); client.ChainID != "" && !ok {
			return errors.Errorf(
				".clients[%s] chainId %q not declared in top-level chains",
				client.ClientID, client.ChainID,
			)
		}

		if _, ok := c.Chain(client.CounterpartyChainID); client.CounterpartyChainID != "" && !ok {
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
	case c.Type != ChainTypeEVM:
		return errors.Errorf(".type unknown chain type: %q", c.Type)
	case c.RPC == "":
		return errors.New(".rpc required")
	}

	if c.Type == ChainTypeEVM {
		switch {
		case c.EVM == nil:
			return errors.Errorf(".evm required for %s chains", ChainTypeEVM)
		case c.EVM.ICS26Router == "":
			return errors.New(".evm.ics26Router required")
		}
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

// Validate validates the attestor config. Allows empty attestations.
func (c AttestorConfig) Validate() error {
	nameSet := make(map[string]struct{})
	signerSet := make(map[string]struct{})

	for i, attestation := range c.Attestations {
		if err := attestation.Validate(); err != nil {
			return errors.Wrapf(err, ".attestations[%d]", i)
		}

		if _, exists := nameSet[attestation.Name]; exists {
			return errors.Errorf(".attestations[%d] duplicate name: %q", i, attestation.Name)
		}

		if _, exists := signerSet[attestation.Signer]; exists {
			return errors.Errorf(".attestations[%d] duplicate signer: %q", i, attestation.Signer)
		}

		nameSet[attestation.Name] = struct{}{}
		signerSet[attestation.Signer] = struct{}{}
	}

	return nil
}

func (c AttestationConfig) Validate() error {
	switch {
	case c.ChainID == "":
		return errors.Errorf(".chainId required")
	case c.Name == "":
		return errors.Errorf(".name required")
	case c.Signer == "":
		return errors.Errorf(".signer required")
	}

	return nil
}

func (c Signers) Validate() error {
	set := make(map[string]struct{})

	for i, signer := range c {
		if err := signer.Validate(); err != nil {
			return errors.Wrapf(err, ".signers[%d]", i)
		}

		if _, exists := set[signer.Alias]; exists {
			return errors.Errorf(".signers duplicate alias: %q", signer.Alias)
		}

		set[signer.Alias] = struct{}{}
	}

	return nil
}

func (c SignerConfig) Validate() error {
	switch {
	case c.Alias == "":
		return errors.New(".alias required")
	case c.Type == "":
		return errors.New(".type required")
	case c.Type != SignerLocal && c.Type != SignerRemote:
		return errors.Errorf(".type must be one of [%q, %q], got %q", SignerLocal, SignerRemote, c.Type)
	case c.Type == SignerLocal && c.File == "":
		return errors.New(".file required for local signer")
	case c.Type == SignerRemote && c.GRPC == "":
		return errors.New(".grpc required for remote signer")
	case c.Type == SignerRemote && c.RemoteKeyID == "":
		return errors.New(".remoteKeyId required for remote signer")
	}

	if c.Type == SignerLocal {
		path, err := ExpandHome(c.File)
		if err != nil {
			return errors.Wrap(err, ".file")
		}

		fallbacks := KeyFileFallbacks(path)

		if err := fileExistsInAny(fallbacks...); err != nil {
			return errors.Wrapf(err, ".file %s", path)
		}
	}

	return nil
}

func KeyFileFallbacks(keyPath string) []string {
	fallbacks := []string{keyPath}

	// absolute path, no fallbacks needed
	if filepath.IsAbs(keyPath) {
		return fallbacks
	}

	// forgot to add .json extension
	if !strings.HasSuffix(keyPath, ".json") {
		keyPath = fmt.Sprintf("%s.json", keyPath)

		fallbacks = append(fallbacks, keyPath)
	}

	// forgot to add keys/ directory
	if !strings.Contains(keyPath, "keys/") {
		keyPath = filepath.Join("keys", keyPath)

		fallbacks = append(fallbacks, keyPath)
	}

	return fallbacks
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

func fileExistsInAny(path ...string) error {
	for _, p := range path {
		if err := fileExists(p); err == nil {
			return nil
		}
	}

	return errors.New("file not found")
}

func fileExists(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		return err
	}

	if info.IsDir() {
		return errors.Errorf("path is a directory")
	}

	return nil
}

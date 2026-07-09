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

const (
	signerTypeLocal  = "local"
	signerTypeRemote = "remote"
)

const sqliteInMemory = ":memory:"

// Config represents a config file
// Should only contain `camelCase` keywords
type Config struct {
	Server   ServerConfig   `yaml:"server"`
	DB       DBConfig       `yaml:"db"`
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

	if err := c.Attestor.Validate(); err != nil {
		return errors.Wrap(err, ".attestor")
	}

	if err := c.Signers.Validate(); err != nil {
		return errors.Wrap(err, ".signers")
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
	case c.Type != signerTypeLocal && c.Type != signerTypeRemote:
		return errors.Errorf(".type must be one of [%q, %q], got %q", signerTypeLocal, signerTypeRemote, c.Type)
	case c.Type == signerTypeLocal && c.File == "":
		return errors.New(".file required for local signer")
	case c.Type == signerTypeRemote && c.GRPC == "":
		return errors.New(".grpc required for remote signer")
	case c.Type == signerTypeRemote && c.RemoteKeyID == "":
		return errors.New(".remoteKeyId required for remote signer")
	}

	if c.Type == signerTypeLocal {
		path, err := ExpandHome(c.File)
		if err != nil {
			return errors.Wrap(err, ".file")
		}

		if err := fileExists(path); err != nil {
			return errors.Wrapf(err, ".file %q", path)
		}
	}

	return nil
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

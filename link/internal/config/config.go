// Package config contains config and flag parsing logic
package config

import (
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"

	"github.com/goccy/go-yaml"
	"github.com/pkg/errors"

	"github.com/cosmos/ibc/link/internal/network"
)

// Database type
const (
	DBTypeSQLite   = "sqlite"
	DBTypePostgres = "postgres"

	sqliteInMemoryURL = ":memory:"
)

// Config represents a config file
// Should only contain `camelCase` keywords
type Config struct {
	GRPC GRPCConfig `yaml:"grpc"`
	DB   DBConfig   `yaml:"db"`
}

// GRPCConfig config for grpc server for both relayer and attestor
type GRPCConfig struct {
	ListenAddress string `yaml:"listenAddr"`
}

// DBConfig config for database storage.
type DBConfig struct {
	Type string `yaml:"type"`
	URL  string `yaml:"url"`
}

// DefaultConfig sample config using default values and Sqlite.
func DefaultConfig() Config {
	return Config{
		GRPC: GRPCConfig{
			ListenAddress: "0.0.0.0:3000",
		},
		DB: DBConfig{
			Type: DBTypeSQLite,
			URL:  "ibc.db",
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
	if err := c.GRPC.Validate(); err != nil {
		return errors.Wrap(err, ".grpc")
	}

	if err := c.DB.Validate(); err != nil {
		return errors.Wrap(err, ".db")
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

func (c GRPCConfig) Validate() error {
	if err := network.ValidateListenAddr(c.ListenAddress); err != nil {
		return errors.Wrapf(err, ".listenAddr %q", c.ListenAddress)
	}

	return nil
}

func (c DBConfig) Validate() error {
	switch c.Type {
	case DBTypeSQLite:
		if err := validateSQLiteURL(c.URL); err != nil {
			return err
		}
	case DBTypePostgres:
		if err := validatePostgresURL(c.URL); err != nil {
			return err
		}
	default:
		return errors.Errorf(".type must be one of [%q, %q], got %q", DBTypeSQLite, DBTypePostgres, c.Type)
	}

	return nil
}

func validateSQLiteURL(raw string) error {
	if raw == "" {
		return errors.New(".url must not be empty")
	}

	if raw == sqliteInMemoryURL {
		return errors.Errorf(".url must be a filesystem path, got %q", sqliteInMemoryURL)
	}

	parsed, err := url.Parse(raw)
	if err != nil {
		return errors.Wrap(err, ".url")
	}

	if parsed.Scheme != "" {
		return errors.Errorf(".url for sqlite must be a filesystem path, got scheme %q", parsed.Scheme)
	}

	return nil
}

func validatePostgresURL(raw string) error {
	if raw == "" {
		return errors.New(".url must not be empty")
	}

	parsed, err := url.Parse(raw)
	if err != nil {
		return errors.Wrap(err, ".url")
	}

	switch parsed.Scheme {
	case DBTypePostgres, "postgresql":
		return nil
	default:
		return errors.Errorf(
			".url for postgres must use scheme %q or %q, got %q",
			DBTypePostgres,
			"postgresql",
			parsed.Scheme,
		)
	}
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
func DBConfigFromURL(raw string) (DBConfig, error) {
	if raw == "" {
		return DBConfig{}, errors.New("empty db url")
	}

	if raw == sqliteInMemoryURL {
		return DBConfig{}, validateSQLiteURL(raw)
	}

	parsed, err := url.Parse(raw)
	if err != nil {
		return DBConfig{}, errors.Wrap(err, "parse db url")
	}

	var cfg DBConfig
	switch parsed.Scheme {
	case "":
		cfg = DBConfig{
			Type: DBTypeSQLite,
			URL:  raw,
		}
	case DBTypePostgres, "postgresql":
		cfg = DBConfig{
			Type: DBTypePostgres,
			URL:  raw,
		}
	default:
		return DBConfig{}, errors.Errorf("unsupported db url scheme %q", parsed.Scheme)
	}

	if err := cfg.Validate(); err != nil {
		return DBConfig{}, err
	}

	return cfg, nil
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

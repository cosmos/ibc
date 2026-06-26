// Package config contains config and flag parsing logic
package config

import (
	"os"
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

func DefaultConfig() Config {
	return Config{
		GRPC: GRPCConfig{
			ListenAddress: "0.0.0.0:3000",
		},
		DB: DBConfig{
			Type: DBTypeSQLite,
			URL:  "ibc.sqlite",
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
	if err := ensureDirectory(path); err != nil {
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
	case DBTypeSQLite, DBTypePostgres:
	default:
		return errors.Errorf(".type must be one of [%q, %q], got %q", DBTypeSQLite, DBTypePostgres, c.Type)
	}

	if c.URL == "" {
		return errors.New(".url must not be empty")
	}

	return nil
}

// DBConfigFromURL infers DB type from a CLI database URL override.
func DBConfigFromURL(raw string) (DBConfig, error) {
	if raw == "" {
		return DBConfig{}, errors.New("empty db url")
	}

	if isSQLitePath(raw) {
		return DBConfig{
			Type: DBTypeSQLite,
			URL:  raw,
		}, nil
	}

	return DBConfig{
		Type: DBTypePostgres,
		URL:  raw,
	}, nil
}

func isSQLitePath(path string) bool {
	return stringMatches(path, strings.HasPrefix, ".", "/") ||
		stringMatches(path, strings.HasSuffix, ".sqlite", ".db")
}

func stringMatches(input string, matcher func(string, string) bool, patterns ...string) bool {
	for _, pattern := range patterns {
		if matcher(input, pattern) {
			return true
		}
	}

	return false
}

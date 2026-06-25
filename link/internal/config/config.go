// Package config contains config and flag parsing logic
package config

import (
	"os"

	"github.com/goccy/go-yaml"
	"github.com/pkg/errors"

	"github.com/cosmos/ibc/link/internal/network"
)

// Config represents a config file
type Config struct {
	GRPC GRPCConfig `yaml:"grpc"`
}

// GRPCConfig represents a config for the GRPC server
type GRPCConfig struct {
	ListenAddress string `yaml:"listenAddr"`
}

// DefaultConfig returns the default Config.
func DefaultConfig() Config {
	return Config{
		GRPC: GRPCConfig{
			ListenAddress: "0.0.0.0:3000",
		},
	}
}

// LoadFromFile loads Config from file with optional validation.
// Note: supports ENV variables expansion!
func LoadFromFile(path string, validate bool) (Config, error) {
	bz, err := os.ReadFile(path)
	if err != nil {
		return Config{}, err
	}

	config := DefaultConfig()
	expanded := os.ExpandEnv(string(bz))
	if err := yaml.Unmarshal([]byte(expanded), &config); err != nil {
		return Config{}, err
	}

	if validate {
		if err := config.Validate(); err != nil {
			return Config{}, errors.Wrap(err, "validation failed")
		}
	}

	return config, nil
}

// Validate validates the Config.
func (c Config) Validate() error {
	if err := c.GRPC.Validate(); err != nil {
		return errors.Wrap(err, ".grpc")
	}

	return nil
}

// StoreToFile stores the Config to a file.
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

// Validate validates the GRPCConfig.
func (c GRPCConfig) Validate() error {
	if err := network.ValidateListenAddr(c.ListenAddress); err != nil {
		return errors.Wrapf(err, ".listenAddr %q", c.ListenAddress)
	}

	return nil
}

// Package config contains config and flag parsing logic
package config

import (
	"os"

	"github.com/pkg/errors"

	"github.com/cosmos/ibc/ibc-link/packages/go/network"
	"github.com/goccy/go-yaml"
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

func (c Config) Validate() error {
	if err := c.GRPC.Validate(); err != nil {
		return errors.Wrap(err, ".grpc")
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

	if err := os.WriteFile(path, bz, 0644); err != nil {
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

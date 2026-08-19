// SPDX-License-Identifier: Apache-2.0

// Package config defines, validates, and encodes IBC Link configuration.
package config

import (
	"fmt"
	"path/filepath"
	"strings"
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
	Server    ServerConfig  `yaml:"server"`
	DB        DBConfig      `yaml:"db"`
	Chains    []ChainConfig `yaml:"chains"`
	Relayer   RelayerConfig `yaml:"relayer"`
	Attestors Attestors     `yaml:"attestors"`
	Signers   Signers       `yaml:"signers"`
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

// Attestors is the list of attestors, used both by the relayer
// (to resolve who to query) and the attestor binary (to know what it
// serves locally).
type Attestors []AttestorConfig

// AttestorConfig describes one attestor, either run by this process
// (type: local) or reachable over gRPC (type: remote).
type AttestorConfig struct {
	// ChainID is the chain this attestor watches.
	ChainID string `yaml:"chainId,omitempty"`

	// Name is the attestor's own self-reported identity. Not required unique.
	Name string `yaml:"name"`

	Type AttestorType `yaml:"type"`

	// Signer required for type: local only -- the signer used to sign
	// attestations.
	Signer string `yaml:"signer"`

	// FinalityOffset local only. Zero attests up to the chain's "finalized"
	// tag; n > 0 attests up to "latest" - n instead.
	FinalityOffset uint `yaml:"finalityOffset"`

	// GRPC required for type: remote only. Bare host:port.
	GRPC string `yaml:"grpc,omitempty"`
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
	File string `yaml:"file,omitempty"`

	// GRPC address for a remote signer
	GRPC string `yaml:"grpc,omitempty"`

	// RemoteKeyID KMS key ID for a remote signer
	RemoteKeyID string `yaml:"remoteKeyId,omitempty"`
}

// ChainType the execution environment of a chain.
type ChainType string

// Chain types
const (
	ChainTypeEVM ChainType = "evm"
)

// ChainConfig chain information shared by the attestor and relayer.
type ChainConfig struct {
	ChainID string          `yaml:"chainId"`
	EVM     *EVMChainConfig `yaml:"evm,omitempty"`

	// Deployer optional signer alias used by `ibc deploy` for this chain.
	Deployer string `yaml:"deployer,omitempty"`
}

// Type returns the chain type implied by the configured settings.
func (c ChainConfig) Type() ChainType {
	if c.EVM != nil {
		return ChainTypeEVM
	}

	return ""
}

// EVMChainConfig EVM-specific chain details.
type EVMChainConfig struct {
	RPC         string `yaml:"rpc"`
	ICS26Router string `yaml:"ics26Router"`
}

// Default returns a config populated with the standard defaults.
func Default() Config {
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
			ChainOverrides: []RelayerChainOverride{},
			Connections:    []ConnectionConfig{},
		},
		Attestors: Attestors{},
		Signers:   Signers{},
	}
}

// ChainSignerPair one (chain, signer alias) pair a client end submits with.
type ChainSignerPair struct {
	ChainID     string
	SignerAlias string
}

// RelayerChainSignerPairs resolves the unique (chain, signer) pairs across
// every configured connection's two client ends.
func RelayerChainSignerPairs(c Config) []ChainSignerPair {
	seen := make(map[ChainSignerPair]struct{})

	var pairs []ChainSignerPair

	for _, conn := range c.Relayer.Connections {
		for _, end := range []ClientEnd{conn.ClientA, conn.ClientB} {
			pair := ChainSignerPair{ChainID: end.ChainID, SignerAlias: end.Signer}
			if _, dup := seen[pair]; dup {
				continue
			}

			seen[pair] = struct{}{}
			pairs = append(pairs, pair)
		}
	}

	return pairs
}

func (c Config) Chain(chainID string) (ChainConfig, bool) {
	for _, chain := range c.Chains {
		if chain.ChainID == chainID {
			return chain, true
		}
	}

	return ChainConfig{}, false
}

// Signer returns the signer with the given alias.
func (c Config) Signer(alias string) (SignerConfig, bool) {
	for _, signer := range c.Signers {
		if signer.Alias == alias {
			return signer, true
		}
	}

	return SignerConfig{}, false
}

// AttestorByName returns the configured attestor with the given name.
func (c Config) AttestorByName(name string) (AttestorConfig, bool) {
	for _, attestor := range c.Attestors {
		if attestor.Name == name {
			return attestor, true
		}
	}

	return AttestorConfig{}, false
}

// AttestorsForChain returns every configured attestor watching chainID.
func (c Config) AttestorsForChain(chainID string) []AttestorConfig {
	var attestors []AttestorConfig
	for _, attestor := range c.Attestors {
		if attestor.ChainID == chainID {
			attestors = append(attestors, attestor)
		}
	}

	return attestors
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

const finalityOffsetTODO = `TODO: set appropriately. 0 defaults to chain finality`

// CollectComments builds TODO comments for every field in cfg that's left
// for the operator to fill in by hand, keyed by YAML path for
// MarshalYAMLWithComments.
func CollectComments(cfg Config) map[string]string {
	comments := map[string]string{}

	for i, chain := range cfg.Chains {
		if chain.EVM != nil && chain.EVM.ICS26Router == "" {
			path := fmt.Sprintf("$.chains[%d].evm.ics26Router", i)
			comments[path] = "TODO: fill in"
		}
	}

	for i, conn := range cfg.Relayer.Connections {
		if conn.ClientA.Signer == "" {
			path := fmt.Sprintf("$.relayer.connections[%d].clientA.signer", i)
			comments[path] = "TODO: signers[] alias that submits relay txs on chainA"
		}
		if conn.ClientB.Signer == "" {
			path := fmt.Sprintf("$.relayer.connections[%d].clientB.signer", i)
			comments[path] = "TODO: signers[] alias that submits relay txs on chainB"
		}
	}

	for i, attestor := range cfg.Attestors {
		if attestor.Type != AttestorTypeLocal {
			continue
		}
		if attestor.Signer == "" {
			path := fmt.Sprintf("$.attestors[%d].signer", i)
			comments[path] = "TODO: signers[] alias backing this attestor's key"
		}
		path := fmt.Sprintf("$.attestors[%d].finalityOffset", i)
		comments[path] = finalityOffsetTODO
	}

	return comments
}

func dbTypeFromURL(raw string) string {
	if strings.HasPrefix(raw, "postgres://") || strings.HasPrefix(raw, "postgresql://") {
		return DBTypePostgres
	}

	return DBTypeSQLite
}

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
	// Alias is this config's unique local handle for the attestor, referenced
	// by the relayer
	Alias string `yaml:"alias"`

	// Name is the attestor's own self-reported identity. Not required unique.
	Name string `yaml:"name"`

	// ChainID is the chain this attestor watches.
	ChainID string `yaml:"chainId"`

	Type AttestorType `yaml:"type"`

	// Signer required for type: local only -- the signer used to sign attestations.
	Signer string `yaml:"signer,omitempty"`

	// FinalityOffset local only. Zero attests up to the chain's "finalized"
	// tag; n > 0 attests up to "latest" - n instead.
	FinalityOffset uint `yaml:"finalityOffset,omitempty"`

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
	File string `yaml:"file"`

	// GRPC address for a remote signer
	GRPC string `yaml:"grpc"`

	// RemoteKeyID KMS key ID for a remote signer
	RemoteKeyID string `yaml:"remoteKeyId"`
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
			ChainOverrides: []RelayerChainOverride{},
			Connections:    []ConnectionConfig{},
		},
		Attestors: Attestors{},
		Signers:   Signers{},
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

	if err := c.Relayer.Validate(); err != nil {
		return errors.Wrap(err, ".relayer")
	}

	if err := c.Attestors.Validate(); err != nil {
		return errors.Wrap(err, ".attestors")
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

	attestorSet := make(map[string]struct{}, len(c.Attestors))
	for _, a := range c.Attestors {
		attestorSet[a.Alias] = struct{}{}
	}

	for i, a := range c.Attestors {
		if a.Type != AttestorTypeLocal {
			continue
		}
		if _, exists := signerSet[a.Signer]; !exists {
			return errors.Errorf(".attestors[%d].signer references unknown signer: %q", i, a.Signer)
		}
	}

	if err := c.validateChainReferences(); err != nil {
		return err
	}

	if err := c.validateConnectionSigners(signerSet); err != nil {
		return errors.Wrap(err, ".relayer.connections")
	}

	if err := c.validateAttestorReferences(attestorSet); err != nil {
		return errors.Wrap(err, ".relayer.connections")
	}

	return nil
}

type namedClientEnd struct {
	label string
	cfg   ClientConfig
}

func connectionEnds(conn ConnectionConfig) []namedClientEnd {
	return []namedClientEnd{{"clientA", conn.ClientA}, {"clientB", conn.ClientB}}
}

// ChainSignerPair one (chain, signer alias) pair a client end submits with.
type ChainSignerPair struct {
	ChainID     string
	SignerAlias string
}

// RelayerChainSignerPairs resolves the unique (chain, signer) pairs across
// every configured connection's two client ends.
func RelayerChainSignerPairs(c Config) ([]ChainSignerPair, error) {
	seen := make(map[ChainSignerPair]struct{})

	var pairs []ChainSignerPair

	for _, conn := range c.Relayer.Connections {
		for _, end := range []ClientConfig{conn.ClientA, conn.ClientB} {
			pair := ChainSignerPair{ChainID: end.ChainID, SignerAlias: end.Signer}
			if _, dup := seen[pair]; dup {
				continue
			}

			seen[pair] = struct{}{}
			pairs = append(pairs, pair)
		}
	}

	return pairs, nil
}

// validateChainReferences ensures chains referenced by the relayer config are
// declared in the top-level chains block.
func (c Config) validateChainReferences() error {
	for _, chain := range c.Relayer.ChainOverrides {
		if _, ok := c.Chain(chain.ChainID); chain.ChainID != "" && !ok {
			return errors.Errorf(".chainOverrides[%s] chainId not declared in top-level chains", chain.ChainID)
		}
	}

	for _, conn := range c.Relayer.Connections {
		for _, end := range connectionEnds(conn) {
			if _, ok := c.Chain(end.cfg.ChainID); end.cfg.ChainID != "" && !ok {
				return errors.Errorf(
					".connections[%s].%s chainId %q not declared in top-level chains",
					conn.Alias, end.label, end.cfg.ChainID,
				)
			}
		}
	}

	return nil
}

// validateConnectionSigners ensures every client end's signer resolves to a
// configured signer.
func (c Config) validateConnectionSigners(signerSet map[string]struct{}) error {
	for _, conn := range c.Relayer.Connections {
		for _, end := range connectionEnds(conn) {
			if _, exists := signerSet[end.cfg.Signer]; !exists {
				return errors.Errorf(
					"connection %q %s references unknown signer %q",
					conn.Alias, end.label, end.cfg.Signer,
				)
			}
		}
	}

	return nil
}

// validateAttestorReferences ensures every attestorSet.attestors[] alias
// resolves to a declared top-level attestor.
func (c Config) validateAttestorReferences(attestorSet map[string]struct{}) error {
	for _, conn := range c.Relayer.Connections {
		for _, end := range connectionEnds(conn) {
			if end.cfg.AttestorSet == nil {
				continue
			}
			for _, alias := range end.cfg.AttestorSet.Attestors {
				if _, exists := attestorSet[alias]; !exists {
					return errors.Errorf(
						"connection %q %s attestorSet references unknown attestor %q",
						conn.Alias, end.label, alias,
					)
				}
			}
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
	if c.ChainID == "" {
		return errors.New(".chainId required")
	}

	if c.Type() == ChainTypeEVM {
		switch {
		case c.EVM.RPC == "":
			return errors.New(".evm.rpc required")
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

// Validate validates the attestors list. Allows empty.
func (a Attestors) Validate() error {
	aliases := make(map[string]struct{})
	for i, attestor := range a {
		if err := attestor.Validate(); err != nil {
			return errors.Wrapf(err, "[%d]", i)
		}
		if _, exists := aliases[attestor.Alias]; exists {
			return errors.Errorf("duplicate alias: %q", attestor.Alias)
		}
		aliases[attestor.Alias] = struct{}{}
	}

	return nil
}

func (c AttestorConfig) Validate() error {
	switch {
	case c.Alias == "":
		return errors.New(".alias required")
	case c.Name == "":
		return errors.New(".name required")
	case c.ChainID == "":
		return errors.New(".chainId required")
	case c.Type != AttestorTypeLocal && c.Type != AttestorTypeRemote:
		return errors.Errorf(".type unknown attestor type: %q", c.Type)
	}

	switch c.Type {
	case AttestorTypeLocal:
		switch {
		case c.Signer == "":
			return errors.New(".signer required for local attestors")
		case c.GRPC != "":
			return errors.New(".grpc must not be set for local attestors")
		}
	case AttestorTypeRemote:
		switch {
		case c.GRPC == "":
			return errors.New(".grpc required for remote attestors")
		case strings.Contains(c.GRPC, "://"):
			return errors.Errorf(".grpc must be a bare host:port, not a URL: %q", c.GRPC)
		case c.Signer != "":
			return errors.New(".signer must not be set for remote attestors")
		case c.FinalityOffset != 0:
			return errors.New(".finalityOffset must not be set for remote attestors")
		}
	}

	return nil
}

// Locals returns the subset of attestors this process runs itself.
func (a Attestors) Locals() []AttestorConfig {
	var locals []AttestorConfig
	for _, attestor := range a {
		if attestor.Type == AttestorTypeLocal {
			locals = append(locals, attestor)
		}
	}

	return locals
}

// ByAlias returns the attestor config for the given alias.
func (a Attestors) ByAlias(alias string) (AttestorConfig, bool) {
	for _, attestor := range a {
		if attestor.Alias == alias {
			return attestor, true
		}
	}

	return AttestorConfig{}, false
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

// KeyFileFallbacks returns the paths tried for a local signer key file.
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

// PrintYAML prints anything as YAML to stdout.
func PrintYAML(v any) error {
	bz, err := yaml.Marshal(v)
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

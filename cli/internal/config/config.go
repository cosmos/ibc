// SPDX-License-Identifier: Apache-2.0

// Package config contains config and flag parsing logic
package config

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/ethereum/go-ethereum/common"
	"github.com/goccy/go-yaml"

	"github.com/cosmos/ibc/cli/internal/network"
)

// Chain types
const (
	ChainTypeEVM ChainType = "evm"
)

// Attestor types
const (
	AttestorTypeRemote AttestorType = "remote"
	AttestorTypeLocal  AttestorType = "local"
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

const finalityOffsetTODO = `TODO: set appropriately. 0 defaults to chain finality`

type (
	// ChainType the execution environment of a chain.
	ChainType string

	// AttestorType how an attestor is reached.
	AttestorType string

	// ValidationType the type of config validation to perform.
	ValidationType uint8
)

// Config represents a config file
// Should only contain `camelCase` keywords
type Config struct {
	Server    ServerConfig  `yaml:"server"`
	DB        DBConfig      `yaml:"db"`
	Chains    Chains        `yaml:"chains"`
	Relayer   RelayerConfig `yaml:"relayer"`
	Attestors Attestors     `yaml:"attestors"`
	Signers   Signers       `yaml:"signers"`

	originalFilePath string
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

// Chains is the list of configured chains.
type Chains []ChainConfig

// ChainConfig chain information shared by the attestor and relayer.
type ChainConfig struct {
	ChainID string          `yaml:"chainId"`
	EVM     *EVMChainConfig `yaml:"evm,omitempty"`

	// Deployer optional signer alias used by `ibc deploy` for this chain.
	Deployer string `yaml:"deployer,omitempty"`
}

// EVMChainConfig EVM-specific chain details.
type EVMChainConfig struct {
	RPC string `yaml:"rpc"`

	// WS is a websocket endpoint, required for chains sourcing auto-relayed routes.
	WS string `yaml:"ws,omitempty"`

	ICS26Router string `yaml:"ics26Router"`
}

// Attestors is the list of attestors, used both by the relayer
// (to resolve who to query) and the attestor binary (to know what it serves locally).
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

// ChainSignerPair one (chain, signer alias) pair a client end submits with.
type ChainSignerPair struct {
	ChainID     string
	SignerAlias string
}

type namedClientEnd struct {
	label string
	cfg   ClientEnd
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

// Validate perform basic correctness checks
func (c Config) Validate() error {
	// .path.of.the.config ==> validation function
	type validationStep struct {
		path     string
		validate func() error
	}

	for _, step := range []validationStep{
		{"server", c.Server.Validate},
		{"db", c.DB.Validate},
		{"signers", c.Signers.Validate},
		{"chains", c.Chains.Validate},
		{"attestors", c.Attestors.Validate},
		{"relayer", c.Relayer.Validate},
		{"relayer", c.validateAutoRelay},
		{"", c.crossValidate},
	} {
		if err := step.validate(); err != nil {
			return errPath(step.path, err)
		}
	}

	return nil
}

// RelayerSufficiency validates the relayer server is runnable
func (c Config) RelayerSufficiency() error {
	if len(c.Relayer.Connections) == 0 {
		return errPathf("connections", "no connections configured")
	}

	return c.validateRunnable()
}

// AttestorSufficiency validates the attestor server is runnable
func (c Config) AttestorSufficiency() error {
	var hasLocal bool
	for _, a := range c.Attestors {
		if a.Type == AttestorTypeLocal {
			hasLocal = true
			break
		}
	}

	if !hasLocal {
		return errPathf("attestors", "no local attestors configured")
	}

	return c.validateRunnable()
}

func (c Config) OriginalFilePath() string {
	return c.originalFilePath
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

// AttestorsByChain returns every configured attestor watching chainID.
func (c Config) AttestorsByChain(chainID string) []AttestorConfig {
	var attestors []AttestorConfig
	for _, attestor := range c.Attestors {
		if attestor.ChainID == chainID {
			attestors = append(attestors, attestor)
		}
	}

	return attestors
}

func (c Config) StoreToFile(path string) error {
	return storeConfig(c, path, nil)
}

// StoreToFileWithComments writes c to path as YAML, with a TODO comment
// attached to every field CollectComments flags as left for the operator to
// fill in.
func (c Config) StoreToFileWithComments(path string) error {
	return storeConfig(c, path, CollectComments(c))
}

func (c ServerConfig) Validate() error {
	if err := network.ValidateListenAddr(c.ListenAddress); err != nil {
		return errPathf("listenAddr", "invalid listen address: %q", c.ListenAddress)
	}

	return nil
}

func (c DBConfig) Validate() error {
	switch {
	case c.Type != DBTypeSQLite && c.Type != DBTypePostgres:
		return errPathf("type", "must be one of [%q, %q], got %q", DBTypeSQLite, DBTypePostgres, c.Type)
	case c.Type == DBTypeSQLite && c.URL == sqliteInMemory:
		return errPathf("url", "must not be :memory: for sqlite")
	case c.URL == "":
		return errPathf("url", "must not be empty")
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

func (c Chains) Validate() error {
	chainIDs := make(map[string]struct{})

	for i, chain := range c {
		if err := chain.Validate(); err != nil {
			return errPathIndex(i, err)
		}

		if _, ok := chainIDs[chain.ChainID]; ok {
			return errPathIndexf(i, "duplicate %q", chain.ChainID)
		}
		chainIDs[chain.ChainID] = struct{}{}
	}

	return nil
}

func (c ChainConfig) Validate() error {
	chainType := c.Type()

	switch {
	case c.ChainID == "":
		return errPathf("chainId", "required")
	case chainType == "":
		return fmt.Errorf("unknown chain type")
	case chainType != ChainTypeEVM:
		return fmt.Errorf("unsupported chain type %s", chainType)
	case chainType == ChainTypeEVM:
		// allow empty / zero ics26 router for general Validate()
		return errPath("evm", c.EVM.Validate(false))
	}

	return nil
}

// Type returns the chain type implied by the configured settings.
func (c ChainConfig) Type() ChainType {
	if c.EVM != nil {
		return ChainTypeEVM
	}

	return ""
}

func (c EVMChainConfig) Validate(validateICS26Router bool) error {
	switch {
	case c.RPC == "":
		return errPathf("rpc", "required")
	case c.WS != "" && !strings.HasPrefix(c.WS, "ws://") && !strings.HasPrefix(c.WS, "wss://"):
		return errPathf("ws", "must be a ws:// or wss:// URL, got %q", c.WS)
	case validateICS26Router:
		return errPath("ics26Router", c.validateICS26Router())
	default:
		return nil
	}
}

func (c EVMChainConfig) validateICS26Router() error {
	switch {
	case c.ICS26Router == "":
		return fmt.Errorf("required")
	case !common.IsHexAddress(c.ICS26Router):
		return fmt.Errorf("invalid EVM address %q", c.ICS26Router)
	case common.HexToAddress(c.ICS26Router) == (common.Address{}):
		return fmt.Errorf("must not be the zero address")
	default:
		return nil
	}
}

// Validate validates the attestors list. Allows empty.
func (a Attestors) Validate() error {
	localNames := make(map[string]struct{})
	// keyed by chainId+signer: the same signer backing one operator's local
	// attestor on two different chains is fine, but reusing it for two
	// attestors on the same chain is always a redundant duplicate.
	localChainSigners := make(map[string]struct{})
	for i, attestor := range a {
		if err := attestor.Validate(); err != nil {
			return errPathIndex(i, err)
		}

		if attestor.Type != AttestorTypeLocal {
			continue
		}

		if _, exists := localNames[attestor.Name]; exists {
			return errPathIndexf(i, "duplicate local attestor name: %q", attestor.Name)
		}
		localNames[attestor.Name] = struct{}{}

		chainSigner := attestor.ChainID + "/" + attestor.Signer
		if _, exists := localChainSigners[chainSigner]; exists {
			return errPathIndexf(i, "duplicate local attestor signer %q on chain %q", attestor.Signer, attestor.ChainID)
		}
		localChainSigners[chainSigner] = struct{}{}
	}

	return nil
}

func (c AttestorConfig) Validate() error {
	switch {
	case c.Name == "":
		return errPathf("name", "required")
	case c.Type != AttestorTypeLocal && c.Type != AttestorTypeRemote:
		return errPathf("type", "unknown attestor type: %q", c.Type)
	}

	switch c.Type {
	case AttestorTypeLocal:
		switch {
		case c.ChainID == "":
			return errPathf("chainId", "required for local attestors")
		case c.Signer == "":
			return errPathf("signer", "required for local attestors")
		case c.GRPC != "":
			return errPathf("grpc", "must not be set for local attestors")
		}
	case AttestorTypeRemote:
		switch {
		case c.GRPC == "":
			return errPathf("grpc", "required for remote attestors")
		case strings.Contains(c.GRPC, "://"):
			return errPathf("grpc", "must be a bare host:port, not a URL: %q", c.GRPC)
		case c.ChainID != "":
			return errPathf("chainId", "must not be set for remote attestors")
		case c.Signer != "":
			return errPathf("signer", "must not be set for remote attestors")
		case c.FinalityOffset != 0:
			return errPathf("finalityOffset", "must not be set for remote attestors")
		}
	}

	return nil
}

func (c Signers) Validate() error {
	set := make(map[string]struct{})

	for i, signer := range c {
		if err := signer.Validate(); err != nil {
			return errPathIndex(i, err)
		}

		if _, exists := set[signer.Alias]; exists {
			return errPathIndexf(i, "duplicate alias: %q", signer.Alias)
		}

		set[signer.Alias] = struct{}{}
	}

	return nil
}

func (c SignerConfig) Validate() error {
	switch {
	case c.Alias == "":
		return errPathf("alias", "required")
	case c.Type == "":
		return errPathf("type", "required")
	case c.Type != SignerLocal && c.Type != SignerRemote:
		return errPathf("type", "must be one of [%q, %q], got %q", SignerLocal, SignerRemote, c.Type)
	case c.Type == SignerLocal && c.File == "":
		return errPathf("file", "required for local signer")
	case c.Type == SignerRemote && c.GRPC == "":
		return errPathf("grpc", "required for remote signer")
	case c.Type == SignerRemote && c.RemoteKeyID == "":
		return errPathf("remoteKeyId", "required for remote signer")
	}

	if c.Type == SignerLocal {
		path, err := ExpandHome(c.File)
		if err != nil {
			return errPath("file", err)
		}

		fallbacks := KeyFileFallbacks(path)

		if err := fileExistsInAny(fallbacks...); err != nil {
			return errPath("file", err)
		}
	}

	return nil
}

// validateAutoRelay ensures every auto-relayed client end can be subscribed to.
// It lives here rather than on ChainConfig because a chain cannot see the
// connections that source from it.
func (c Config) validateAutoRelay() error {
	for i, conn := range c.Relayer.Connections {
		for _, side := range []struct {
			name string
			end  ClientEnd
		}{{"clientA", conn.ClientA}, {"clientB", conn.ClientB}} {
			end := side.end

			if end.AutoRelay.Enabled == nil || !*end.AutoRelay.Enabled {
				continue
			}

			chain, ok := c.Chain(end.ChainID)
			if !ok {
				continue
			}

			if chain.EVM == nil || chain.EVM.WS == "" {
				return errPathf(
					fmt.Sprintf("connections[%d].%s.autoRelay", i, side.name),
					"requires chains[%s].evm.ws", end.ChainID,
				)
			}
		}
	}

	return nil
}

func (c Config) crossValidate() error {
	signerSet := make(map[string]struct{}, len(c.Signers))
	for _, signer := range c.Signers {
		signerSet[signer.Alias] = struct{}{}
	}

	for i, a := range c.Attestors {
		if a.Type != AttestorTypeLocal {
			continue
		}
		if _, exists := signerSet[a.Signer]; !exists {
			seg := fmt.Sprintf("attestors[%d].signer", i)
			return errPathf(seg, "attestor %q references unknown signer %q", a.Name, a.Signer)
		}

		if _, exists := c.Chain(a.ChainID); !exists {
			seg := fmt.Sprintf("attestors[%d].chainId", i)
			return errPathf(seg, "attestor %q references unknown chain %q", a.Name, a.ChainID)
		}
	}

	for i, chain := range c.Chains {
		if chain.Deployer == "" {
			continue
		}
		if _, exists := signerSet[chain.Deployer]; !exists {
			seg := fmt.Sprintf("chains[%d].deployer", i)
			return errPathf(seg, "chain %q references unknown signer %q", chain.ChainID, chain.Deployer)
		}
	}

	if err := c.validateChainReferences(); err != nil {
		return err
	}

	return c.validateConnectionSigners(signerSet)
}

// validateChainReferences ensures chains referenced by the relayer config are
// declared in the top-level chains block.
func (c Config) validateChainReferences() error {
	for i, chain := range c.Relayer.ChainOverrides {
		if _, ok := c.Chain(chain.ChainID); chain.ChainID != "" && !ok {
			seg := fmt.Sprintf("relayer.chainOverrides[%d].chainId", i)
			return errPathf(seg, "%q not declared in top-level chains", chain.ChainID)
		}
	}

	for i, conn := range c.Relayer.Connections {
		for _, end := range connectionEnds(conn) {
			if _, ok := c.Chain(end.cfg.ChainID); end.cfg.ChainID != "" && !ok {
				seg := fmt.Sprintf("relayer.connections[%d].%s.chainId", i, end.label)
				return errPathf(seg, "%q not declared in top-level chains", end.cfg.ChainID)
			}
		}
	}

	return nil
}

// validateConnectionSigners ensures every client end's signer resolves to a
// configured signer.
func (c Config) validateConnectionSigners(signerSet map[string]struct{}) error {
	for i, conn := range c.Relayer.Connections {
		for _, end := range connectionEnds(conn) {
			if _, exists := signerSet[end.cfg.Signer]; !exists {
				seg := fmt.Sprintf("relayer.connections[%d].%s.signer", i, end.label)
				return errPathf(seg, "references unknown signer %q", end.cfg.Signer)
			}
		}
	}

	return nil
}

func (c Config) validateRunnable() error {
	switch {
	case len(c.Chains) == 0:
		return errPathf("chains", "no chains configured")
	case len(c.Signers) == 0:
		return errPathf("signers", "no signers configured")
	}

	// we allow empty / zero ics26 router for general Validate()
	// but require it for runnable validation
	for i, chain := range c.Chains {
		if chain.EVM == nil {
			continue
		}

		if err := chain.EVM.Validate(true); err != nil {
			return errPath(fmt.Sprintf("chains[%d].evm", i), err)
		}
	}

	return nil
}

// CollectComments builds TODO comments for every field in cfg that's left
// for the operator to fill in by hand, keyed by YAML path for
// PrintYAMLWithComments/StoreToFileWithComments.
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

// DBConfigFromURL infers DB type from a CLI database URL override.
func DBConfigFromURL(url string) (DBConfig, error) {
	db := DBConfig{
		URL:  url,
		Type: dbTypeFromURL(url),
	}

	return db, db.Validate()
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

func dbTypeFromURL(raw string) string {
	if strings.HasPrefix(raw, "postgres://") || strings.HasPrefix(raw, "postgresql://") {
		return DBTypePostgres
	}

	return DBTypeSQLite
}

// toCommentMap converts comments (YAML path -> text) into a yaml.CommentMap
// of line comments, as PrintYAMLWithComments/store both need.
func toCommentMap(comments map[string]string) yaml.CommentMap {
	cm := make(yaml.CommentMap, len(comments))
	for path, text := range comments {
		cm[path] = []*yaml.Comment{yaml.LineComment(" " + text)}
	}
	return cm
}

func connectionEnds(conn ConnectionConfig) []namedClientEnd {
	return []namedClientEnd{{"clientA", conn.ClientA}, {"clientB", conn.ClientB}}
}

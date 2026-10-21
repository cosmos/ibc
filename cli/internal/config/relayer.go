// SPDX-License-Identifier: Apache-2.0

package config

import (
	"fmt"
	"slices"
	"time"

	"github.com/goccy/go-yaml"
)

// Client types the IBC CLI proves itself.
const (
	ClientTypeAttestation = "attestation"
	ClientTypeBesuQBFT    = "besu-qbft"
)

// DefaultClearOnStart whether a clearing pass runs at startup when clearOnStart is unset.
const DefaultClearOnStart = true

// builtInClientTypes are the types the IBC CLI proves without a ProverService.
var builtInClientTypes = []string{ClientTypeAttestation, ClientTypeBesuQBFT}

// DefaultClearInterval how often a clearing pass runs when clearInterval is unset.
const DefaultClearInterval = 5 * time.Minute

// ClientParams is a client type's decoded params
type ClientParams interface {
	isClientParams()
	Validate() error
}

// RelayerConfig the relayer block of the config.
type RelayerConfig struct {
	DispatchPollInterval *time.Duration `yaml:"dispatchPollInterval,omitempty"`
	// ClearOnStart runs a clearing pass at startup. Unset runs it.
	ClearOnStart *bool `yaml:"clearOnStart,omitempty"`
	// ClearInterval is how often a clearing pass runs; a chainOverrides entry wins.
	ClearInterval  *time.Duration         `yaml:"clearInterval,omitempty"`
	ChainOverrides []RelayerChainOverride `yaml:"chainOverrides"`
	Connections    []ConnectionConfig     `yaml:"connections"`
}

// RelayerChainOverride relay settings for one chain.
type RelayerChainOverride struct {
	ChainID            string            `yaml:"chainId"`
	EVM                *RelayerEVMConfig `yaml:"evm,omitempty"`
	TxSubmissionDelay  *time.Duration    `yaml:"txSubmissionDelay,omitempty"`
	PacketBatchSize    *int              `yaml:"packetBatchSize,omitempty"`
	PacketBatchTimeout *time.Duration    `yaml:"packetBatchTimeout,omitempty"`

	// ClearInterval overrides relayer.clearInterval for packets sourced from this chain.
	ClearInterval *time.Duration `yaml:"clearInterval,omitempty"`

	// AbandonUnrecoverablePackets stops re-probing packets whose send log the
	// endpoint will not serve. They are remembered but never looked at again,
	// so turning it back off recovers them against an archive endpoint.
	AbandonUnrecoverablePackets *bool `yaml:"abandonUnrecoverablePackets,omitempty"`
}

// RelayerEVMConfig EVM relaying settings.
type RelayerEVMConfig struct {
	GasFeeCapMultiplier *float64 `yaml:"gasFeeCapMultiplier,omitempty"`
	GasTipCapMultiplier *float64 `yaml:"gasTipCapMultiplier,omitempty"`
}

// ConnectionConfig one bidirectional IBC connection the relayer actively
// relays, in both directions. ClientA's counterparty is simply ClientB (and vice versa).
type ConnectionConfig struct {
	Alias   string    `yaml:"alias"`
	ClientA ClientEnd `yaml:"clientA"`
	ClientB ClientEnd `yaml:"clientB"`
}

// ClientEnd one side of a connection: a light client on chainId,
// tracking the connection's other end as its counterparty
type ClientEnd struct {
	ChainID  string `yaml:"chainId"`
	Signer   string `yaml:"signer"`
	ClientID string `yaml:"clientId"`

	// Type is the on-chain light client type: `attestation` or `besu-qbft`,
	// or with a prover any type, including ones the IBC CLI does not implement.
	Type string `yaml:"type"`

	// Params is settings specific to Type: none yet for `attestation` and
	// `besu-qbft`, and not accepted for types the IBC CLI does not implement.
	Params yaml.RawMessage `yaml:"params,omitempty"`

	// Prover is a ProverService that generates this client's proofs instead of
	// the built-in prover for Type.
	Prover *ProverConfig `yaml:"prover,omitempty"`

	// AutoRelay configures auto-relay for packets flowing FROM this end's
	// chain TOWARD the counterparty end.
	AutoRelay AutoRelayConfig `yaml:"autoRelay,omitempty"`
}

// clientEndIdentity identifies a light client independent of aliases and
// operational settings.
type clientEndIdentity struct{ chainID, clientID string }

func (c ClientEnd) identity() clientEndIdentity {
	return clientEndIdentity{c.ChainID, c.ClientID}
}

// AutoRelayConfig automatic relaying settings.
type AutoRelayConfig struct {
	Enabled *bool `yaml:"enabled,omitempty"`
}

// ProverConfig is a ProverService endpoint.
type ProverConfig struct {
	// URL is the ProverService endpoint: an http(s):// URL, or a bare host:port
	// dialed as http://. Without a query or fragment.
	URL string `yaml:"url"`

	// TLS settings for an https:// URL, see TLSClientConfig.
	TLS *TLSClientConfig `yaml:"tls,omitempty"`
}

// AttestationParams is empty
type AttestationParams struct{}

// BesuQBFTParams is empty: the besu-qbft prover reads everything it needs from
// the two chains and the light client itself.
type BesuQBFTParams struct{}

// Validate validates the relayer config. Allows empty blocks.
func (c RelayerConfig) Validate() error {
	if c.DispatchPollInterval != nil && *c.DispatchPollInterval <= 0 {
		return errPathf("dispatchPollInterval", "must be positive")
	}

	if c.ClearInterval != nil && *c.ClearInterval <= 0 {
		return errPathf("clearInterval", "must be positive")
	}

	if err := c.validateChainOverrides(); err != nil {
		return err
	}

	return c.validateConnections()
}

// ChainOverride returns the relay settings override for a chain.
func (c RelayerConfig) ChainOverride(chainID string) (RelayerChainOverride, bool) {
	for _, override := range c.ChainOverrides {
		if override.ChainID == chainID {
			return override, true
		}
	}

	return RelayerChainOverride{}, false
}

// ClientEnd returns the client end matching (chainID, clientID) in any
// configured connection, along with its counterparty
func (c RelayerConfig) ClientEnd(chainID, clientID string) (end, counterparty ClientEnd, ok bool) {
	for _, conn := range c.Connections {
		switch {
		case conn.ClientA.ChainID == chainID && conn.ClientA.ClientID == clientID:
			return conn.ClientA, conn.ClientB, true
		case conn.ClientB.ChainID == chainID && conn.ClientB.ClientID == clientID:
			return conn.ClientB, conn.ClientA, true
		}
	}

	return ClientEnd{}, ClientEnd{}, false
}

// AutoRelayConnections returns the connections whose end on chainID has
// auto-relay enabled.
func (c RelayerConfig) AutoRelayConnections(chainID string) []ConnectionConfig {
	var connections []ConnectionConfig

	for _, conn := range c.Connections {
		source, _, ok := conn.SourceEnd(chainID)
		if !ok {
			continue
		}

		if source.AutoRelay.Enabled == nil || !*source.AutoRelay.Enabled {
			continue
		}

		connections = append(connections, conn)
	}

	return connections
}

func (c RelayerChainOverride) Validate() error {
	switch {
	case c.ChainID == "":
		return errPathf("chainId", "required")
	case c.TxSubmissionDelay != nil && *c.TxSubmissionDelay < 0:
		return errPathf("txSubmissionDelay", "must not be negative")
	case c.PacketBatchSize != nil && *c.PacketBatchSize <= 0:
		return errPathf("packetBatchSize", "must be positive")
	case c.PacketBatchTimeout != nil && *c.PacketBatchTimeout <= 0:
		return errPathf("packetBatchTimeout", "must be positive")
	case c.ClearInterval != nil && *c.ClearInterval <= 0:
		return errPathf("clearInterval", "must be positive")
	}

	if c.EVM != nil {
		if err := c.EVM.Validate(); err != nil {
			return errPath("evm", err)
		}
	}

	return nil
}

func (c RelayerEVMConfig) Validate() error {
	switch {
	case c.GasFeeCapMultiplier != nil && *c.GasFeeCapMultiplier <= 0:
		return errPathf("gasFeeCapMultiplier", "must be positive")
	case c.GasTipCapMultiplier != nil && *c.GasTipCapMultiplier <= 0:
		return errPathf("gasTipCapMultiplier", "must be positive")
	}

	return nil
}

func (c ConnectionConfig) Validate() error {
	if c.Alias == "" {
		return errPathf("alias", "required")
	}

	if err := c.ClientA.Validate(); err != nil {
		return errPath("clientA", err)
	}
	if err := c.ClientB.Validate(); err != nil {
		return errPath("clientB", err)
	}

	if c.ClientA.ChainID != "" && c.ClientA.ChainID == c.ClientB.ChainID {
		return fmt.Errorf("clientA and clientB must be on different chains")
	}

	return nil
}

// SourceEnd returns this connection's end on chainID, with the end its packets flow toward.
func (c ConnectionConfig) SourceEnd(chainID string) (source, destination ClientEnd, ok bool) {
	switch chainID {
	case c.ClientA.ChainID:
		return c.ClientA, c.ClientB, true
	case c.ClientB.ChainID:
		return c.ClientB, c.ClientA, true
	}

	return ClientEnd{}, ClientEnd{}, false
}

func (c ClientEnd) Validate() error {
	switch {
	case c.ChainID == "":
		return errPathf("chainId", "required")
	case c.ClientID == "":
		return errPathf("clientId", "required")
	case c.Signer == "":
		return errPathf("signer", "required")
	case c.Type == "":
		return errPathf("type", "required")
	case c.Prover == nil && !slices.Contains(builtInClientTypes, c.Type):
		return errPathf(
			"type", "unknown client type: %q; set prover for a light client the IBC CLI does not implement", c.Type,
		)
	}

	if err := c.validateParams(); err != nil {
		return errPath("params", err)
	}

	if c.Prover == nil {
		return nil
	}

	return errPath("prover", c.Prover.Validate())
}

func (c ClientEnd) validateParams() error {
	if !slices.Contains(builtInClientTypes, c.Type) {
		if len(c.Params) > 0 {
			return fmt.Errorf("client type %q takes no params", c.Type)
		}

		return nil
	}

	params, err := c.ClientParams()
	if err != nil {
		return err
	}

	return params.Validate()
}

// ClientParams decodes this client's params
func (c ClientEnd) ClientParams() (ClientParams, error) {
	switch c.Type {
	case ClientTypeAttestation:
		return decodeYAML[AttestationParams](c.Params)
	case ClientTypeBesuQBFT:
		return decodeYAML[BesuQBFTParams](c.Params)
	default:
		return nil, errPathf("type", "unknown client type: %q", c.Type)
	}
}

// EndpointURL returns URL as a URL, see endpointURL.
func (p ProverConfig) EndpointURL() string {
	return endpointURL(p.URL)
}

func (p ProverConfig) Validate() error {
	if p.URL == "" {
		return errPathf("url", "required")
	}

	if err := errPath("url", checkEndpointAddr(p.URL)); err != nil {
		return err
	}

	return validateEndpointTLS(p.URL, p.TLS)
}

func (AttestationParams) Validate() error { return nil }

func (BesuQBFTParams) Validate() error { return nil }

func (c RelayerConfig) validateChainOverrides() error {
	chainIDs := make(map[string]struct{})

	for i, chain := range c.ChainOverrides {
		seg := fmt.Sprintf("chainOverrides[%d]", i)

		if err := chain.Validate(); err != nil {
			return errPath(seg, err)
		}

		if _, ok := chainIDs[chain.ChainID]; ok {
			return errPathf(seg, "duplicate chainId: %q", chain.ChainID)
		}
		chainIDs[chain.ChainID] = struct{}{}
	}

	return nil
}

func (c RelayerConfig) validateConnections() error {
	for i, conn := range c.Connections {
		if err := conn.Validate(); err != nil {
			return errPath(fmt.Sprintf("connections[%d]", i), err)
		}
	}
	return c.validateConnectionIdentities()
}

func (c RelayerConfig) validateConnectionIdentities() error {
	aliases := make(map[string]struct{})
	clientEnds := make(map[clientEndIdentity]struct{})

	for i, conn := range c.Connections {
		seg := fmt.Sprintf("connections[%d]", i)

		if _, ok := aliases[conn.Alias]; ok {
			return errPathf(seg, "duplicate alias: %q", conn.Alias)
		}
		aliases[conn.Alias] = struct{}{}

		for _, end := range []ClientEnd{conn.ClientA, conn.ClientB} {
			key := end.identity()
			if _, ok := clientEnds[key]; ok {
				return errPathf(seg, "duplicate client %q on chain %q", end.ClientID, end.ChainID)
			}
			clientEnds[key] = struct{}{}
		}
	}

	return nil
}

func (AttestationParams) isClientParams() {}
func (BesuQBFTParams) isClientParams()    {}

func decodeYAML[T any](raw yaml.RawMessage) (*T, error) {
	var params T

	if len(raw) == 0 {
		return &params, nil
	}

	if err := yaml.UnmarshalWithOptions(raw, &params, yaml.DisallowUnknownField()); err != nil {
		return nil, err
	}

	return &params, nil
}

// ClearOnStartEnabled reports whether a clearing pass runs at startup, defaulting to true.
func (c RelayerConfig) ClearOnStartEnabled() bool {
	if c.ClearOnStart == nil {
		return DefaultClearOnStart
	}
	return *c.ClearOnStart
}

// ClearIntervalFor resolves the clearing cadence for a chain, preferring its chain override.
func (c RelayerConfig) ClearIntervalFor(chainID string) time.Duration {
	// per chain
	override, ok := c.ChainOverride(chainID)
	if ok && override.ClearInterval != nil {
		return *override.ClearInterval
	}

	// global
	if c.ClearInterval != nil {
		return *c.ClearInterval
	}

	// fallback
	return DefaultClearInterval
}

// AbandonUnrecoverablePacketsFor reports whether a chain stops re-probing
// packets whose send log no endpoint would serve. It defaults to false, which
// keeps them in the probe set until an endpoint serves them.
func (c RelayerConfig) AbandonUnrecoverablePacketsFor(chainID string) bool {
	override, ok := c.ChainOverride(chainID)
	if ok && override.AbandonUnrecoverablePackets != nil {
		return *override.AbandonUnrecoverablePackets
	}

	return false
}

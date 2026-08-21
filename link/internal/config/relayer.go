// SPDX-License-Identifier: Apache-2.0

package config

import (
	"time"

	"github.com/goccy/go-yaml"
	"github.com/pkg/errors"
)

// ClientType the light client type.
type ClientType string

// Client types
const (
	ClientTypeAttestation ClientType = "attestation"
	// ClientTypeRemoteProver delegates proof generation to a ProverService
	// over gRPC, which is how a custom light client is supported without
	// linking its code into the relayer.
	ClientTypeRemoteProver ClientType = "remoteProver"
)

// AttestorType how an attestor is reached.
type AttestorType string

// Attestor types
const (
	AttestorTypeRemote AttestorType = "remote"
	AttestorTypeLocal  AttestorType = "local"
)

// RelayerConfig the relayer block of the config.
type RelayerConfig struct {
	DispatchPollInterval *time.Duration         `yaml:"dispatchPollInterval,omitempty"`
	ChainOverrides       []RelayerChainOverride `yaml:"chainOverrides"`
	Connections          []ConnectionConfig     `yaml:"connections"`
}

// RelayerChainOverride relay settings for one chain.
type RelayerChainOverride struct {
	ChainID            string            `yaml:"chainId"`
	EVM                *RelayerEVMConfig `yaml:"evm,omitempty"`
	TxSubmissionDelay  *time.Duration    `yaml:"txSubmissionDelay,omitempty"`
	PacketBatchSize    *int              `yaml:"packetBatchSize,omitempty"`
	PacketBatchTimeout *time.Duration    `yaml:"packetBatchTimeout,omitempty"`
}

// RelayerEVMConfig EVM relaying settings.
type RelayerEVMConfig struct {
	GasFeeCapMultiplier *float64 `yaml:"gasFeeCapMultiplier,omitempty"`
	GasTipCapMultiplier *float64 `yaml:"gasTipCapMultiplier,omitempty"`
}

// ConnectionConfig one bidirectional IBC connection the relayer actively
// relays, in both directions. ClientA's counterparty is simply ClientB (and
// vice versa).
type ConnectionConfig struct {
	Alias   string    `yaml:"alias"`
	ClientA ClientEnd `yaml:"clientA"`
	ClientB ClientEnd `yaml:"clientB"`
}

// ClientEnd one side of a connection: a light client on chainId,
// tracking the connection's other end as its counterparty
type ClientEnd struct {
	ChainID  string     `yaml:"chainId"`
	Signer   string     `yaml:"signer"`
	ClientID string     `yaml:"clientId"`
	Type     ClientType `yaml:"type"`

	// Params carries the settings this client type needs, captured without
	// being interpreted. Keeping them here rather than as fields on ClientEnd
	// means a new client type adds nothing to every client.
	Params yaml.RawMessage `yaml:"params,omitempty"`

	// AutoRelay configures auto-relay for packets flowing FROM this end's
	// chain TOWARD the counterparty end.
	AutoRelay AutoRelayConfig `yaml:"autoRelay,omitempty"`
}

// ErrUnknownClientType marks a type this build does not implement. Parsing
// tolerates it so a config still being assembled loads; Validate reports it.
var ErrUnknownClientType = errors.New("unknown client type")

// ClientParams is a client type's decoded params. The unexported method seals
// the interface, so only the types below satisfy it. Each type owns both its
// schema and its rules.
type ClientParams interface {
	isClientParams()
	Validate() error
}

// AttestationParams is empty: an attestation client is described entirely by
// its on-chain attestor set. Declaring it keeps every type in one switch.
type AttestationParams struct{}

func (*AttestationParams) isClientParams() {}

// Validate reports nothing: an attestation client declares no params.
func (*AttestationParams) Validate() error { return nil }

// RemoteProverParams is the params block a remoteProver client declares.
type RemoteProverParams struct {
	// URL is the ProverService endpoint.
	URL string `yaml:"url"`
}

func (*RemoteProverParams) isClientParams() {}

func (p *RemoteProverParams) Validate() error {
	if p.URL == "" {
		return errors.New(".params.url required")
	}

	return nil
}

// ClientParams decodes this client's params for its type, without applying the
// type's rules. One accessor covers every type, so a new type adds a case here
// rather than a method on ClientEnd.
func (c ClientEnd) ClientParams() (ClientParams, error) {
	switch c.Type {
	case ClientTypeAttestation:
		// Strict decoding rejects a params block on a type that takes none.
		return decodeParams[AttestationParams](c.Params)
	case ClientTypeRemoteProver:
		return decodeParams[RemoteProverParams](c.Params)
	default:
		return nil, errors.Wrapf(ErrUnknownClientType, ".type %q", c.Type)
	}
}

// decodeParams reads a params block strictly: a misspelled key is an error,
// not a silently absent value.
func decodeParams[T any](raw yaml.RawMessage) (*T, error) {
	var params T

	if len(raw) == 0 {
		return &params, nil
	}

	if err := yaml.UnmarshalWithOptions(raw, &params, yaml.DisallowUnknownField()); err != nil {
		return nil, errors.Wrap(err, "decoding params")
	}

	return &params, nil
}

// UnmarshalYAML reads the fields, then checks that the params block decodes
// for the declared type, so a malformed block fails the load rather than the
// first read. An unknown type is left for Validate to report, so a config
// still being assembled loads.
func (c *ClientEnd) UnmarshalYAML(data []byte) error {
	// plain drops the methods, so unmarshalling it does not re-enter here.
	type plain ClientEnd

	var raw plain
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return err
	}

	*c = ClientEnd(raw)

	if _, err := c.ClientParams(); err != nil && !errors.Is(err, ErrUnknownClientType) {
		return err
	}

	return nil
}

// AutoRelayConfig automatic relaying settings.
type AutoRelayConfig struct {
	Enabled *bool `yaml:"enabled,omitempty"`
	// Lookback the number of blocks the relayer looks back from the latest
	// block to check for packets to relay.
	Lookback uint64 `yaml:"lookback,omitempty"`
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

// Validate validates the relayer config. Allows empty blocks.
func (c RelayerConfig) Validate() error {
	if c.DispatchPollInterval != nil && *c.DispatchPollInterval <= 0 {
		return errors.New(".dispatchPollInterval must be positive")
	}
	if err := c.validateChainOverrides(); err != nil {
		return err
	}

	return c.validateConnections()
}

func (c RelayerConfig) validateChainOverrides() error {
	chainIDs := make(map[string]struct{})

	for _, chain := range c.ChainOverrides {
		if err := chain.Validate(); err != nil {
			return errors.Wrapf(err, ".chainOverrides[%s]", chain.ChainID)
		}

		if _, ok := chainIDs[chain.ChainID]; ok {
			return errors.Errorf(".chainOverrides duplicate chainId: %q", chain.ChainID)
		}
		chainIDs[chain.ChainID] = struct{}{}
	}

	return nil
}

func (c RelayerConfig) validateConnections() error {
	aliases := make(map[string]struct{})
	clientEnds := make(map[string]struct{})

	for _, conn := range c.Connections {
		if err := conn.Validate(); err != nil {
			return errors.Wrapf(err, ".connections[%s]", conn.Alias)
		}

		if _, ok := aliases[conn.Alias]; ok {
			return errors.Errorf(".connections duplicate alias: %q", conn.Alias)
		}
		aliases[conn.Alias] = struct{}{}

		for _, end := range []ClientEnd{conn.ClientA, conn.ClientB} {
			key := end.ChainID + "/" + end.ClientID
			if _, ok := clientEnds[key]; ok {
				return errors.Errorf(".connections duplicate client %q on chain %q", end.ClientID, end.ChainID)
			}
			clientEnds[key] = struct{}{}
		}
	}

	return nil
}

func (c ConnectionConfig) Validate() error {
	if c.Alias == "" {
		return errors.New(".alias required")
	}

	if err := c.ClientA.Validate(); err != nil {
		return errors.Wrap(err, ".clientA")
	}
	if err := c.ClientB.Validate(); err != nil {
		return errors.Wrap(err, ".clientB")
	}

	if c.ClientA.ChainID != "" && c.ClientA.ChainID == c.ClientB.ChainID {
		return errors.New(".clientA and .clientB must be on different chains")
	}

	return nil
}

func (c ClientEnd) Validate() error {
	switch {
	case c.ChainID == "":
		return errors.New(".chainId required")
	case c.ClientID == "":
		return errors.New(".clientId required")
	case c.Signer == "":
		return errors.New(".signer required")
	case c.Type != ClientTypeAttestation && c.Type != ClientTypeRemoteProver:
		return errors.Errorf(".type unknown client type: %q", c.Type)
	}

	params, err := c.ClientParams()
	if err != nil {
		return err
	}

	return params.Validate()
}

func (c RelayerChainOverride) Validate() error {
	switch {
	case c.ChainID == "":
		return errors.New(".chainId required")
	case c.TxSubmissionDelay != nil && *c.TxSubmissionDelay < 0:
		return errors.New(".txSubmissionDelay must not be negative")
	case c.PacketBatchSize != nil && *c.PacketBatchSize <= 0:
		return errors.New(".packetBatchSize must be positive")
	case c.PacketBatchTimeout != nil && *c.PacketBatchTimeout <= 0:
		return errors.New(".packetBatchTimeout must be positive")
	}

	if c.EVM != nil {
		if err := c.EVM.Validate(); err != nil {
			return errors.Wrap(err, ".evm")
		}
	}

	return nil
}

func (c RelayerEVMConfig) Validate() error {
	switch {
	case c.GasFeeCapMultiplier != nil && *c.GasFeeCapMultiplier <= 0:
		return errors.New(".gasFeeCapMultiplier must be positive")
	case c.GasTipCapMultiplier != nil && *c.GasTipCapMultiplier <= 0:
		return errors.New(".gasTipCapMultiplier must be positive")
	}

	return nil
}

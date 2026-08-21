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

// RemoteProverParams is the params block a remoteProver client declares. It
// lives here while there is one custom client type; a second type would move
// each schema next to the code that implements it.
type RemoteProverParams struct {
	// URL is the ProverService endpoint.
	URL string `yaml:"url"`
}

// RemoteProverParams decodes and validates this client's params.
func (c ClientEnd) RemoteProverParams() (RemoteProverParams, error) {
	var params RemoteProverParams

	if len(c.Params) > 0 {
		// Strict: a misspelled key must not read as an absent one.
		if err := yaml.UnmarshalWithOptions(c.Params, &params, yaml.DisallowUnknownField()); err != nil {
			return RemoteProverParams{}, errors.Wrap(err, "decoding params")
		}
	}

	if params.URL == "" {
		return RemoteProverParams{}, errors.New(".params.url required")
	}

	return params, nil
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

	if c.Type == ClientTypeRemoteProver {
		if _, err := c.RemoteProverParams(); err != nil {
			return err
		}
	}

	return nil
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

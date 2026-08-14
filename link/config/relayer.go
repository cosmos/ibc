// SPDX-License-Identifier: Apache-2.0

package config

import "time"

// ClientType the light client type.
type ClientType string

// Client types
const (
	ClientTypeAttestation ClientType = "attestation"
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

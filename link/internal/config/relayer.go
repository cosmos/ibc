package config

import (
	"time"

	"github.com/pkg/errors"
)

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
	ChainOverrides []RelayerChainOverride `yaml:"chainOverrides"`
	Clients        []ClientConfig         `yaml:"clients"`
	Routes         []RouteConfig          `yaml:"routesToRelay"`
	// Signers the signer alias used to submit relay transactions per chain id.
	Signers map[string]string `yaml:"signers,omitempty"`
}

// ChainOverride returns the relay settings override for a chain, or nil.
func (c RelayerConfig) ChainOverride(chainID string) *RelayerChainOverride {
	for i := range c.ChainOverrides {
		if c.ChainOverrides[i].ChainID == chainID {
			return &c.ChainOverrides[i]
		}
	}

	return nil
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

// ClientConfig a light client on a chain.
type ClientConfig struct {
	// Alias unique config-level handle referenced by routes.
	Alias                string             `yaml:"alias"`
	ClientID             string             `yaml:"clientId"`
	ChainID              string             `yaml:"chainId"`
	CounterpartyChainID  string             `yaml:"counterpartyChainId"`
	CounterpartyClientID string             `yaml:"counterpartyClientId"`
	Type                 ClientType         `yaml:"type"`
	AttestorSet          *AttestorSetConfig `yaml:"attestorSet,omitempty"`
}

// AttestorSetConfig attestation policy for a client.
type AttestorSetConfig struct {
	CounterpartyChainFinalityOffset uint64          `yaml:"counterpartyChainFinalityOffset"`
	Threshold                       int             `yaml:"threshold"`
	Attestors                       []AttestorEntry `yaml:"attestors"`
}

// AttestorEntry an attestor the relayer queries for packet and state attestations.
type AttestorEntry struct {
	Name string       `yaml:"name"`
	Type AttestorType `yaml:"type"`
	GRPC string       `yaml:"grpc,omitempty"`
}

// RouteConfig packets sent from the source client are relayed through the
// entire packet lifecycle: recv, ack, timeout.
type RouteConfig struct {
	SourceClient string          `yaml:"sourceClient"`
	AutoRelay    AutoRelayConfig `yaml:"autoRelay,omitempty"`
}

// AutoRelayConfig automatic relaying settings.
type AutoRelayConfig struct {
	Enabled *bool `yaml:"enabled,omitempty"`
	// Lookback the number of blocks the relayer looks back from the latest
	// block to check for packets to relay.
	Lookback uint64 `yaml:"lookback,omitempty"`
}

// ClientByAlias returns the client config for the given alias.
func (c RelayerConfig) ClientByAlias(alias string) (ClientConfig, bool) {
	for _, client := range c.Clients {
		if client.Alias == alias {
			return client, true
		}
	}

	return ClientConfig{}, false
}

func (c RelayerConfig) Client(chainID, clientID string) (ClientConfig, bool) {
	for _, client := range c.Clients {
		if client.ChainID == chainID && client.ClientID == clientID {
			return client, true
		}
	}

	return ClientConfig{}, false
}

// Validate validates the relayer config. Allows empty blocks.
func (c RelayerConfig) Validate() error {
	if err := c.validateChainOverrides(); err != nil {
		return err
	}

	if err := c.validateClients(); err != nil {
		return err
	}

	return c.validateRoutes()
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

func (c RelayerConfig) validateClients() error {
	clients := make(map[string]struct{})
	aliases := make(map[string]struct{})

	for _, client := range c.Clients {
		if err := client.Validate(); err != nil {
			return errors.Wrapf(err, ".clients[%s]", client.Alias)
		}

		if _, ok := aliases[client.Alias]; ok {
			return errors.Errorf(".clients duplicate alias: %q", client.Alias)
		}
		aliases[client.Alias] = struct{}{}

		key := client.ChainID + "/" + client.ClientID
		if _, ok := clients[key]; ok {
			return errors.Errorf(".clients duplicate client %q on chain %q", client.ClientID, client.ChainID)
		}
		clients[key] = struct{}{}
	}

	for _, client := range c.Clients {
		if err := c.validateCounterparty(client); err != nil {
			return err
		}
	}

	return nil
}

// validateCounterparty ensures both sides of a connection are configured for bi-directional relaying.
func (c RelayerConfig) validateCounterparty(client ClientConfig) error {
	counterparty, ok := c.Client(client.CounterpartyChainID, client.CounterpartyClientID)
	if !ok {
		return errors.Errorf(
			".clients[%s] counterparty client %q on chain %q must also be configured for bi-directional relaying",
			client.Alias, client.CounterpartyClientID, client.CounterpartyChainID,
		)
	}

	if counterparty.CounterpartyChainID != client.ChainID || counterparty.CounterpartyClientID != client.ClientID {
		return errors.Errorf(
			".clients[%s] counterparty client %q does not reference it back",
			client.Alias, counterparty.Alias,
		)
	}

	return nil
}

func (c RelayerConfig) validateRoutes() error {
	routes := make(map[string]struct{})

	for i, route := range c.Routes {
		if err := route.Validate(); err != nil {
			return errors.Wrapf(err, ".routesToRelay[%d]", i)
		}

		if _, ok := c.ClientByAlias(route.SourceClient); !ok {
			return errors.Errorf(".routesToRelay[%d] references unknown client %q", i, route.SourceClient)
		}

		if _, ok := routes[route.SourceClient]; ok {
			return errors.Errorf(".routesToRelay duplicate route for client %q", route.SourceClient)
		}
		routes[route.SourceClient] = struct{}{}
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

func (c ClientConfig) Validate() error {
	switch {
	case c.Alias == "":
		return errors.New(".alias required")
	case c.ClientID == "":
		return errors.New(".clientId required")
	case c.ChainID == "":
		return errors.New(".chainId required")
	case c.Type != ClientTypeAttestation:
		return errors.Errorf(".type unknown client type: %q", c.Type)
	case c.CounterpartyChainID == "":
		return errors.New(".counterpartyChainId required")
	case c.CounterpartyClientID == "":
		return errors.New(".counterpartyClientId required")
	}

	if c.Type == ClientTypeAttestation {
		if c.AttestorSet == nil {
			return errors.Errorf(".attestorSet required for %s clients", ClientTypeAttestation)
		}

		if err := c.AttestorSet.Validate(); err != nil {
			return errors.Wrap(err, ".attestorSet")
		}
	}

	return nil
}

func (c AttestorSetConfig) Validate() error {
	if c.Threshold < 1 {
		return errors.New(".threshold must be at least 1")
	}

	if c.Threshold > len(c.Attestors) {
		return errors.Errorf(".threshold %d exceeds number of attestors %d", c.Threshold, len(c.Attestors))
	}

	seen := make(map[AttestorEntry]struct{})

	for i, attestor := range c.Attestors {
		if err := attestor.Validate(); err != nil {
			return errors.Wrapf(err, ".attestors[%d]", i)
		}

		if _, ok := seen[attestor]; ok {
			return errors.Errorf(".attestors duplicate entry: name %q", attestor.Name)
		}
		seen[attestor] = struct{}{}
	}

	return nil
}

func (c AttestorEntry) Validate() error {
	switch {
	case c.Name == "":
		return errors.New(".name required")
	case c.Type != AttestorTypeRemote && c.Type != AttestorTypeLocal:
		return errors.Errorf(".type unknown attestor type: %q", c.Type)
	case c.Type == AttestorTypeRemote && c.GRPC == "":
		return errors.New(".grpc required for remote attestors")
	}

	return nil
}

func (c RouteConfig) Validate() error {
	if c.SourceClient == "" {
		return errors.New(".sourceClient required")
	}

	return nil
}

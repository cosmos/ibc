package config

import (
	"math/big"
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
	Chains    []RelayerChainConfig `yaml:"chains"`
	Clients   []ClientConfig       `yaml:"clients"`
	Attestors []AttestorEntry      `yaml:"attestors"`
	Routes    []RouteConfig        `yaml:"routesToRelay"`
}

// RelayerChainConfig relaying settings for one chain.
type RelayerChainConfig struct {
	ChainID            string              `yaml:"chainId"`
	EVM                *RelayerEVMConfig   `yaml:"evm,omitempty"`
	GasAlertThresholds *GasAlertThresholds `yaml:"gasAlertThresholds,omitempty"`
	PacketBatchSize    int                 `yaml:"packetBatchSize"`
	PacketBatchTimeout time.Duration       `yaml:"packetBatchTimeout"`
}

// Type returns the chain type implied by the configured settings.
func (c RelayerChainConfig) Type() ChainType {
	if c.EVM != nil {
		return ChainTypeEVM
	}

	return ""
}

// RelayerEVMConfig EVM relaying settings.
type RelayerEVMConfig struct {
	Contracts           EVMContracts  `yaml:"contracts"`
	TxSubmissionDelay   time.Duration `yaml:"txSubmissionDelay"`
	GasFeeCapMultiplier *float64      `yaml:"gasFeeCapMultiplier,omitempty"`
	GasTipCapMultiplier *float64      `yaml:"gasTipCapMultiplier,omitempty"`
}

// EVMContracts IBC contract addresses.
type EVMContracts struct {
	ICS26Router string `yaml:"ics26Router"`
}

// GasAlertThresholds gas balances that trigger low-balance metrics.
type GasAlertThresholds struct {
	WarningThreshold  string `yaml:"warningThreshold"`
	CriticalThreshold string `yaml:"criticalThreshold"`
}

// ClientConfig a light client on a chain.
type ClientConfig struct {
	// Alias unique config-level handle referenced by attestors and routes.
	Alias               string             `yaml:"alias"`
	ClientID            string             `yaml:"clientId"`
	ChainID             string             `yaml:"chainId"`
	CounterpartyChainID string             `yaml:"counterpartyChainId"`
	Type                ClientType         `yaml:"type"`
	AttestorSet         *AttestorSetConfig `yaml:"attestorSet,omitempty"`
}

// AttestorSetConfig attestation policy for a client. The set's attestors are
// the entries in the top-level attestors block associated with the client.
type AttestorSetConfig struct {
	CounterpartyChainFinalityOffset uint64 `yaml:"counterpartyChainFinalityOffset"`
	Threshold                       int    `yaml:"threshold"`
}

// AttestorEntry an attestor the relayer queries for the associated client.
type AttestorEntry struct {
	Alias  string       `yaml:"alias"`
	Type   AttestorType `yaml:"type"`
	GRPC   string       `yaml:"grpc,omitempty"`
	Client string       `yaml:"client"`
}

// RouteConfig packets sent from the source client are relayed through the
// entire packet lifecycle: recv, ack, timeout.
type RouteConfig struct {
	SourceClient string          `yaml:"sourceClient"`
	AutoRelay    AutoRelayConfig `yaml:"autoRelay,omitempty"`
}

// AutoRelayConfig automatic relaying settings.
type AutoRelayConfig struct {
	// Enabled defaults to true when omitted.
	Enabled  *bool  `yaml:"enabled,omitempty"`
	Lookback uint64 `yaml:"lookback,omitempty"`
}

func (c AutoRelayConfig) IsEnabled() bool {
	return c.Enabled == nil || *c.Enabled
}

func (c RelayerConfig) Chain(chainID string) (RelayerChainConfig, bool) {
	for _, chain := range c.Chains {
		if chain.ChainID == chainID {
			return chain, true
		}
	}

	return RelayerChainConfig{}, false
}

// ClientAttestors returns the attestors associated with a client.
func (c RelayerConfig) ClientAttestors(clientAlias string) []AttestorEntry {
	var attestors []AttestorEntry

	for _, attestor := range c.Attestors {
		if attestor.Client == clientAlias {
			attestors = append(attestors, attestor)
		}
	}

	return attestors
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
	if err := c.validateChains(); err != nil {
		return err
	}

	if err := c.validateClients(); err != nil {
		return err
	}

	if err := c.validateAttestors(); err != nil {
		return err
	}

	return c.validateRoutes()
}

func (c RelayerConfig) validateChains() error {
	chainIDs := make(map[string]struct{})

	for _, chain := range c.Chains {
		if err := chain.Validate(); err != nil {
			return errors.Wrapf(err, ".chains[%s]", chain.ChainID)
		}

		if _, ok := chainIDs[chain.ChainID]; ok {
			return errors.Errorf(".chains duplicate chainId: %q", chain.ChainID)
		}
		chainIDs[chain.ChainID] = struct{}{}
	}

	return nil
}

func (c RelayerConfig) validateAttestors() error {
	aliases := make(map[string]struct{})

	for i, attestor := range c.Attestors {
		if err := attestor.Validate(); err != nil {
			return errors.Wrapf(err, ".attestors[%d]", i)
		}

		if _, ok := c.ClientByAlias(attestor.Client); !ok {
			return errors.Errorf(".attestors[%s] references unknown client %q", attestor.Alias, attestor.Client)
		}

		if _, ok := aliases[attestor.Alias]; ok {
			return errors.Errorf(".attestors duplicate alias: %q", attestor.Alias)
		}
		aliases[attestor.Alias] = struct{}{}
	}

	for _, client := range c.Clients {
		if client.Type != ClientTypeAttestation {
			continue
		}

		count := len(c.ClientAttestors(client.Alias))
		if client.AttestorSet.Threshold > count {
			return errors.Errorf(
				".clients[%s].attestorSet threshold %d exceeds number of attestors %d",
				client.Alias, client.AttestorSet.Threshold, count,
			)
		}
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

		if _, ok := c.Chain(client.ChainID); !ok {
			return errors.Errorf(".clients[%s] chainId %q not configured in .chains", client.Alias, client.ChainID)
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
	if _, ok := c.Chain(client.CounterpartyChainID); !ok {
		return errors.Errorf(
			".clients[%s] counterparty chain %q must also be configured for bi-directional relaying",
			client.ClientID, client.CounterpartyChainID,
		)
	}

	for _, counterpartyClient := range c.Clients {
		if counterpartyClient.ChainID == client.CounterpartyChainID &&
			counterpartyClient.CounterpartyChainID == client.ChainID {
			return nil
		}
	}

	return errors.Errorf(
		".clients[%s] counterparty chain %q must configure a client with counterpartyChainId %q for bi-directional relaying",
		client.Alias,
		client.CounterpartyChainID,
		client.ChainID,
	)
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

func (c RelayerChainConfig) Validate() error {
	switch {
	case c.ChainID == "":
		return errors.New(".chainId required")
	case c.PacketBatchSize < 0:
		return errors.New(".packetBatchSize must not be negative")
	case c.PacketBatchTimeout < 0:
		return errors.New(".packetBatchTimeout must not be negative")
	}

	if c.EVM != nil {
		if err := c.EVM.Validate(); err != nil {
			return errors.Wrap(err, ".evm")
		}
	}

	if c.GasAlertThresholds != nil {
		if err := c.GasAlertThresholds.Validate(); err != nil {
			return errors.Wrap(err, ".gasAlertThresholds")
		}
	}

	return nil
}

func (c RelayerEVMConfig) Validate() error {
	switch {
	case c.Contracts.ICS26Router == "":
		return errors.New(".contracts.ics26Router required")
	case c.TxSubmissionDelay < 0:
		return errors.New(".txSubmissionDelay must not be negative")
	case c.GasFeeCapMultiplier != nil && *c.GasFeeCapMultiplier <= 0:
		return errors.New(".gasFeeCapMultiplier must be positive")
	case c.GasTipCapMultiplier != nil && *c.GasTipCapMultiplier <= 0:
		return errors.New(".gasTipCapMultiplier must be positive")
	}

	return nil
}

func (c GasAlertThresholds) Validate() error {
	for name, value := range map[string]string{
		".warningThreshold":  c.WarningThreshold,
		".criticalThreshold": c.CriticalThreshold,
	} {
		if value == "" {
			continue
		}

		amount, ok := new(big.Int).SetString(value, 10)
		if !ok || amount.Sign() < 0 {
			return errors.Errorf("%s must be a non-negative integer, got %q", name, value)
		}
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
		return errors.Errorf(".type must be %q, got %q", ClientTypeAttestation, c.Type)
	case c.CounterpartyChainID == "":
		return errors.New(".counterpartyChainId required")
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

	return nil
}

func (c AttestorEntry) Validate() error {
	switch {
	case c.Alias == "":
		return errors.New(".alias required")
	case c.Client == "":
		return errors.New(".client required")
	case c.Type != AttestorTypeRemote && c.Type != AttestorTypeLocal:
		return errors.Errorf(".type must be one of [%q, %q], got %q", AttestorTypeRemote, AttestorTypeLocal, c.Type)
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

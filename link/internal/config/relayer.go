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
	Chains []RelayerChainConfig `yaml:"chains"`
}

// RelayerChainConfig relaying settings for one chain.
type RelayerChainConfig struct {
	ChainID            string              `yaml:"chainId"`
	EVM                *RelayerEVMConfig   `yaml:"evm,omitempty"`
	GasAlertThresholds *GasAlertThresholds `yaml:"gasAlertThresholds,omitempty"`
	Clients            []ClientConfig      `yaml:"clients"`
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

// ClientConfig a source client to relay transfers from.
type ClientConfig struct {
	ID                  string             `yaml:"id"`
	Type                ClientType         `yaml:"type"`
	AttestorSet         *AttestorSetConfig `yaml:"attestorSet,omitempty"`
	CounterpartyChainID string             `yaml:"counterpartyChainId"`
	AutoRelay           AutoRelayConfig    `yaml:"autoRelay,omitempty"`
}

// AttestorSetConfig the attestor set backing a client.
type AttestorSetConfig struct {
	CounterpartyChainFinalityOffset uint64          `yaml:"counterpartyChainFinalityOffset"`
	Threshold                       int             `yaml:"threshold"`
	Attestors                       []AttestorEntry `yaml:"attestors"`
}

// AttestorEntry an attestor in the set.
type AttestorEntry struct {
	Type  AttestorType `yaml:"type"`
	GRPC  string       `yaml:"grpc,omitempty"`
	Alias string       `yaml:"alias"`
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

func (c RelayerConfig) Client(chainID, clientID string) (ClientConfig, bool) {
	chain, ok := c.Chain(chainID)
	if !ok {
		return ClientConfig{}, false
	}

	for _, client := range chain.Clients {
		if client.ID == clientID {
			return client, true
		}
	}

	return ClientConfig{}, false
}

func (c RelayerConfig) Validate() error {
	chainIDs := make(map[string]struct{})
	aliases := make(map[string]struct{})

	for _, chain := range c.Chains {
		if err := chain.Validate(); err != nil {
			return errors.Wrapf(err, ".chains[%s]", chain.ChainID)
		}

		if _, ok := chainIDs[chain.ChainID]; ok {
			return errors.Errorf(".chains duplicate chainId: %q", chain.ChainID)
		}
		chainIDs[chain.ChainID] = struct{}{}

		for _, client := range chain.Clients {
			if client.AttestorSet == nil {
				continue
			}

			for _, attestor := range client.AttestorSet.Attestors {
				if _, ok := aliases[attestor.Alias]; ok {
					return errors.Errorf(".chains[%s] duplicate attestor alias: %q", chain.ChainID, attestor.Alias)
				}
				aliases[attestor.Alias] = struct{}{}
			}
		}
	}

	for _, chain := range c.Chains {
		for _, client := range chain.Clients {
			if err := c.validateCounterparty(chain.ChainID, client); err != nil {
				return err
			}
		}
	}

	return nil
}

// validateCounterparty ensures both sides of a connection are configured for bi-directional relaying.
func (c RelayerConfig) validateCounterparty(chainID string, client ClientConfig) error {
	counterparty, ok := c.Chain(client.CounterpartyChainID)
	if !ok {
		return errors.Errorf(
			".chains[%s].clients[%s] counterparty chain %q must also be configured for bi-directional relaying",
			chainID, client.ID, client.CounterpartyChainID,
		)
	}

	for _, counterpartyClient := range counterparty.Clients {
		if counterpartyClient.CounterpartyChainID == chainID {
			return nil
		}
	}

	return errors.Errorf(
		".chains[%s].clients[%s] counterparty chain %q must configure a client with counterpartyChainId %q for bi-directional relaying",
		chainID,
		client.ID,
		client.CounterpartyChainID,
		chainID,
	)
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

	clientIDs := make(map[string]struct{})
	for _, client := range c.Clients {
		if err := client.Validate(); err != nil {
			return errors.Wrapf(err, ".clients[%s]", client.ID)
		}

		if _, ok := clientIDs[client.ID]; ok {
			return errors.Errorf(".clients duplicate id: %q", client.ID)
		}
		clientIDs[client.ID] = struct{}{}
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
	case c.ID == "":
		return errors.New(".id required")
	case c.Type != ClientTypeAttestation:
		return errors.Errorf(".type must be %q, got %q", ClientTypeAttestation, c.Type)
	case c.CounterpartyChainID == "":
		return errors.New(".counterpartyChainId required")
	case c.AttestorSet == nil:
		return errors.Errorf(".attestorSet required for %s clients", ClientTypeAttestation)
	}

	if err := c.AttestorSet.Validate(); err != nil {
		return errors.Wrap(err, ".attestorSet")
	}

	return nil
}

func (c AttestorSetConfig) Validate() error {
	switch {
	case c.Threshold < 1:
		return errors.New(".threshold must be at least 1")
	case c.Threshold > len(c.Attestors):
		return errors.Errorf(".threshold %d exceeds number of attestors %d", c.Threshold, len(c.Attestors))
	}

	for _, attestor := range c.Attestors {
		if err := attestor.Validate(); err != nil {
			return errors.Wrapf(err, ".attestors[%s]", attestor.Alias)
		}
	}

	return nil
}

func (c AttestorEntry) Validate() error {
	switch {
	case c.Alias == "":
		return errors.New(".alias required")
	case c.Type != AttestorTypeRemote && c.Type != AttestorTypeLocal:
		return errors.Errorf(".type must be one of [%q, %q], got %q", AttestorTypeRemote, AttestorTypeLocal, c.Type)
	case c.Type == AttestorTypeRemote && c.GRPC == "":
		return errors.New(".grpc required for remote attestors")
	}

	return nil
}

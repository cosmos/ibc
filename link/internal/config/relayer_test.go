package config

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const fullRelayerConfig = `
server:
  listenAddr: 0.0.0.0:3000
db:
  type: sqlite
  url: ibc.db
chains:
  - chainId: "1"
    evm:
      rpc: https://ethereum-rpc.example.com
  - chainId: "8453"
    evm:
      rpc: https://base-rpc.example.com
relayer:
  chains:
    - chainId: "1"
      evm:
        contracts:
          ics26Router: "0xe20BccD900Fa1B48f46F5a483d9De063b07eDFCC"
        txSubmissionDelay: 2s
        gasFeeCapMultiplier: 1.5
        gasTipCapMultiplier: 1.5
      gasAlertThresholds:
        warningThreshold: "500000000"
        criticalThreshold: "10000000"
      clients:
        - id: "base-0"
          type: "attestation"
          counterpartyChainId: "8453"
          attestorSet:
            counterpartyChainFinalityOffset: 1
            threshold: 2
            attestors:
              - type: remote
                grpc: attestor-alice.example.com:3000
                alias: "attestor-alice-base"
              - type: remote
                grpc: attestor-bob.example.com:3000
                alias: "attestor-bob-base"
              - type: local
                alias: "attestor-dan-base"
          autoRelay:
            enabled: false
            lookback: 100
      packetBatchSize: 20
      packetBatchTimeout: 10s
    - chainId: "8453"
      clients:
        - id: "ethereum-0"
          type: "attestation"
          counterpartyChainId: "1"
          attestorSet:
            threshold: 1
            attestors:
              - type: local
                alias: "attestor-dan-ethereum"
`

func TestRelayerConfig(t *testing.T) {
	t.Run("LoadFullShape", func(t *testing.T) {
		// ARRANGE
		path := writeTestConfig(t, fullRelayerConfig)

		// ACT
		config, err := LoadFromFile(path, true, true)

		// ASSERT
		require.NoError(t, err)

		require.Len(t, config.Chains, 2)
		assert.Equal(t, "https://ethereum-rpc.example.com", config.Chains[0].EVM.RPC)

		require.Len(t, config.Relayer.Chains, 2)
		chain := config.Relayer.Chains[0]
		assert.Equal(t, "0xe20BccD900Fa1B48f46F5a483d9De063b07eDFCC", chain.EVM.Contracts.ICS26Router)
		assert.Equal(t, 2*time.Second, chain.EVM.TxSubmissionDelay)
		assert.Equal(t, 1.5, *chain.EVM.GasFeeCapMultiplier)
		assert.Equal(t, "500000000", chain.GasAlertThresholds.WarningThreshold)
		assert.Equal(t, 20, chain.PacketBatchSize)
		assert.Equal(t, 10*time.Second, chain.PacketBatchTimeout)

		require.Len(t, chain.Clients, 1)
		client := chain.Clients[0]
		assert.Equal(t, "base-0", client.ID)
		assert.Equal(t, ClientTypeAttestation, client.Type)
		assert.Equal(t, "8453", client.CounterpartyChainID)
		assert.False(t, client.AutoRelay.IsEnabled())
		assert.Equal(t, uint64(100), client.AutoRelay.Lookback)

		require.NotNil(t, client.AttestorSet)
		assert.Equal(t, uint64(1), client.AttestorSet.CounterpartyChainFinalityOffset)
		assert.Equal(t, 2, client.AttestorSet.Threshold)
		require.Len(t, client.AttestorSet.Attestors, 3)
		assert.Equal(t, AttestorTypeLocal, client.AttestorSet.Attestors[2].Type)

		// autoRelay omitted -> enabled by default
		assert.True(t, config.Relayer.Chains[1].Clients[0].AutoRelay.IsEnabled())
	})

	t.Run("Helpers", func(t *testing.T) {
		// ARRANGE
		path := writeTestConfig(t, fullRelayerConfig)
		config, err := LoadFromFile(path, true, true)
		require.NoError(t, err)

		// ACT / ASSERT
		_, ok := config.Chain("1")
		assert.True(t, ok)

		_, ok = config.Chain("999")
		assert.False(t, ok)

		client, ok := config.Relayer.Client("1", "base-0")
		assert.True(t, ok)
		assert.Equal(t, "8453", client.CounterpartyChainID)

		_, ok = config.Relayer.Client("1", "unknown-0")
		assert.False(t, ok)

		_, ok = config.Relayer.Client("999", "base-0")
		assert.False(t, ok)
	})

	t.Run("Validate", func(t *testing.T) {
		for _, tt := range []struct {
			name        string
			patch       func(c *Config)
			errContains string
		}{
			{
				name:  "empty blocks are valid",
				patch: func(c *Config) { c.Chains = nil; c.Relayer.Chains = nil },
			},
			{
				name: "chain missing chainId",
				patch: func(c *Config) {
					c.Chains[0].ChainID = ""
				},
				errContains: ".chainId required",
			},
			{
				name: "chain missing evm",
				patch: func(c *Config) {
					c.Chains[0].EVM = nil
				},
				errContains: ".evm required",
			},
			{
				name: "chain missing rpc",
				patch: func(c *Config) {
					c.Chains[0].EVM.RPC = ""
				},
				errContains: ".evm.rpc required",
			},
			{
				name: "duplicate top-level chainId",
				patch: func(c *Config) {
					c.Chains[1].ChainID = "1"
				},
				errContains: "duplicate chainId",
			},
			{
				name: "duplicate relayer chainId",
				patch: func(c *Config) {
					c.Relayer.Chains[1].ChainID = "1"
				},
				errContains: "duplicate chainId",
			},
			{
				name: "relayer chain not declared",
				patch: func(c *Config) {
					c.Relayer.Chains[0].ChainID = "43"
				},
				errContains: "not declared in top-level chains",
			},
			{
				name: "counterparty chain not declared",
				patch: func(c *Config) {
					c.Relayer.Chains[0].Clients[0].CounterpartyChainID = "999"
				},
				errContains: `counterpartyChainId "999" not declared`,
			},
			{
				name: "counterparty chain not configured in relayer",
				patch: func(c *Config) {
					c.Relayer.Chains = c.Relayer.Chains[:1]
				},
				errContains: `counterparty chain "8453" must also be configured for bi-directional relaying`,
			},
			{
				name: "counterparty chain has no client back",
				patch: func(c *Config) {
					c.Relayer.Chains[1].Clients = nil
				},
				errContains: `counterparty chain "8453" must configure a client with counterpartyChainId "1"`,
			},
			{
				name: "negative batch size",
				patch: func(c *Config) {
					c.Relayer.Chains[0].PacketBatchSize = -1
				},
				errContains: ".packetBatchSize must not be negative",
			},
			{
				name: "missing router contract",
				patch: func(c *Config) {
					c.Relayer.Chains[0].EVM.Contracts.ICS26Router = ""
				},
				errContains: ".contracts.ics26Router required",
			},
			{
				name: "zero gas multiplier",
				patch: func(c *Config) {
					zero := 0.0
					c.Relayer.Chains[0].EVM.GasFeeCapMultiplier = &zero
				},
				errContains: ".gasFeeCapMultiplier must be positive",
			},
			{
				name: "non-numeric gas threshold",
				patch: func(c *Config) {
					c.Relayer.Chains[0].GasAlertThresholds.WarningThreshold = "lots"
				},
				errContains: ".warningThreshold must be a non-negative integer",
			},
			{
				name: "empty gas thresholds are valid",
				patch: func(c *Config) {
					c.Relayer.Chains[0].GasAlertThresholds = &GasAlertThresholds{}
				},
			},
			{
				name: "client missing id",
				patch: func(c *Config) {
					c.Relayer.Chains[0].Clients[0].ID = ""
				},
				errContains: ".id required",
			},
			{
				name: "unsupported client type",
				patch: func(c *Config) {
					c.Relayer.Chains[0].Clients[0].Type = "tendermint"
				},
				errContains: `.type must be "attestation"`,
			},
			{
				name: "duplicate client id",
				patch: func(c *Config) {
					chain := &c.Relayer.Chains[0]
					chain.Clients = append(chain.Clients, chain.Clients[0])
				},
				errContains: ".clients duplicate id",
			},
			{
				name: "missing attestor set",
				patch: func(c *Config) {
					c.Relayer.Chains[0].Clients[0].AttestorSet = nil
				},
				errContains: `.attestorSet required for attestation clients`,
			},
			{
				name: "threshold exceeds attestors",
				patch: func(c *Config) {
					c.Relayer.Chains[0].Clients[0].AttestorSet.Threshold = 4
				},
				errContains: ".threshold 4 exceeds number of attestors 3",
			},
			{
				name: "zero threshold",
				patch: func(c *Config) {
					c.Relayer.Chains[0].Clients[0].AttestorSet.Threshold = 0
				},
				errContains: ".threshold must be at least 1",
			},
			{
				name: "remote attestor missing grpc",
				patch: func(c *Config) {
					c.Relayer.Chains[0].Clients[0].AttestorSet.Attestors[0].GRPC = ""
				},
				errContains: ".grpc required for remote attestors",
			},
			{
				name: "invalid attestor type",
				patch: func(c *Config) {
					c.Relayer.Chains[0].Clients[0].AttestorSet.Attestors[0].Type = "hybrid"
				},
				errContains: `.type must be one of ["remote", "local"]`,
			},
			{
				name: "attestor missing alias",
				patch: func(c *Config) {
					c.Relayer.Chains[0].Clients[0].AttestorSet.Attestors[0].Alias = ""
				},
				errContains: ".alias required",
			},
			{
				name: "duplicate attestor alias across chains",
				patch: func(c *Config) {
					c.Relayer.Chains[1].Clients[0].AttestorSet = &AttestorSetConfig{
						Threshold: 1,
						Attestors: []AttestorEntry{
							{Type: AttestorTypeLocal, Alias: "attestor-alice-base"},
						},
					}
				},
				errContains: "duplicate attestor alias",
			},
		} {
			t.Run(tt.name, func(t *testing.T) {
				// ARRANGE
				path := writeTestConfig(t, fullRelayerConfig)
				config, err := LoadFromFile(path, true, true)
				require.NoError(t, err)

				tt.patch(&config)

				// ACT
				err = config.Validate()

				// ASSERT
				if tt.errContains != "" {
					require.ErrorContains(t, err, tt.errContains)
					return
				}

				require.NoError(t, err)
			})
		}
	})

	t.Run("SampleConfigParsesStrictly", func(t *testing.T) {
		// ARRANGE / ACT
		config, err := LoadFromFile("ibc.yml", true, true)

		// ASSERT
		require.NoError(t, err)
		require.NoError(t, config.Validate())
	})
}

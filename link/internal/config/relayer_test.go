package config

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRelayerConfig(t *testing.T) {
	t.Run("LoadFullShape", func(t *testing.T) {
		// ARRANGE
		path := filepath.Join("testdata", "sample.yml")

		// ACT
		config, err := LoadFromFile(path, true, true)

		// ASSERT
		require.NoError(t, err)

		require.Len(t, config.Chains, 2)
		assert.Equal(t, "https://ethereum-rpc.example.com", config.Chains[0].EVM.RPC)
		assert.Equal(t, ChainTypeEVM, config.Chains[0].Type())

		require.Len(t, config.Relayer.ChainOverrides, 2)
		chain := config.Relayer.ChainOverrides[0]
		assert.Equal(t, "0xe20BccD900Fa1B48f46F5a483d9De063b07eDFCC", config.Chains[0].EVM.ICS26Router)
		assert.Equal(t, 2*time.Second, *chain.TxSubmissionDelay)
		assert.Equal(t, 1.5, *chain.EVM.GasFeeCapMultiplier)
		assert.Equal(t, 20, *chain.PacketBatchSize)
		assert.Equal(t, 10*time.Second, *chain.PacketBatchTimeout)

		require.Len(t, config.Relayer.Clients, 2)
		client := config.Relayer.Clients[0]
		assert.Equal(t, "eth-to-base", client.Alias)
		assert.Equal(t, "base-0", client.ClientID)
		assert.Equal(t, "1", client.ChainID)
		assert.Equal(t, "8453", client.CounterpartyChainID)
		assert.Equal(t, "ethereum-0", client.CounterpartyClientID)
		assert.Equal(t, ClientTypeAttestation, client.Type)

		attestors := client.AttestorSet.Attestors
		require.Len(t, attestors, 3)
		assert.Equal(t, AttestorTypeRemote, attestors[0].Type)
		assert.Equal(t, "attestor-alice-base", attestors[0].Name)

		require.NotNil(t, client.AttestorSet)
		assert.Equal(t, uint64(1), client.AttestorSet.CounterpartyChainFinalityOffset)
		assert.Equal(t, 2, client.AttestorSet.Threshold)

		require.Len(t, config.Relayer.Routes, 2)
		route := config.Relayer.Routes[0]
		assert.Equal(t, "eth-to-base", route.SourceClient)
		assert.False(t, *route.AutoRelay.Enabled)
		assert.Equal(t, uint64(100), route.AutoRelay.Lookback)

		// autoRelay omitted -> unset
		assert.Nil(t, config.Relayer.Routes[1].AutoRelay.Enabled)
	})

	t.Run("Helpers", func(t *testing.T) {
		// ARRANGE
		path := filepath.Join("testdata", "sample.yml")
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
				name: "empty blocks are valid",
				patch: func(c *Config) {
					c.Chains = nil
					c.Relayer = RelayerConfig{}
				},
			},
			{
				name: "chain missing chainId",
				patch: func(c *Config) {
					c.Chains[0].ChainID = ""
				},
				errContains: ".chainId required",
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
					c.Relayer.ChainOverrides[1].ChainID = "1"
				},
				errContains: "duplicate chainId",
			},
			{
				name: "relayer chain not declared",
				patch: func(c *Config) {
					c.Relayer.ChainOverrides[0].ChainID = "43"
				},
				errContains: "not declared in top-level chains",
			},
			{
				name: "client chain not declared",
				patch: func(c *Config) {
					c.Relayer.Clients[0].ChainID = "999"
					c.Relayer.Clients[1].CounterpartyChainID = "999"
				},
				errContains: `.clients[base-0] chainId "999" not declared`,
			},
			{
				name: "counterparty chain not declared",
				patch: func(c *Config) {
					c.Relayer.Clients[0].CounterpartyChainID = "999"
				},
				errContains: `counterparty client "ethereum-0" on chain "999" must also be configured`,
			},
			{
				name: "client missing counterpartyClientId",
				patch: func(c *Config) {
					c.Relayer.Clients[0].CounterpartyClientID = ""
				},
				errContains: ".counterpartyClientId required",
			},
			{
				name: "counterparty client not configured",
				patch: func(c *Config) {
					c.Relayer.Clients = c.Relayer.Clients[:1]
					c.Relayer.Routes = c.Relayer.Routes[:1]
				},
				errContains: `counterparty client "ethereum-0" on chain "8453" must also be configured`,
			},
			{
				name: "counterparty client does not reference back",
				patch: func(c *Config) {
					c.Relayer.Clients[1].CounterpartyClientID = "other-0"
				},
				errContains: `counterparty client "base-to-eth" does not reference it back`,
			},
			{
				name: "non-positive batch size",
				patch: func(c *Config) {
					size := 0
					c.Relayer.ChainOverrides[0].PacketBatchSize = &size
				},
				errContains: ".packetBatchSize must be positive",
			},
			{
				name: "negative tx submission delay",
				patch: func(c *Config) {
					delay := -time.Second
					c.Relayer.ChainOverrides[0].TxSubmissionDelay = &delay
				},
				errContains: ".txSubmissionDelay must not be negative",
			},
			{
				name: "missing router contract",
				patch: func(c *Config) {
					c.Chains[0].EVM.ICS26Router = ""
				},
				errContains: ".evm.ics26Router required",
			},
			{
				name: "zero gas multiplier",
				patch: func(c *Config) {
					zero := 0.0
					c.Relayer.ChainOverrides[0].EVM.GasFeeCapMultiplier = &zero
				},
				errContains: ".gasFeeCapMultiplier must be positive",
			},
			{
				name: "client missing clientId",
				patch: func(c *Config) {
					c.Relayer.Clients[0].ClientID = ""
				},
				errContains: ".clientId required",
			},
			{
				name: "client missing chainId",
				patch: func(c *Config) {
					c.Relayer.Clients[0].ChainID = ""
				},
				errContains: ".chainId required",
			},
			{
				name: "unsupported client type",
				patch: func(c *Config) {
					c.Relayer.Clients[0].Type = "tendermint"
				},
				errContains: `unknown client type: "tendermint"`,
			},
			{
				name: "duplicate client",
				patch: func(c *Config) {
					duplicate := c.Relayer.Clients[0]
					duplicate.Alias = "eth-to-base-2"
					c.Relayer.Clients = append(c.Relayer.Clients, duplicate)
				},
				errContains: `.clients duplicate client "base-0" on chain "1"`,
			},
			{
				name: "client without attestor set",
				patch: func(c *Config) {
					c.Relayer.Clients[0].AttestorSet = nil
				},
				errContains: `.attestorSet required for attestation clients`,
			},
			{
				name: "client missing alias",
				patch: func(c *Config) {
					c.Relayer.Clients[0].Alias = ""
				},
				errContains: ".alias required",
			},
			{
				name: "duplicate client alias",
				patch: func(c *Config) {
					c.Relayer.Clients[1].Alias = "eth-to-base"
				},
				errContains: `.clients duplicate alias`,
			},
			{
				name: "threshold exceeds attestors",
				patch: func(c *Config) {
					c.Relayer.Clients[0].AttestorSet.Threshold = 4
				},
				errContains: `threshold 4 exceeds number of attestors 3`,
			},
			{
				name: "zero threshold",
				patch: func(c *Config) {
					c.Relayer.Clients[0].AttestorSet.Threshold = 0
				},
				errContains: ".threshold must be at least 1",
			},
			{
				name: "remote attestor missing grpc",
				patch: func(c *Config) {
					c.Relayer.Clients[0].AttestorSet.Attestors[0].GRPC = ""
				},
				errContains: ".grpc required for remote attestors",
			},
			{
				name: "invalid attestor type",
				patch: func(c *Config) {
					c.Relayer.Clients[0].AttestorSet.Attestors[0].Type = "hybrid"
				},
				errContains: `unknown attestor type: "hybrid"`,
			},
			{
				name: "attestor missing name",
				patch: func(c *Config) {
					c.Relayer.Clients[0].AttestorSet.Attestors[0].Name = ""
				},
				errContains: ".name required",
			},
			{
				name: "duplicate attestor name routed to a different grpc endpoint",
				patch: func(c *Config) {
					c.Relayer.Clients[0].AttestorSet.Attestors[1].Name = "attestor-alice-base"
				},
			},
			{
				name: "duplicate attestor entry",
				patch: func(c *Config) {
					c.Relayer.Clients[0].AttestorSet.Attestors[1] = c.Relayer.Clients[0].AttestorSet.Attestors[0]
				},
				errContains: ".attestors duplicate entry",
			},
			{
				name: "relayer signer for undeclared chain",
				patch: func(c *Config) {
					c.Relayer.Signers["999"] = "relayer-key"
				},
				errContains: `.relayer.signers: chainId "999" not declared`,
			},
			{
				name: "relayer signer with unknown alias",
				patch: func(c *Config) {
					c.Relayer.Signers["1"] = "ghost"
				},
				errContains: `references unknown signer "ghost"`,
			},
			{
				name: "route chain missing signer",
				patch: func(c *Config) {
					delete(c.Relayer.Signers, "8453")
				},
				errContains: `missing signer for chain "8453"`,
			},
			{
				name: "routes require proof api",
				patch: func(c *Config) {
					c.Relayer.ProofAPI.GRPC = ""
				},
				errContains: "proof api grpc address required",
			},
			{
				name: "route missing sourceClient",
				patch: func(c *Config) {
					c.Relayer.Routes[0].SourceClient = ""
				},
				errContains: ".sourceClient required",
			},
			{
				name: "route references unknown client",
				patch: func(c *Config) {
					c.Relayer.Routes[0].SourceClient = "unknown"
				},
				errContains: `references unknown client "unknown"`,
			},
			{
				name: "duplicate route",
				patch: func(c *Config) {
					c.Relayer.Routes = append(c.Relayer.Routes, c.Relayer.Routes[0])
				},
				errContains: ".routesToRelay duplicate route",
			},
		} {
			t.Run(tt.name, func(t *testing.T) {
				// ARRANGE
				path := filepath.Join("testdata", "sample.yml")
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

}

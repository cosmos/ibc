// SPDX-License-Identifier: Apache-2.0

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

		assert.Equal(t, 3*time.Second, *config.Relayer.DispatchPollInterval)
		require.Len(t, config.Relayer.ChainOverrides, 2)
		chain := config.Relayer.ChainOverrides[0]
		assert.Equal(t, "0xe20BccD900Fa1B48f46F5a483d9De063b07eDFCC", config.Chains[0].EVM.ICS26Router)
		assert.Equal(t, 2*time.Second, *chain.TxSubmissionDelay)
		//nolint:testifylint // exact literal from the fixture; a tolerance would mask decoding drift
		assert.Equal(t, 1.5, *chain.EVM.GasFeeCapMultiplier)
		assert.Equal(t, 20, *chain.PacketBatchSize)
		assert.Equal(t, 10*time.Second, *chain.PacketBatchTimeout)

		require.Len(t, config.Relayer.Connections, 1)
		conn := config.Relayer.Connections[0]
		assert.Equal(t, "eth-base", conn.Alias)

		clientA := conn.ClientA
		assert.Equal(t, "1", clientA.ChainID)
		assert.Equal(t, "base-0", clientA.ClientID)
		assert.Equal(t, "relayer-key", clientA.Signer)
		assert.Equal(t, ClientTypeAttestation, clientA.Type)

		assert.False(t, *clientA.AutoRelay.Enabled)
		assert.Equal(t, uint64(100), clientA.AutoRelay.Lookback)

		clientB := conn.ClientB
		assert.Equal(t, "8453", clientB.ChainID)
		assert.Equal(t, "ethereum-0", clientB.ClientID)

		// autoRelay omitted -> unset
		assert.Nil(t, clientB.AutoRelay.Enabled)
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

		end, counterparty, ok := config.Relayer.ClientEnd("1", "base-0")
		assert.True(t, ok)
		assert.Equal(t, "1", end.ChainID)
		assert.Equal(t, "8453", counterparty.ChainID)
		assert.Equal(t, "ethereum-0", counterparty.ClientID)

		_, _, ok = config.Relayer.ClientEnd("1", "unknown-0")
		assert.False(t, ok)

		_, _, ok = config.Relayer.ClientEnd("999", "base-0")
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
					c.Relayer.Connections[0].ClientA.ChainID = "999"
				},
				errContains: `.connections[eth-base].clientA chainId "999" not declared`,
			},
			{
				name: "clientA and clientB on same chain",
				patch: func(c *Config) {
					c.Relayer.Connections[0].ClientB.ChainID = c.Relayer.Connections[0].ClientA.ChainID
				},
				errContains: ".clientA and .clientB must be on different chains",
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
				name: "non-positive dispatch poll interval",
				patch: func(c *Config) {
					interval := time.Duration(0)
					c.Relayer.DispatchPollInterval = &interval
				},
				errContains: ".dispatchPollInterval must be positive",
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
				name: "missing router contract is allowed -- it's a deploy output, not operator input",
				patch: func(c *Config) {
					c.Chains[0].EVM.ICS26Router = ""
				},
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
					c.Relayer.Connections[0].ClientA.ClientID = ""
				},
				errContains: ".clientId required",
			},
			{
				name: "client missing chainId",
				patch: func(c *Config) {
					c.Relayer.Connections[0].ClientA.ChainID = ""
				},
				errContains: ".chainId required",
			},
			{
				// An unregistered type is not a structural config error. It is
				// resolved when the relayer constructs proof generators.
				name: "unregistered client type passes structural validation",
				patch: func(c *Config) {
					c.Relayer.Connections[0].ClientA.Type = "tendermint"
				},
			},
			{
				name: "missing client type",
				patch: func(c *Config) {
					c.Relayer.Connections[0].ClientA.Type = ""
				},
				errContains: ".type required",
			},
			{
				name: "duplicate client",
				patch: func(c *Config) {
					duplicate := ConnectionConfig{
						Alias:   "eth-base-2",
						ClientA: c.Relayer.Connections[0].ClientA,
						ClientB: c.Relayer.Connections[0].ClientB,
					}
					duplicate.ClientB.ChainID = "999"
					duplicate.ClientB.ClientID = "ethereum-1"
					c.Relayer.Connections = append(c.Relayer.Connections, duplicate)
				},
				errContains: `.connections duplicate client "base-0" on chain "1"`,
			},
			{
				name: "connection missing alias",
				patch: func(c *Config) {
					c.Relayer.Connections[0].Alias = ""
				},
				errContains: ".alias required",
			},
			{
				name: "duplicate connection alias",
				patch: func(c *Config) {
					duplicate := c.Relayer.Connections[0]
					duplicate.ClientA.ClientID = "base-1"
					duplicate.ClientB.ClientID = "ethereum-1"
					c.Relayer.Connections = append(c.Relayer.Connections, duplicate)
				},
				errContains: `.connections duplicate alias: "eth-base"`,
			},
			{
				name: "clientA signer unknown",
				patch: func(c *Config) {
					c.Relayer.Connections[0].ClientA.Signer = "ghost"
				},
				errContains: `references unknown signer "ghost"`,
			},
			{
				name: "clientB signer unknown",
				patch: func(c *Config) {
					c.Relayer.Connections[0].ClientB.Signer = "ghost"
				},
				errContains: `references unknown signer "ghost"`,
			},
			{
				name: "clientA missing signer",
				patch: func(c *Config) {
					c.Relayer.Connections[0].ClientA.Signer = ""
				},
				errContains: ".signer required",
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

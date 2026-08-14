// SPDX-License-Identifier: Apache-2.0

package ibclink

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	linkconfig "github.com/cosmos/ibc/link/config"
)

func TestBuildRelayerConfigPreservesDefaultYAML(t *testing.T) {
	file, err := buildRelayerFileConfig(testRelayerConfig())
	require.NoError(t, err)
	data, err := linkconfig.MarshalYAML(file)
	require.NoError(t, err)
	require.Equal(t, `server:
  listenAddr: 127.0.0.1:0
db:
  type: sqlite
  url: /tmp/ibc.db
chains:
- chainId: "1"
  evm:
    rpc: http://chain-1
    ics26Router: router-1
- chainId: "2"
  evm:
    rpc: http://chain-2
    ics26Router: router-2
relayer:
  dispatchPollInterval: 100ms
  chainOverrides:
  - chainId: "1"
    txSubmissionDelay: 10ms
    packetBatchSize: 1
  - chainId: "2"
    txSubmissionDelay: 10ms
    packetBatchSize: 1
  connections:
  - alias: client-1-client-2
    clientA:
      chainId: "1"
      signer: tx
      clientId: client-1
      type: attestation
    clientB:
      chainId: "2"
      signer: tx
      clientId: client-2
      type: attestation
attestors:
- chainId: "1"
  name: local-attestor-1
  type: local
  signer: local-attestor-1-signer
  finalityOffset: 3
- chainId: "2"
  name: local-attestor-2
  type: local
  signer: local-attestor-2-signer
  finalityOffset: 3
signers:
- alias: tx
  type: local
  file: /tmp/default.key
- alias: local-attestor-1-signer
  type: local
  file: /tmp/default.key
- alias: local-attestor-2-signer
  type: local
  file: /tmp/default.key
`, string(data))
}

func TestBuildRelayerConfigOverrides(t *testing.T) {
	cfg := testRelayerConfig()
	cfg.SignerType = linkconfig.SignerRemote
	cfg.SignerKeyFile = ""
	cfg.SignerGRPC = "kms:9090"
	cfg.SignerRemoteKeyID = "relay-key"
	cfg.Chains[0].PacketBatchSize = 7
	cfg.Chains[0].PacketBatchTimeout = 250 * time.Millisecond
	cfg.Attestors = []RelayerAttestor{
		{Name: "alice", Type: linkconfig.AttestorTypeLocal, ChainID: "2", KeyFile: "/tmp/alice.key"},
		{Name: "bob", Type: linkconfig.AttestorTypeRemote, GRPC: "bob:8080"},
		{Name: "carol", Type: linkconfig.AttestorTypeRemote, GRPC: "carol:8080"},
	}

	file, err := buildRelayerFileConfig(cfg)
	require.NoError(t, err)
	override := file.Relayer.ChainOverrides[0]
	require.Equal(t, "1", override.ChainID)
	require.Equal(t, 10*time.Millisecond, *override.TxSubmissionDelay)
	require.Equal(t, 7, *override.PacketBatchSize)
	require.Equal(t, 250*time.Millisecond, *override.PacketBatchTimeout)
	require.Equal(t, linkconfig.Signers{
		{Alias: "tx", Type: linkconfig.SignerRemote, GRPC: "kms:9090", RemoteKeyID: "relay-key"},
		{Alias: "alice-signer", Type: linkconfig.SignerLocal, File: "/tmp/alice.key"},
	}, file.Signers)
	require.Equal(t, linkconfig.Attestors{
		{Name: "alice", ChainID: "2", Type: linkconfig.AttestorTypeLocal, Signer: "alice-signer", FinalityOffset: 3},
		{Name: "bob", Type: linkconfig.AttestorTypeRemote, GRPC: "bob:8080"},
		{Name: "carol", Type: linkconfig.AttestorTypeRemote, GRPC: "carol:8080"},
	}, file.Attestors)
	require.Equal(t, cfg.SignerAlias, file.Relayer.Connections[0].ClientA.Signer)
	require.Equal(t, cfg.SignerAlias, file.Relayer.Connections[0].ClientB.Signer)
}

func TestRemoteSignerBacksDefaultLocalAttestors(t *testing.T) {
	cfg := testRelayerConfig()
	cfg.SignerType = linkconfig.SignerRemote
	cfg.SignerKeyFile = ""
	cfg.SignerGRPC = "kms:9090"
	cfg.SignerRemoteKeyID = "relay-key"

	file, err := buildRelayerFileConfig(cfg)
	require.NoError(t, err)
	require.Equal(t, linkconfig.Signers{
		{Alias: "tx", Type: linkconfig.SignerRemote, GRPC: "kms:9090", RemoteKeyID: "relay-key"},
		{
			Alias: "local-attestor-1-signer", Type: linkconfig.SignerRemote,
			GRPC: "kms:9090", RemoteKeyID: "relay-key",
		},
		{
			Alias: "local-attestor-2-signer", Type: linkconfig.SignerRemote,
			GRPC: "kms:9090", RemoteKeyID: "relay-key",
		},
	}, file.Signers)
}

func TestExplicitAttestorsSuppressDefaultLocalAttestors(t *testing.T) {
	cfg := testRelayerConfig()
	cfg.Attestors = []RelayerAttestor{{
		Name: "remote", Type: linkconfig.AttestorTypeRemote, GRPC: "attestor:8080",
	}}

	file, err := buildRelayerFileConfig(cfg)
	require.NoError(t, err)
	// Explicit attestors suppress defaults for every chain, not just the ones referenced.
	require.Equal(t, linkconfig.Attestors{
		{Name: "remote", Type: linkconfig.AttestorTypeRemote, GRPC: "attestor:8080"},
	}, file.Attestors)
	require.Equal(t, linkconfig.Signers{
		{Alias: "tx", Type: linkconfig.SignerLocal, File: "/tmp/default.key"},
	}, file.Signers)
}

func TestBuildRelayerConfigRejectsHarnessInvalidConfig(t *testing.T) {
	tests := []struct {
		name string
		edit func(*RelayerConfig)
		err  string
	}{
		{"signer key", func(c *RelayerConfig) { c.SignerKeyFile = "" }, "signer key file is required"},
		{"attestor key required", func(c *RelayerConfig) {
			c.Attestors = []RelayerAttestor{{Name: "a", Type: linkconfig.AttestorTypeLocal, ChainID: "1"}}
		}, "key file is required for local attestors"},
		{"attestor chainId required", func(c *RelayerConfig) {
			c.Attestors = []RelayerAttestor{{Name: "a", Type: linkconfig.AttestorTypeLocal, KeyFile: "key"}}
		}, "chainId is required for local attestors"},
		{"unsupported attestor type", func(c *RelayerConfig) {
			c.Attestors = []RelayerAttestor{{Name: "a", Type: "hybrid"}}
		}, `unsupported attestor type "hybrid"`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := testRelayerConfig()
			tt.edit(&cfg)
			_, err := buildRelayerFileConfig(cfg)
			require.ErrorContains(t, err, tt.err)
		})
	}
}

func testRelayerConfig() RelayerConfig {
	return RelayerConfig{
		DBPath: "/tmp/ibc.db", SignerAlias: "tx", SignerKeyFile: "/tmp/default.key", FinalityOffset: 3,
		Chains: []RelayerChain{
			{ChainID: "1", RPC: "http://chain-1", ICS26Router: "router-1"},
			{ChainID: "2", RPC: "http://chain-2", ICS26Router: "router-2"},
		},
		Connections: []RelayerConnection{{
			ChainA: "1", ClientA: "client-1", ChainB: "2", ClientB: "client-2",
		}},
	}
}

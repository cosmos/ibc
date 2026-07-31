package ibclink

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func TestBuildRelayerConfigPreservesDefaultYAML(t *testing.T) {
	file, err := buildRelayerFileConfig(testRelayerConfig())
	require.NoError(t, err)
	data, err := yaml.Marshal(file)
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
attestor:
    attestations:
        - chainId: "1"
          name: local-attestor-1
          signer: local-attestor-1-signer
          finalityOffset: 3
        - chainId: "2"
          name: local-attestor-2
          signer: local-attestor-2-signer
          finalityOffset: 3
relayer:
    dispatchPollInterval: 100ms
    chainOverrides:
        - chainId: "1"
          txSubmissionDelay: 10ms
          packetBatchSize: 1
        - chainId: "2"
          txSubmissionDelay: 10ms
          packetBatchSize: 1
    clients:
        - alias: client-1
          clientId: client-1
          chainId: "1"
          counterpartyChainId: "2"
          counterpartyClientId: client-2
          type: attestation
          attestorSet:
            counterpartyChainFinalityOffset: 3
            threshold: 1
            attestors:
                - name: local-attestor-2
                  type: local
        - alias: client-2
          clientId: client-2
          chainId: "2"
          counterpartyChainId: "1"
          counterpartyClientId: client-1
          type: attestation
          attestorSet:
            counterpartyChainFinalityOffset: 3
            threshold: 1
            attestors:
                - name: local-attestor-1
                  type: local
    routesToRelay:
        - sourceClient: client-1
          sourceSignerAlias: tx
          destSignerAlias: tx
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
	cfg.SignerType = RelayerSignerRemote
	cfg.SignerKeyFile = ""
	cfg.SignerGRPC = "kms:9090"
	cfg.SignerRemoteKeyID = "relay-key"
	cfg.Chains[0].PacketBatchSize = 7
	cfg.Chains[0].PacketBatchTimeout = 250 * time.Millisecond
	cfg.Connections[0].AttestorSetA = &RelayerAttestorSet{
		Threshold: 2,
		Attestors: []RelayerAttestor{
			{Name: "alice", Type: RelayerAttestorLocal, KeyFile: "/tmp/alice.key"},
			{Name: "bob", Type: RelayerAttestorRemote, GRPC: "bob:8080"},
		},
	}
	cfg.Connections[0].AttestorSetB = &RelayerAttestorSet{
		Threshold: 1,
		Attestors: []RelayerAttestor{{Name: "carol", Type: RelayerAttestorRemote, GRPC: "carol:8080"}},
	}

	file, err := buildRelayerFileConfig(cfg)
	require.NoError(t, err)
	require.Equal(t, chainOverrideFileConfig{
		ChainID: "1", TxSubmissionDelay: "10ms", PacketBatchSize: 7, PacketBatchTimeout: "250ms",
	}, file.Relayer.ChainOverrides[0])
	require.Equal(t, []signerConfig{
		{Alias: "tx", Type: RelayerSignerRemote, GRPC: "kms:9090", RemoteKeyID: "relay-key"},
		{Alias: "alice-signer", Type: RelayerSignerLocal, File: "/tmp/alice.key"},
	}, file.Signers)
	require.Equal(t, []attestationConfig{{
		ChainID: "2", Name: "alice", Signer: "alice-signer", FinalityOffset: 3,
	}}, file.Attestor.Attestations)
	require.Equal(t, []attestorEntryFileConfig{
		{Name: "alice", Type: RelayerAttestorLocal},
		{Name: "bob", Type: RelayerAttestorRemote, GRPC: "bob:8080"},
	}, file.Relayer.Clients[0].AttestorSet.Attestors)
	require.Equal(t, cfg.SignerAlias, file.Relayer.Routes[0].SourceSignerAlias)
	require.Equal(t, cfg.SignerAlias, file.Relayer.Routes[0].DestSignerAlias)
}

func TestRemoteSignerBacksLegacyAttestors(t *testing.T) {
	cfg := testRelayerConfig()
	cfg.SignerType = RelayerSignerRemote
	cfg.SignerKeyFile = ""
	cfg.SignerGRPC = "kms:9090"
	cfg.SignerRemoteKeyID = "relay-key"

	file, err := buildRelayerFileConfig(cfg)
	require.NoError(t, err)
	require.Equal(t, []signerConfig{
		{Alias: "tx", Type: RelayerSignerRemote, GRPC: "kms:9090", RemoteKeyID: "relay-key"},
		{
			Alias: "local-attestor-1-signer", Type: RelayerSignerRemote,
			GRPC: "kms:9090", RemoteKeyID: "relay-key",
		},
		{
			Alias: "local-attestor-2-signer", Type: RelayerSignerRemote,
			GRPC: "kms:9090", RemoteKeyID: "relay-key",
		},
	}, file.Signers)
}

func TestMixedAttestorSetsEmitOnlyReferencedLocals(t *testing.T) {
	cfg := testRelayerConfig()
	cfg.SignerType = RelayerSignerRemote
	cfg.SignerKeyFile = ""
	cfg.SignerGRPC = "kms:9090"
	cfg.SignerRemoteKeyID = "relay-key"
	cfg.Connections[0].AttestorSetA = &RelayerAttestorSet{
		Threshold: 1,
		Attestors: []RelayerAttestor{{
			Name: "remote", Type: RelayerAttestorRemote, GRPC: "attestor:8080",
		}},
	}

	file, err := buildRelayerFileConfig(cfg)
	require.NoError(t, err)
	require.Equal(t, []attestationConfig{{
		ChainID: "1", Name: "local-attestor-1", Signer: "local-attestor-1-signer", FinalityOffset: 3,
	}}, file.Attestor.Attestations)
	require.Equal(t, signerConfig{
		Alias: "local-attestor-1-signer", Type: RelayerSignerRemote,
		GRPC: "kms:9090", RemoteKeyID: "relay-key",
	}, file.Signers[1])
}

func TestBuildRelayerConfigRejectsHarnessInvalidConfig(t *testing.T) {
	tests := []struct {
		name string
		edit func(*RelayerConfig)
		err  string
	}{
		{"signer key", func(c *RelayerConfig) { c.SignerKeyFile = "" }, "signer key file is required"},
		{"local key", func(c *RelayerConfig) {
			c.Connections[0].AttestorSetA = &RelayerAttestorSet{
				Threshold: 1,
				Attestors: []RelayerAttestor{{Name: "a", Type: RelayerAttestorLocal}},
			}
		}, "local attestor \"a\" key file is required"},
		{"local conflict", func(c *RelayerConfig) {
			c.Connections[0].AttestorSetA = &RelayerAttestorSet{
				Threshold: 1,
				Attestors: []RelayerAttestor{{Name: "a", Type: RelayerAttestorLocal, KeyFile: "key"}},
			}
			c.Connections[0].AttestorSetB = &RelayerAttestorSet{
				Threshold: 1,
				Attestors: []RelayerAttestor{{Name: "a", Type: RelayerAttestorLocal, KeyFile: "key"}},
			}
		}, "local attestor \"a\" has conflicting chain or key file"},
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
		Routes: []RelayerRoute{{SourceChain: "1", SourceClient: "client-1"}},
	}
}

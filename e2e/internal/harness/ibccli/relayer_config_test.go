// SPDX-License-Identifier: Apache-2.0

package ibccli

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func TestBuildRelayerConfigYAML(t *testing.T) {
	cfg := testRelayerConfig()
	for _, chain := range cfg.Chains {
		cfg.Attestors = append(cfg.Attestors, RelayerAttestor{
			Name: "local-attestor-" + chain.ChainID, Type: RelayerAttestorLocal,
			ChainID: chain.ChainID, KeyFile: cfg.SignerKeyFile,
		})
	}
	file, err := buildRelayerFileConfig(cfg)
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
        ws: ws://chain-1
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
            autoRelay:
                enabled: true
          clientB:
            chainId: "2"
            signer: tx
            clientId: client-2
            type: attestation
attestors:
    - name: local-attestor-1
      chainId: "1"
      type: local
      signer: local-attestor-1-signer
      finalityOffset: 3
    - name: local-attestor-2
      chainId: "2"
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

func TestClientEndsOptOutOfAutoRelay(t *testing.T) {
	cfg := testRelayerConfig()
	cfg.Connections[0].A.AutoRelay = false

	file, err := buildRelayerFileConfig(cfg)
	require.NoError(t, err)
	require.Nil(t, file.Relayer.Connections[0].ClientA.AutoRelay)
	require.Nil(t, file.Relayer.Connections[0].ClientB.AutoRelay)
}

func TestBuildRelayerConfigOverrides(t *testing.T) {
	cfg := testRelayerConfig()
	cfg.SignerType = RelayerSignerRemote
	cfg.SignerKeyFile = ""
	cfg.SignerGRPC = "kms:9090"
	cfg.SignerRemoteKeyID = "relay-key"
	cfg.ClearInterval = 2 * time.Second
	cfg.Chains[0].PacketBatchSize = 7
	cfg.Chains[0].PacketBatchTimeout = 250 * time.Millisecond
	cfg.Attestors = []RelayerAttestor{
		{Name: "alice", Type: RelayerAttestorLocal, ChainID: "2", KeyFile: "/tmp/alice.key"},
		{Name: "bob", Type: RelayerAttestorRemote, GRPC: "bob:8080"},
		{Name: "carol", Type: RelayerAttestorRemote, GRPC: "carol:8080"},
	}

	file, err := buildRelayerFileConfig(cfg)
	require.NoError(t, err)
	require.Equal(t, "2s", file.Relayer.ClearInterval)
	require.Equal(t, chainOverrideFileConfig{
		ChainID: "1", TxSubmissionDelay: "10ms", PacketBatchSize: 7,
		PacketBatchTimeout: 250 * time.Millisecond,
	}, file.Relayer.ChainOverrides[0])
	require.Equal(t, []signerConfig{
		{Alias: "tx", Type: RelayerSignerRemote, GRPC: "kms:9090", RemoteKeyID: "relay-key"},
		{Alias: "alice-signer", Type: RelayerSignerLocal, File: "/tmp/alice.key"},
	}, file.Signers)
	require.Equal(t, []attestorFileConfig{
		{Name: "alice", ChainID: "2", Type: RelayerAttestorLocal, Signer: "alice-signer", FinalityOffset: 3},
		{Name: "bob", Type: RelayerAttestorRemote, GRPC: "bob:8080"},
		{Name: "carol", Type: RelayerAttestorRemote, GRPC: "carol:8080"},
	}, file.Attestors)
	require.Equal(t, cfg.SignerAlias, file.Relayer.Connections[0].ClientA.Signer)
	require.Equal(t, cfg.SignerAlias, file.Relayer.Connections[0].ClientB.Signer)
}

func TestBuildRelayerConfigRejectsHarnessInvalidConfig(t *testing.T) {
	tests := []struct {
		name string
		edit func(*RelayerConfig)
		err  string
	}{
		{
			"missing client type A",
			func(c *RelayerConfig) { c.Connections[0].A.ClientType = "" },
			`end A: unsupported client type ""`,
		},
		{
			"missing client type B",
			func(c *RelayerConfig) { c.Connections[0].B.ClientType = "" },
			`end B: unsupported client type ""`,
		},
		{"signer key", func(c *RelayerConfig) { c.SignerKeyFile = "" }, "signer key file is required"},
		{"attestor key required", func(c *RelayerConfig) {
			c.Attestors = []RelayerAttestor{{Name: "a", Type: RelayerAttestorLocal, ChainID: "1"}}
		}, "key file is required for local attestors"},
		{"attestor chainId required", func(c *RelayerConfig) {
			c.Attestors = []RelayerAttestor{{Name: "a", Type: RelayerAttestorLocal, KeyFile: "key"}}
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
			{ChainID: "1", RPC: "http://chain-1", WS: "ws://chain-1", ICS26Router: "router-1"},
			{ChainID: "2", RPC: "http://chain-2", ICS26Router: "router-2"},
		},
		Connections: []RelayerConnection{
			{
				A: RelayerClientEnd{
					ChainID:    "1",
					ClientID:   "client-1",
					ClientType: RelayerClientAttestation,
					AutoRelay:  true,
				},
				B: RelayerClientEnd{ChainID: "2", ClientID: "client-2", ClientType: RelayerClientAttestation},
			},
		},
	}
}

func TestClientTypesPerEnd(t *testing.T) {
	cfg := testRelayerConfig()
	cfg.Connections[0].A.ClientType = RelayerClientBesuQBFT
	cfg.Connections[0].B.ClientType = RelayerClientBesuQBFT

	file, err := buildRelayerFileConfig(cfg)
	require.NoError(t, err)
	require.Equal(t, RelayerClientBesuQBFT, file.Relayer.Connections[0].ClientA.Type)
	require.Equal(t, RelayerClientBesuQBFT, file.Relayer.Connections[0].ClientB.Type)
	require.Nil(t, file.Relayer.Connections[0].ClientA.Params)
	require.Empty(t, file.Attestors)
	require.Equal(t, []signerConfig{
		{Alias: "tx", Type: RelayerSignerLocal, File: "/tmp/default.key"},
	}, file.Signers)

	// ends may differ
	cfg.Connections[0].B.ClientType = RelayerClientAttestation
	file, err = buildRelayerFileConfig(cfg)
	require.NoError(t, err)
	require.Equal(t, RelayerClientBesuQBFT, file.Relayer.Connections[0].ClientA.Type)
	require.Equal(t, RelayerClientAttestation, file.Relayer.Connections[0].ClientB.Type)

	// a prover URL overrides both ends
	cfg.Connections[0].ProverURL = "http://prover:9090"
	file, err = buildRelayerFileConfig(cfg)
	require.NoError(t, err)
	require.Equal(t, RelayerClientRemote, file.Relayer.Connections[0].ClientA.Type)
	require.Equal(t, RelayerClientRemote, file.Relayer.Connections[0].ClientB.Type)

	cfg.Connections[0].ProverURL = ""
	cfg.Connections[0].A.ClientType = "unknown"
	_, err = buildRelayerFileConfig(cfg)
	require.ErrorContains(t, err, "unsupported client type")
}

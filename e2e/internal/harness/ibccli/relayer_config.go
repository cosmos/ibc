// SPDX-License-Identifier: Apache-2.0

package ibccli

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"gopkg.in/yaml.v3"
)

// RelayerConfig describes one relayer process configuration for the black-box
// binary. An empty Attestors list runs without attestors.
type RelayerConfig struct {
	DBPath      string
	SignerAlias string
	// SignerType defaults to RelayerSignerLocal. A remote transaction signer is
	// shared by both ends of every route.
	SignerType string
	SignerGRPC string
	// SignerRemoteKeyID is the opaque remote KMS key selector sent to GetKey
	// and Sign. It is distinct from the signer alias and address.
	SignerRemoteKeyID string
	// SignerKeyFile backs the local transaction signer.
	SignerKeyFile string
	// FinalityOffset applies to local attestations and pipeline finality
	// checks: heights up to "latest" minus the offset count as final. The dev
	// chains behind the harness never serve a moving "finalized" tag.
	FinalityOffset uint64
	Chains         []RelayerChain
	Connections    []RelayerConnection
	Attestors      []RelayerAttestor
}

// RelayerChain is one chain the relayer connects to. ChainID is the EVM
// chain id in decimal.
type RelayerChain struct {
	ChainID string
	RPC     string
	// WS is required by the relayer for chains sourcing auto-relayed routes.
	WS                 string
	ICS26Router        string
	PacketBatchSize    int
	PacketBatchTimeout time.Duration
}

// RelayerClientEnd is a registered client and its endpoint relay policy.
type RelayerClientEnd struct {
	ChainID  string
	ClientID string
	// ClientType is required unless the connection uses a remote prover.
	ClientType string
	AutoRelay  bool
}

// RelayerConnection is a reciprocal on-chain client pair.
type RelayerConnection struct {
	A, B RelayerClientEnd

	// ProverURL points both client ends at a ProverService, overriding the
	// declared client types.
	ProverURL string
}

// RelayerAttestor describes one candidate attestor: a local entry runs in
// the relayer and provisions its signer from KeyFile, watching ChainID; a
// remote entry is reached at a bare gRPC host:port
type RelayerAttestor struct {
	Name    string
	Type    string
	ChainID string // local only
	GRPC    string // remote only
	KeyFile string // local only
}

// WriteRelayerConfig renders the relayer process configuration YAML.
func WriteRelayerConfig(path string, cfg RelayerConfig) error {
	file, err := buildRelayerFileConfig(cfg)
	if err != nil {
		return fmt.Errorf("ibccli: relayer config: %w", err)
	}
	data, err := yaml.Marshal(file)
	if err != nil {
		return fmt.Errorf("ibccli: encode relayer config: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("ibccli: relayer config dir: %w", err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return fmt.Errorf("ibccli: write relayer config: %w", err)
	}
	return nil
}

func buildRelayerFileConfig(cfg RelayerConfig) (fileConfig, error) {
	signerType := cfg.SignerType
	if signerType == "" {
		signerType = RelayerSignerLocal
	}
	switch {
	case cfg.DBPath == "":
		return fileConfig{}, errors.New("db path is required")
	case cfg.SignerAlias == "":
		return fileConfig{}, errors.New("signer alias is required")
	case signerType == RelayerSignerLocal && cfg.SignerKeyFile == "":
		return fileConfig{}, errors.New("signer key file is required")
	case len(cfg.Chains) == 0:
		return fileConfig{}, errors.New("at least one chain is required")
	case len(cfg.Connections) == 0:
		return fileConfig{}, errors.New("at least one connection is required")
	}

	processSigner := signerConfig{Alias: cfg.SignerAlias, Type: signerType}
	if signerType == RelayerSignerRemote {
		processSigner.GRPC = cfg.SignerGRPC
		processSigner.RemoteKeyID = cfg.SignerRemoteKeyID
	} else {
		processSigner.File = cfg.SignerKeyFile
	}
	file := fileConfig{
		Server:  serverConfig{ListenAddress: loopbackAnyPort},
		DB:      dbConfig{Type: dbTypeSQLite, URL: cfg.DBPath},
		Signers: []signerConfig{processSigner},
		// The default 5s dispatch poll is mainnet-shaped; harness awaits are sub-second.
		Relayer: &relayerFileConfig{DispatchPollInterval: "100ms"},
	}

	for _, chain := range cfg.Chains {
		batchSize := chain.PacketBatchSize
		if batchSize == 0 {
			batchSize = 1
		}
		file.Chains = append(file.Chains, chainConfig{
			ChainID: chain.ChainID,
			EVM: evmChainConfig{
				RPC:         chain.RPC,
				WS:          chain.WS,
				ICS26Router: chain.ICS26Router,
			},
		})
		// The relayer's batching and pacing defaults are mainnet-shaped. A
		// batch size of one flushes each harness packet on arrival, and the
		// submission delay paces only consecutive transactions on one chain
		// (retries and multi-route traffic); it must stay non-zero because
		// zero is coerced back to the mainnet default.
		file.Relayer.ChainOverrides = append(file.Relayer.ChainOverrides, chainOverrideFileConfig{
			ChainID:            chain.ChainID,
			TxSubmissionDelay:  "10ms",
			PacketBatchSize:    batchSize,
			PacketBatchTimeout: chain.PacketBatchTimeout,
		})
	}

	for _, attestor := range cfg.Attestors {
		if err := addAttestor(&file, cfg.FinalityOffset, attestor); err != nil {
			return fileConfig{}, fmt.Errorf("attestor %q: %w", attestor.Name, err)
		}
	}

	for _, connection := range cfg.Connections {
		a, err := relayerClientEndConfig(connection.A, cfg.SignerAlias, connection.ProverURL)
		if err != nil {
			return fileConfig{}, fmt.Errorf(
				"connection %s/%s end A: %w",
				connection.A.ChainID,
				connection.A.ClientID,
				err,
			)
		}
		b, err := relayerClientEndConfig(connection.B, cfg.SignerAlias, connection.ProverURL)
		if err != nil {
			return fileConfig{}, fmt.Errorf(
				"connection %s/%s end B: %w",
				connection.B.ChainID,
				connection.B.ClientID,
				err,
			)
		}
		file.Relayer.Connections = append(file.Relayer.Connections, connectionFileConfig{
			Alias:   connection.A.ClientID + "-" + connection.B.ClientID,
			ClientA: a,
			ClientB: b,
		})
	}
	return file, nil
}

func relayerClientEndConfig(end RelayerClientEnd, signer, proverURL string) (clientEndFileConfig, error) {
	result := clientEndFileConfig{
		ChainID:   end.ChainID,
		ClientID:  end.ClientID,
		Signer:    signer,
		Type:      end.ClientType,
		AutoRelay: autoRelay(end.AutoRelay),
	}
	if proverURL != "" {
		result.Type = RelayerClientRemote
		result.Params = map[string]any{"url": proverURL}
	} else if end.ClientType != RelayerClientAttestation && end.ClientType != RelayerClientBesuQBFT {
		return clientEndFileConfig{}, fmt.Errorf("unsupported client type %q", end.ClientType)
	}
	return result, nil
}

// addAttestor declares one explicitly-configured candidate attestor.
// Local entries bring their own key file.
func addAttestor(file *fileConfig, finalityOffset uint64, attestor RelayerAttestor) error {
	switch attestor.Type {
	case RelayerAttestorRemote:
		file.Attestors = append(file.Attestors, attestorFileConfig{
			Name: attestor.Name, Type: RelayerAttestorRemote, GRPC: attestor.GRPC,
		})
		return nil
	case RelayerAttestorLocal:
		switch {
		case attestor.ChainID == "":
			return errors.New("chainId is required for local attestors")
		case attestor.KeyFile == "":
			return errors.New("key file is required for local attestors")
		}

		signerAlias := attestor.Name + "-signer"
		file.Signers = append(file.Signers, signerConfig{
			Alias: signerAlias, Type: RelayerSignerLocal, File: attestor.KeyFile,
		})
		file.Attestors = append(file.Attestors, attestorFileConfig{
			Name: attestor.Name, ChainID: attestor.ChainID, Type: RelayerAttestorLocal,
			Signer: signerAlias, FinalityOffset: uint(finalityOffset),
		})
		return nil
	default:
		return fmt.Errorf("unsupported attestor type %q", attestor.Type)
	}
}

const (
	RelayerSignerLocal       = "local"
	RelayerClientAttestation = "attestation"
	RelayerClientBesuQBFT    = "besu-qbft"
	RelayerClientRemote      = "remote"

	RelayerSignerRemote   = "remote"
	RelayerAttestorLocal  = "local"
	RelayerAttestorRemote = "remote"
)

type relayerFileConfig struct {
	DispatchPollInterval string                    `yaml:"dispatchPollInterval,omitempty"`
	ChainOverrides       []chainOverrideFileConfig `yaml:"chainOverrides,omitempty"`
	Connections          []connectionFileConfig    `yaml:"connections"`
}

type chainOverrideFileConfig struct {
	ChainID            string        `yaml:"chainId"`
	TxSubmissionDelay  string        `yaml:"txSubmissionDelay"`
	PacketBatchSize    int           `yaml:"packetBatchSize"`
	PacketBatchTimeout time.Duration `yaml:"packetBatchTimeout,omitempty"`
}

type connectionFileConfig struct {
	Alias   string              `yaml:"alias"`
	ClientA clientEndFileConfig `yaml:"clientA"`
	ClientB clientEndFileConfig `yaml:"clientB"`
}

type clientEndFileConfig struct {
	ChainID   string               `yaml:"chainId"`
	Signer    string               `yaml:"signer"`
	ClientID  string               `yaml:"clientId"`
	Type      string               `yaml:"type"`
	Params    map[string]any       `yaml:"params,omitempty"`
	AutoRelay *autoRelayFileConfig `yaml:"autoRelay,omitempty"`
}

type autoRelayFileConfig struct {
	Enabled bool `yaml:"enabled"`
}

func autoRelay(enabled bool) *autoRelayFileConfig {
	if !enabled {
		return nil
	}

	return &autoRelayFileConfig{Enabled: true}
}

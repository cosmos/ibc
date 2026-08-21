// SPDX-License-Identifier: Apache-2.0

package ibclink

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"gopkg.in/yaml.v3"
)

// RelayerConfig describes one relayer process configuration for the black-box
// binary. If Attestors is empty, one default local attestor per chain is used.
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
	// SignerKeyFile backs the default local signer and default local attestors.
	SignerKeyFile string
	// FinalityOffset applies to local attestations and pipeline finality
	// checks: heights up to "latest" minus the offset count as final. The dev
	// chains behind the harness never serve a moving "finalized" tag.
	FinalityOffset uint64
	Chains         []RelayerChain
	Connections    []RelayerConnection

	Attestors []RelayerAttestor
}

// RelayerChain is one chain the relayer connects to. ChainID is the EVM
// chain id in decimal.
type RelayerChain struct {
	ChainID            string
	RPC                string
	ICS26Router        string
	PacketBatchSize    int
	PacketBatchTimeout time.Duration
}

// RelayerConnection is a reciprocal on-chain client pair. Clients are the
// registered client identifiers (locators).
type RelayerConnection struct {
	ChainA  string
	ClientA string
	ChainB  string
	ClientB string

	// ProverURL points both client ends at a ProverService instead of
	// resolving attestation locally. Empty keeps the attestation default.
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
		return fmt.Errorf("ibclink: relayer config: %w", err)
	}
	data, err := yaml.Marshal(file)
	if err != nil {
		return fmt.Errorf("ibclink: encode relayer config: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("ibclink: relayer config dir: %w", err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return fmt.Errorf("ibclink: write relayer config: %w", err)
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

	if len(cfg.Attestors) == 0 {
		for _, chain := range cfg.Chains {
			addDefaultLocalAttestor(&file, processSigner, cfg.FinalityOffset, chain.ChainID)
		}
	} else {
		for _, attestor := range cfg.Attestors {
			if err := addAttestor(&file, cfg.FinalityOffset, attestor); err != nil {
				return fileConfig{}, fmt.Errorf("attestor %q: %w", attestor.Name, err)
			}
		}
	}

	for _, connection := range cfg.Connections {
		clientType := "attestation"

		var params map[string]any

		if connection.ProverURL != "" {
			clientType = "remoteProver"
			params = map[string]any{"url": connection.ProverURL}
		}

		file.Relayer.Connections = append(file.Relayer.Connections, connectionFileConfig{
			Alias: connection.ClientA + "-" + connection.ClientB,
			ClientA: clientEndFileConfig{
				ChainID:  connection.ChainA,
				Signer:   cfg.SignerAlias,
				ClientID: connection.ClientA,
				Type:     clientType,
				Params:   params,
			},
			ClientB: clientEndFileConfig{
				ChainID:  connection.ChainB,
				Signer:   cfg.SignerAlias,
				ClientID: connection.ClientB,
				Type:     clientType,
				Params:   params,
			},
		})
	}
	return file, nil
}

// addAttestor declares one explicitly-configured candidate attestor.
// Local entries always bring their own key file, unlike the implicit
// default (addDefaultLocalAttestor), so multiple local attestors don't
// share a signing identity.
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

// addDefaultLocalAttestor declares the default local attestor for a chain,
// backed by the relayer process's own signer.
func addDefaultLocalAttestor(file *fileConfig, processSigner signerConfig, finalityOffset uint64, chainID string) {
	name := localAttestorName(chainID)
	signerAlias := name + "-signer"

	signer := processSigner
	signer.Alias = signerAlias
	file.Signers = append(file.Signers, signer)
	file.Attestors = append(file.Attestors, attestorFileConfig{
		Name: name, ChainID: chainID, Type: RelayerAttestorLocal,
		Signer: signerAlias, FinalityOffset: uint(finalityOffset),
	})
}

func localAttestorName(chainID string) string {
	return "local-attestor-" + chainID
}

const (
	RelayerSignerLocal    = "local"
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
	ChainID  string         `yaml:"chainId"`
	Signer   string         `yaml:"signer"`
	ClientID string         `yaml:"clientId"`
	Type     string         `yaml:"type"`
	Params   map[string]any `yaml:"params,omitempty"`
}

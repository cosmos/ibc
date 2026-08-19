// SPDX-License-Identifier: Apache-2.0

package ibclink

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	linkconfig "github.com/cosmos/ibc/link/config"
)

// RelayerConfig describes one relayer process configuration for the black-box
// binary. If Attestors is empty, one default local attestor per chain is used.
type RelayerConfig struct {
	DBPath      string
	SignerAlias string
	// SignerType defaults to config.SignerLocal. A remote transaction signer is
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
	Attestors      []RelayerAttestor
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
}

// RelayerAttestor describes one candidate attestor: a local entry runs in
// the relayer and provisions its signer from KeyFile, watching ChainID; a
// remote entry is reached at a bare gRPC host:port
type RelayerAttestor struct {
	Name    string
	Type    linkconfig.AttestorType
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
	data, err := linkconfig.MarshalYAML(file)
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

func buildRelayerFileConfig(cfg RelayerConfig) (linkconfig.Config, error) {
	signerType := cfg.SignerType
	if signerType == "" {
		signerType = linkconfig.SignerLocal
	}
	switch {
	case cfg.DBPath == "":
		return linkconfig.Config{}, errors.New("db path is required")
	case cfg.SignerAlias == "":
		return linkconfig.Config{}, errors.New("signer alias is required")
	case signerType == linkconfig.SignerLocal && cfg.SignerKeyFile == "":
		return linkconfig.Config{}, errors.New("signer key file is required")
	case len(cfg.Chains) == 0:
		return linkconfig.Config{}, errors.New("at least one chain is required")
	case len(cfg.Connections) == 0:
		return linkconfig.Config{}, errors.New("at least one connection is required")
	}

	processSigner := linkconfig.SignerConfig{Alias: cfg.SignerAlias, Type: signerType}
	if signerType == linkconfig.SignerRemote {
		processSigner.GRPC = cfg.SignerGRPC
		processSigner.RemoteKeyID = cfg.SignerRemoteKeyID
	} else {
		processSigner.File = cfg.SignerKeyFile
	}
	dispatchPollInterval := 100 * time.Millisecond
	file := linkconfig.Config{
		Server:  linkconfig.ServerConfig{ListenAddress: loopbackAnyPort},
		DB:      linkconfig.DBConfig{Type: linkconfig.DBTypeSQLite, URL: cfg.DBPath},
		Signers: linkconfig.Signers{processSigner},
		// The default 5s dispatch poll is mainnet-shaped; harness awaits are sub-second.
		Relayer: linkconfig.RelayerConfig{DispatchPollInterval: &dispatchPollInterval},
	}

	for _, chain := range cfg.Chains {
		batchSize := chain.PacketBatchSize
		if batchSize == 0 {
			batchSize = 1
		}
		file.Chains = append(file.Chains, linkconfig.ChainConfig{
			ChainID: chain.ChainID,
			EVM: &linkconfig.EVMChainConfig{
				RPC:         chain.RPC,
				ICS26Router: chain.ICS26Router,
			},
		})
		// The relayer's batching and pacing defaults are mainnet-shaped. A
		// batch size of one flushes each harness packet on arrival, and the
		// submission delay paces only consecutive transactions on one chain
		// (retries and multi-route traffic); it must stay non-zero because
		// zero is coerced back to the mainnet default.
		txSubmissionDelay := 10 * time.Millisecond
		override := linkconfig.RelayerChainOverride{
			ChainID:           chain.ChainID,
			TxSubmissionDelay: &txSubmissionDelay,
			PacketBatchSize:   &batchSize,
		}
		if chain.PacketBatchTimeout != 0 {
			override.PacketBatchTimeout = &chain.PacketBatchTimeout
		}
		file.Relayer.ChainOverrides = append(file.Relayer.ChainOverrides, override)
	}

	if len(cfg.Attestors) == 0 {
		for _, chain := range cfg.Chains {
			addDefaultLocalAttestor(&file, processSigner, cfg.FinalityOffset, chain.ChainID)
		}
	} else {
		for _, attestor := range cfg.Attestors {
			if err := addAttestor(&file, cfg.FinalityOffset, attestor); err != nil {
				return linkconfig.Config{}, fmt.Errorf("attestor %q: %w", attestor.Name, err)
			}
		}
	}

	for _, connection := range cfg.Connections {
		file.Relayer.Connections = append(file.Relayer.Connections, linkconfig.ConnectionConfig{
			Alias: connection.ClientA + "-" + connection.ClientB,
			ClientA: linkconfig.ClientEnd{
				ChainID:  connection.ChainA,
				Signer:   cfg.SignerAlias,
				ClientID: connection.ClientA,
				Type:     linkconfig.ClientTypeAttestation,
			},
			ClientB: linkconfig.ClientEnd{
				ChainID:  connection.ChainB,
				Signer:   cfg.SignerAlias,
				ClientID: connection.ClientB,
				Type:     linkconfig.ClientTypeAttestation,
			},
		})
	}
	return file, nil
}

// addAttestor declares one explicitly-configured candidate attestor.
// Local entries always bring their own key file, unlike the implicit
// default (addDefaultLocalAttestor), so multiple local attestors don't
// share a signing identity.
func addAttestor(file *linkconfig.Config, finalityOffset uint64, attestor RelayerAttestor) error {
	switch attestor.Type {
	case linkconfig.AttestorTypeRemote:
		file.Attestors = append(file.Attestors, linkconfig.AttestorConfig{
			Name: attestor.Name, Type: linkconfig.AttestorTypeRemote, GRPC: attestor.GRPC,
		})
		return nil
	case linkconfig.AttestorTypeLocal:
		switch {
		case attestor.ChainID == "":
			return errors.New("chainId is required for local attestors")
		case attestor.KeyFile == "":
			return errors.New("key file is required for local attestors")
		}

		signerAlias := attestor.Name + "-signer"
		file.Signers = append(file.Signers, linkconfig.SignerConfig{
			Alias: signerAlias, Type: linkconfig.SignerLocal, File: attestor.KeyFile,
		})
		file.Attestors = append(file.Attestors, linkconfig.AttestorConfig{
			Name: attestor.Name, ChainID: attestor.ChainID, Type: linkconfig.AttestorTypeLocal,
			Signer: signerAlias, FinalityOffset: uint(finalityOffset),
		})
		return nil
	default:
		return fmt.Errorf("unsupported attestor type %q", attestor.Type)
	}
}

// addDefaultLocalAttestor declares the default local attestor for a chain,
// backed by the relayer process's own signer.
func addDefaultLocalAttestor(
	file *linkconfig.Config,
	processSigner linkconfig.SignerConfig,
	finalityOffset uint64,
	chainID string,
) {
	name := localAttestorName(chainID)
	signerAlias := name + "-signer"

	signer := processSigner
	signer.Alias = signerAlias
	file.Signers = append(file.Signers, signer)
	file.Attestors = append(file.Attestors, linkconfig.AttestorConfig{
		Name: name, ChainID: chainID, Type: linkconfig.AttestorTypeLocal,
		Signer: signerAlias, FinalityOffset: uint(finalityOffset),
	})
}

func localAttestorName(chainID string) string {
	return "local-attestor-" + chainID
}

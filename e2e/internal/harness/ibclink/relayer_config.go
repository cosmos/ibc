package ibclink

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// RelayerConfig describes one relayer process configuration for the black-box
// binary. The relayer runs in dual mode: it hosts one local attestor per
// chain, and every configured client draws its attestation quorum from the
// local attestor watching the client's counterparty chain.
type RelayerConfig struct {
	DBPath      string
	SignerAlias string
	// SignerKeyFile signs relay transactions and local attestations.
	SignerKeyFile string
	// FinalityOffset applies to local attestations and pipeline finality
	// checks: heights up to "latest" minus the offset count as final. The dev
	// chains behind the harness never serve a moving "finalized" tag.
	FinalityOffset uint64
	Chains         []RelayerChain
	Connections    []RelayerConnection
	Routes         []RelayerRoute
}

// RelayerChain is one chain the relayer connects to. ChainID is the EVM
// chain id in decimal.
type RelayerChain struct {
	ChainID     string
	RPC         string
	ICS26Router string
}

// RelayerConnection is a reciprocal on-chain client pair. Clients are the
// registered client identifiers (locators).
type RelayerConnection struct {
	ChainA  string
	ClientA string
	ChainB  string
	ClientB string
}

// RelayerRoute relays the full packet lifecycle for packets sent through
// SourceClient on SourceChain.
type RelayerRoute struct {
	SourceChain  string
	SourceClient string
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
	switch {
	case cfg.DBPath == "":
		return fileConfig{}, errors.New("db path is required")
	case cfg.SignerAlias == "":
		return fileConfig{}, errors.New("signer alias is required")
	case cfg.SignerKeyFile == "":
		return fileConfig{}, errors.New("signer key file is required")
	case len(cfg.Chains) == 0:
		return fileConfig{}, errors.New("at least one chain is required")
	case len(cfg.Connections) == 0:
		return fileConfig{}, errors.New("at least one connection is required")
	case len(cfg.Routes) == 0:
		return fileConfig{}, errors.New("at least one route is required")
	}

	file := fileConfig{
		Server: serverConfig{ListenAddress: "127.0.0.1:0"},
		DB:     dbConfig{Type: "sqlite", URL: cfg.DBPath},
		Signers: []signerConfig{{
			Alias: cfg.SignerAlias,
			Type:  typeLocal,
			File:  cfg.SignerKeyFile,
		}},
		// The default 5s dispatch poll is mainnet-shaped; harness awaits are sub-second.
		Relayer: &relayerFileConfig{DispatchPollInterval: "100ms"},
	}
	for _, chain := range cfg.Chains {
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
			ChainID:           chain.ChainID,
			TxSubmissionDelay: "10ms",
			PacketBatchSize:   1,
		})
		// Attestations must not share a signer alias; every per-chain alias
		// still loads the same key file.
		attestorSigner := localAttestorName(chain.ChainID) + "-signer"
		file.Signers = append(file.Signers, signerConfig{
			Alias: attestorSigner,
			Type:  typeLocal,
			File:  cfg.SignerKeyFile,
		})
		file.Attestor.Attestations = append(file.Attestor.Attestations, attestationConfig{
			ChainID:        chain.ChainID,
			Name:           localAttestorName(chain.ChainID),
			Signer:         attestorSigner,
			FinalityOffset: uint(cfg.FinalityOffset),
		})
	}

	clients := make(map[string]string, 2*len(cfg.Connections))
	for _, connection := range cfg.Connections {
		for _, end := range []struct {
			chain, client, counterpartyChain, counterpartyClient string
		}{
			{connection.ChainA, connection.ClientA, connection.ChainB, connection.ClientB},
			{connection.ChainB, connection.ClientB, connection.ChainA, connection.ClientA},
		} {
			file.Relayer.Clients = append(file.Relayer.Clients, clientFileConfig{
				Alias:                end.client,
				ClientID:             end.client,
				ChainID:              end.chain,
				CounterpartyChainID:  end.counterpartyChain,
				CounterpartyClientID: end.counterpartyClient,
				Type:                 "attestation",
				AttestorSet: attestorSetFileConfig{
					CounterpartyChainFinalityOffset: cfg.FinalityOffset,
					Threshold:                       1,
					Attestors: []attestorEntryFileConfig{{
						Name: localAttestorName(end.counterpartyChain),
						Type: typeLocal,
					}},
				},
			})
			clients[end.chain+"/"+end.client] = end.client
		}
	}

	for _, route := range cfg.Routes {
		alias, ok := clients[route.SourceChain+"/"+route.SourceClient]
		if !ok {
			return fileConfig{}, fmt.Errorf(
				"route source client %q on chain %q is not part of any connection",
				route.SourceClient,
				route.SourceChain,
			)
		}
		file.Relayer.Routes = append(file.Relayer.Routes, routeFileConfig{
			SourceClient:      alias,
			SourceSignerAlias: cfg.SignerAlias,
			DestSignerAlias:   cfg.SignerAlias,
		})
	}
	return file, nil
}

func localAttestorName(chainID string) string {
	return "local-attestor-" + chainID
}

// typeLocal is the config value for file-backed signers and in-process attestors.
const typeLocal = "local"

type relayerFileConfig struct {
	DispatchPollInterval string                    `yaml:"dispatchPollInterval,omitempty"`
	ChainOverrides       []chainOverrideFileConfig `yaml:"chainOverrides,omitempty"`
	Clients              []clientFileConfig        `yaml:"clients"`
	Routes               []routeFileConfig         `yaml:"routesToRelay"`
}

type chainOverrideFileConfig struct {
	ChainID           string `yaml:"chainId"`
	TxSubmissionDelay string `yaml:"txSubmissionDelay"`
	PacketBatchSize   int    `yaml:"packetBatchSize"`
}

type clientFileConfig struct {
	Alias                string                `yaml:"alias"`
	ClientID             string                `yaml:"clientId"`
	ChainID              string                `yaml:"chainId"`
	CounterpartyChainID  string                `yaml:"counterpartyChainId"`
	CounterpartyClientID string                `yaml:"counterpartyClientId"`
	Type                 string                `yaml:"type"`
	AttestorSet          attestorSetFileConfig `yaml:"attestorSet"`
}

type attestorSetFileConfig struct {
	CounterpartyChainFinalityOffset uint64                    `yaml:"counterpartyChainFinalityOffset"`
	Threshold                       int                       `yaml:"threshold"`
	Attestors                       []attestorEntryFileConfig `yaml:"attestors"`
}

type attestorEntryFileConfig struct {
	Name string `yaml:"name"`
	Type string `yaml:"type"`
}

type routeFileConfig struct {
	SourceClient      string `yaml:"sourceClient"`
	SourceSignerAlias string `yaml:"sourceSignerAlias"`
	DestSignerAlias   string `yaml:"destSignerAlias"`
}

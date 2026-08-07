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
// binary. Clients without an explicit attestor set use the legacy local
// attestor watching their counterparty chain.
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
	// SignerKeyFile backs the default local signer and legacy local attestors.
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
	ChainID            string
	RPC                string
	ICS26Router        string
	PacketBatchSize    int
	PacketBatchTimeout time.Duration
}

// RelayerConnection is a reciprocal on-chain client pair. Clients are the
// registered client identifiers (locators).
type RelayerConnection struct {
	ChainA       string
	ClientA      string
	AttestorSetA *RelayerAttestorSet
	ChainB       string
	ClientB      string
	AttestorSetB *RelayerAttestorSet
}

// RelayerAttestorSet replaces the legacy single local attestor for one client.
type RelayerAttestorSet struct {
	Threshold int
	Attestors []RelayerAttestor
}

// RelayerAttestor describes an explicit attestor: a local entry runs in the
// relayer and provisions its signer from KeyFile; a remote entry is reached at
// a bare gRPC host:port.
type RelayerAttestor struct {
	Name    string
	Type    string
	GRPC    string
	KeyFile string
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
	case len(cfg.Routes) == 0:
		return fileConfig{}, errors.New("at least one route is required")
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
	explicitAttestors := false
	for _, connection := range cfg.Connections {
		explicitAttestors = explicitAttestors ||
			connection.AttestorSetA != nil ||
			connection.AttestorSetB != nil
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
		if explicitAttestors {
			continue
		}
		// Attestations must not share a signer alias; every per-chain alias
		// still loads the same key file.
		addLocalAttestor(
			&file, processSigner, cfg.FinalityOffset,
			chain.ChainID, localAttestorName(chain.ChainID), "",
		)
	}

	clients := make(map[string]string, 2*len(cfg.Connections))
	type localAttestor struct{ chainID, keyFile string }
	locals := make(map[string]localAttestor)
	for _, connection := range cfg.Connections {
		for _, end := range []struct {
			chain, client, counterpartyChain, counterpartyClient string
			set                                                  *RelayerAttestorSet
		}{
			{
				connection.ChainA, connection.ClientA, connection.ChainB,
				connection.ClientB, connection.AttestorSetA,
			},
			{
				connection.ChainB, connection.ClientB, connection.ChainA,
				connection.ClientA, connection.AttestorSetB,
			},
		} {
			set, err := buildAttestorSet(end.set, cfg.FinalityOffset, end.counterpartyChain)
			if err != nil {
				return fileConfig{}, fmt.Errorf("client %q on chain %q: %w", end.client, end.chain, err)
			}
			if explicitAttestors {
				for i, entry := range set.Attestors {
					if entry.Type != RelayerAttestorLocal {
						continue
					}
					keyFile := ""
					if end.set != nil {
						keyFile = end.set.Attestors[i].KeyFile
					}
					local := localAttestor{end.counterpartyChain, keyFile}
					if previous, ok := locals[entry.Name]; ok {
						if previous != local {
							return fileConfig{}, fmt.Errorf(
								"local attestor %q has conflicting chain or key file",
								entry.Name,
							)
						}
						continue
					}
					locals[entry.Name] = local
					addLocalAttestor(
						&file, processSigner, cfg.FinalityOffset,
						local.chainID, entry.Name, local.keyFile,
					)
				}
			}
			file.Relayer.Clients = append(file.Relayer.Clients, clientFileConfig{
				Alias:                end.client,
				ClientID:             end.client,
				ChainID:              end.chain,
				CounterpartyChainID:  end.counterpartyChain,
				CounterpartyClientID: end.counterpartyClient,
				Type:                 "attestation",
				AttestorSet:          set,
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

func addLocalAttestor(
	file *fileConfig,
	signer signerConfig,
	finalityOffset uint64,
	chainID, name, keyFile string,
) {
	alias := name + "-signer"
	if keyFile != "" {
		signer = signerConfig{Type: RelayerSignerLocal, File: keyFile}
	}
	signer.Alias = alias
	file.Signers = append(file.Signers, signer)
	file.Attestor.Attestations = append(file.Attestor.Attestations, attestationConfig{
		ChainID: chainID, Name: name, Signer: alias, FinalityOffset: uint(finalityOffset),
	})
}

func buildAttestorSet(
	set *RelayerAttestorSet,
	finalityOffset uint64,
	counterpartyChain string,
) (attestorSetFileConfig, error) {
	if set == nil {
		return attestorSetFileConfig{
			CounterpartyChainFinalityOffset: finalityOffset,
			Threshold:                       1,
			Attestors: []attestorEntryFileConfig{{
				Name: localAttestorName(counterpartyChain), Type: RelayerAttestorLocal,
			}},
		}, nil
	}
	result := attestorSetFileConfig{
		CounterpartyChainFinalityOffset: finalityOffset,
		Threshold:                       set.Threshold,
	}
	for _, attestor := range set.Attestors {
		entry := attestorEntryFileConfig{Name: attestor.Name, Type: attestor.Type, GRPC: attestor.GRPC}
		if attestor.Type == RelayerAttestorLocal {
			if attestor.KeyFile == "" {
				return attestorSetFileConfig{}, fmt.Errorf(
					"local attestor %q key file is required", attestor.Name,
				)
			}
			entry.GRPC = ""
		}
		result.Attestors = append(result.Attestors, entry)
	}
	return result, nil
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
	Clients              []clientFileConfig        `yaml:"clients"`
	Routes               []routeFileConfig         `yaml:"routesToRelay"`
}

type chainOverrideFileConfig struct {
	ChainID            string        `yaml:"chainId"`
	TxSubmissionDelay  string        `yaml:"txSubmissionDelay"`
	PacketBatchSize    int           `yaml:"packetBatchSize"`
	PacketBatchTimeout time.Duration `yaml:"packetBatchTimeout,omitempty"`
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
	GRPC string `yaml:"grpc,omitempty"`
}

type routeFileConfig struct {
	SourceClient      string `yaml:"sourceClient"`
	SourceSignerAlias string `yaml:"sourceSignerAlias"`
	DestSignerAlias   string `yaml:"destSignerAlias"`
}

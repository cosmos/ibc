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
// SourceClient on SourceChain. Every configured connection is always relayed
// bidirectionally now (there is no per-direction opt-in in the file format
// this harness writes), so this is validation-only: it documents which
// client the test author expects to be relayable and catches typos, but no
// longer affects the emitted YAML.
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
		Server:  serverConfig{ListenAddress: "127.0.0.1:0"},
		DB:      dbConfig{Type: "sqlite", URL: cfg.DBPath},
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
		if err := addLocalAttestor(
			&file, processSigner, cfg.FinalityOffset,
			chain.ChainID, localAttestorName(chain.ChainID), "",
		); err != nil {
			return fileConfig{}, err
		}
	}

	sourceClients := make(map[string]struct{}, 2*len(cfg.Connections))
	// declared tracks every attestor alias declared so far in the unified
	// top-level attestors[] list -- local and remote share one namespace now,
	// so both kinds need to be checked for alias reuse/conflicts together.
	type declaredAttestor struct {
		isLocal bool
		chainID string
		keyFile string // local only
		grpc    string // remote only
	}
	declared := make(map[string]declaredAttestor)
	for _, connection := range cfg.Connections {
		ends := make(map[string]clientEndFileConfig, 2)
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
			set, err := buildAttestorAliases(end.set, cfg.FinalityOffset, end.counterpartyChain)
			if err != nil {
				return fileConfig{}, fmt.Errorf("client %q on chain %q: %w", end.client, end.chain, err)
			}
			if explicitAttestors {
				for i, alias := range set.Attestors {
					var explicitEntry *RelayerAttestor
					if end.set != nil {
						explicitEntry = &end.set.Attestors[i]
					}

					var candidate declaredAttestor
					if explicitEntry == nil || explicitEntry.Type == RelayerAttestorLocal {
						keyFile := ""
						if explicitEntry != nil {
							keyFile = explicitEntry.KeyFile
						}
						candidate = declaredAttestor{isLocal: true, chainID: end.counterpartyChain, keyFile: keyFile}
					} else {
						candidate = declaredAttestor{chainID: end.counterpartyChain, grpc: explicitEntry.GRPC}
					}

					if previous, ok := declared[alias]; ok {
						if previous != candidate {
							return fileConfig{}, fmt.Errorf(
								"attestor %q has conflicting chain, key file, or grpc address",
								alias,
							)
						}
						continue
					}
					declared[alias] = candidate

					if candidate.isLocal {
						if err := addLocalAttestor(
							&file, processSigner, cfg.FinalityOffset,
							candidate.chainID, alias, candidate.keyFile,
						); err != nil {
							return fileConfig{}, err
						}
						continue
					}
					file.Attestors = append(file.Attestors, attestorFileConfig{
						Alias: alias, Name: alias, ChainID: candidate.chainID,
						Type: RelayerAttestorRemote, GRPC: candidate.grpc,
					})
				}
			}
			ends[end.chain+"/"+end.client] = clientEndFileConfig{
				ChainID:     end.chain,
				Signer:      cfg.SignerAlias,
				ClientID:    end.client,
				Type:        "attestation",
				AttestorSet: set,
			}
			sourceClients[end.chain+"/"+end.client] = struct{}{}
		}
		file.Relayer.Connections = append(file.Relayer.Connections, connectionFileConfig{
			Alias:   connection.ClientA + "-" + connection.ClientB,
			ClientA: ends[connection.ChainA+"/"+connection.ClientA],
			ClientB: ends[connection.ChainB+"/"+connection.ClientB],
		})
	}
	for _, route := range cfg.Routes {
		if _, ok := sourceClients[route.SourceChain+"/"+route.SourceClient]; !ok {
			return fileConfig{}, fmt.Errorf(
				"route source client %q on chain %q is not part of any connection",
				route.SourceClient,
				route.SourceChain,
			)
		}
	}
	return file, nil
}

// addLocalAttestor declares a local attestor in the unified top-level
// attestors list and its backing signer. alias also serves as the
// attestor's self-reported name -- this harness has no need for them to
// differ.
func addLocalAttestor(
	file *fileConfig,
	signer signerConfig,
	finalityOffset uint64,
	chainID, alias, keyFile string,
) error {
	signerAlias := alias + "-signer"
	if keyFile != "" {
		signer = signerConfig{Type: RelayerSignerLocal, File: keyFile}
	}
	signer.Alias = signerAlias
	file.Signers = append(file.Signers, signer)
	file.Attestors = append(file.Attestors, attestorFileConfig{
		Alias: alias, Name: alias, ChainID: chainID, Type: RelayerAttestorLocal,
		Signer: signerAlias, FinalityOffset: uint(finalityOffset),
	})
	return nil
}

// buildAttestorAliases resolves one client end's attestor set to the
// aliases its attestorSet should reference. Remote aliases are declared
// directly (they carry everything needed inline); local aliases are
// declared by the caller via addLocalAttestor once resolved, since a local
// attestor's signer must be provisioned exactly once even when referenced
// by both ends of a connection.
func buildAttestorAliases(
	set *RelayerAttestorSet,
	finalityOffset uint64,
	counterpartyChain string,
) (attestorSetFileConfig, error) {
	if set == nil {
		return attestorSetFileConfig{
			CounterpartyChainFinalityOffset: finalityOffset,
			Threshold:                       1,
			Attestors:                       []string{localAttestorName(counterpartyChain)},
		}, nil
	}
	result := attestorSetFileConfig{
		CounterpartyChainFinalityOffset: finalityOffset,
		Threshold:                       set.Threshold,
	}
	for _, attestor := range set.Attestors {
		if attestor.Type == RelayerAttestorLocal && attestor.KeyFile == "" {
			return attestorSetFileConfig{}, fmt.Errorf(
				"local attestor %q key file is required", attestor.Name,
			)
		}
		result.Attestors = append(result.Attestors, attestor.Name)
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
	ChainID     string                `yaml:"chainId"`
	Signer      string                `yaml:"signer"`
	ClientID    string                `yaml:"clientId"`
	Type        string                `yaml:"type"`
	AttestorSet attestorSetFileConfig `yaml:"attestorSet"`
}

type attestorSetFileConfig struct {
	Threshold                       int      `yaml:"threshold"`
	CounterpartyChainFinalityOffset uint64   `yaml:"counterpartyChainFinalityOffset"`
	Attestors                       []string `yaml:"attestors"`
}

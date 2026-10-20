// SPDX-License-Identifier: Apache-2.0

package ibccli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// deployCommandTimeout bounds the sequential deployment transactions a
// single `ibc deploy` call submits and mines.
const deployCommandTimeout = 10 * time.Minute

// DeployConfig describes the minimal IBC CLI config `ibc deploy` needs: chains
// with an RPC endpoint, all sharing one local ECDSA signer as their deployer.
type DeployConfig struct {
	DBPath        string
	SignerAlias   string
	SignerKeyFile string
	Chains        []DeployChain
}

// DeployChain is one chain `ibc deploy` can target. ChainID is the decimal
// EVM chain id. WS is only needed once a rendered connection auto-relays from
// the chain.
type DeployChain struct {
	ChainID string
	RPC     string
	WS      string
}

// WriteDeployConfig renders an IBC CLI config file declaring every Chain with
// DeployConfig.SignerAlias as its deployer.
func WriteDeployConfig(path string, cfg DeployConfig) error {
	switch {
	case cfg.DBPath == "":
		return errors.New("ibccli: deploy config: db path is required")
	case cfg.SignerAlias == "":
		return errors.New("ibccli: deploy config: signer alias is required")
	case cfg.SignerKeyFile == "":
		return errors.New("ibccli: deploy config: signer key file is required")
	case len(cfg.Chains) == 0:
		return errors.New("ibccli: deploy config: at least one chain is required")
	}

	file := fileConfig{
		Server: serverConfig{ListenAddress: loopbackAnyPort},
		DB:     dbConfig{Type: dbTypeSQLite, URL: cfg.DBPath},
		Signers: []signerConfig{{
			Alias: cfg.SignerAlias,
			Type:  RelayerSignerLocal,
			File:  cfg.SignerKeyFile,
		}},
	}
	for _, chain := range cfg.Chains {
		file.Chains = append(file.Chains, chainConfig{
			ChainID:  chain.ChainID,
			EVM:      evmChainConfig{RPC: chain.RPC, WS: chain.WS},
			Deployer: cfg.SignerAlias,
		})
	}

	data, err := yaml.Marshal(file)
	if err != nil {
		return fmt.Errorf("ibccli: encode deploy config: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("ibccli: deploy config dir: %w", err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return fmt.Errorf("ibccli: write deploy config: %w", err)
	}
	return nil
}

// WriteICS26Routers writes routers[chainId] into chains[].evm.ics26Router of
// the existing IBC CLI config at path, leaving the rest of the file as it is.
// Each router value is replaced along with its line comment, such as the
// "TODO: fill in" the CLI leaves on an empty router.
func WriteICS26Routers(path string, routers map[string]string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("ibccli: read config: %w", err)
	}
	var doc yaml.Node
	if err = yaml.Unmarshal(data, &doc); err != nil {
		return fmt.Errorf("ibccli: decode config: %w", err)
	}
	if len(doc.Content) == 0 {
		return errors.New("ibccli: config is empty")
	}

	if chains := mappingValue(doc.Content[0], "chains"); chains != nil {
		for _, chain := range chains.Content {
			id, evm := mappingValue(chain, "chainId"), mappingValue(chain, "evm")
			if id == nil || evm == nil {
				continue
			}
			if router, ok := routers[id.Value]; ok {
				setMappingString(evm, "ics26Router", router)
			}
		}
	}

	out, err := yaml.Marshal(&doc)
	if err != nil {
		return fmt.Errorf("ibccli: encode config: %w", err)
	}
	if err = os.WriteFile(path, out, 0o600); err != nil {
		return fmt.Errorf("ibccli: write config: %w", err)
	}
	return nil
}

// mappingValue returns the value under key in mapping node m, or nil.
func mappingValue(m *yaml.Node, key string) *yaml.Node {
	if m.Kind != yaml.MappingNode {
		return nil
	}
	for i := 0; i+1 < len(m.Content); i += 2 {
		if m.Content[i].Value == key {
			return m.Content[i+1]
		}
	}
	return nil
}

// setMappingString sets key in mapping node m to the string value, adding it
// when missing.
func setMappingString(m *yaml.Node, key, value string) {
	str := func(s string) yaml.Node { return yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: s} }
	if v := mappingValue(m, key); v != nil {
		*v = str(value)
		return
	}
	k, v := str(key), str(value)
	m.Content = append(m.Content, &k, &v)
}

// RenderedConfig holds the relayer connections and attestors in the config
// `ibc deploy render-config` prints: the existing config merged with what it
// rendered.
type RenderedConfig struct {
	Connections []ConnectionFileConfig
	Attestors   []AttestorFileConfig
}

// ParseRenderedConfig decodes the config `ibc deploy render-config` prints.
func ParseRenderedConfig(data []byte) (RenderedConfig, error) {
	var file struct {
		Relayer struct {
			Connections []ConnectionFileConfig `yaml:"connections"`
		} `yaml:"relayer"`
		Attestors []AttestorFileConfig `yaml:"attestors"`
	}
	if err := yaml.Unmarshal(data, &file); err != nil {
		return RenderedConfig{}, fmt.Errorf("ibccli: decode rendered config: %w", err)
	}
	return RenderedConfig{Connections: file.Relayer.Connections, Attestors: file.Attestors}, nil
}

// KeyFilePath returns the local path `ibc keys import`/`ibc keys new` uses
// for name under this Driver's home.
func (r *Driver) KeyFilePath(name string) string {
	return filepath.Join(r.configHome, "keys", name+".json")
}

// Deploy runs `ibc deploy <args...>` against the configured home and returns
// stdout. A non-zero exit becomes an *ExitError.
func (r *Driver) Deploy(ctx context.Context, args ...string) ([]byte, error) {
	return r.run(ctx, "deploy", deployCommandTimeout, append([]string{"deploy"}, args...)...)
}

// Run runs a one-shot `ibc <command> <args...>` against the configured home
// and returns stdout. A non-zero exit becomes an *ExitError.
func (r *Driver) Run(ctx context.Context, command string, args ...string) ([]byte, error) {
	return r.run(ctx, command, defaultCommandTimeout, append([]string{command}, args...)...)
}

func (r *Driver) run(ctx context.Context, label string, timeout time.Duration, args ...string) ([]byte, error) {
	full := slices.Concat(args, r.configArgs())
	res, err := r.exec(ctx, r.bin, label, timeout, full...)
	if err != nil {
		return nil, err
	}
	if res.code != 0 {
		// snippet keeps the stderr tail, but the CLI prints its usage block
		// after the error line, so the head is where the failure is
		return nil, &ExitError{Code: res.code, Class: ErrInternal, Stderr: headSnippet(res.stderr)}
	}
	return res.stdout, nil
}

// headSnippet truncates stderr keeping the start, where the error line
// precedes the CLI's usage output.
func headSnippet(s string) string {
	s = strings.TrimSpace(s)
	if len(s) > maxStderrSnippet {
		s = s[:maxStderrSnippet] + "..."
	}
	return s
}

// SPDX-License-Identifier: Apache-2.0

package ibclink

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	linkconfig "github.com/cosmos/ibc/link/config"
)

// deployCommandTimeout bounds the sequential deployment transactions a
// single `ibc deploy` call submits and mines.
const deployCommandTimeout = 10 * time.Minute

// placeholderRouter satisfies chains[].evm.ics26Router's required-non-empty
// validation before `ibc deploy core` has recorded a real router address;
// deploy commands read the router from the manifest, never from config.
const placeholderRouter = "0x0000000000000000000000000000000000000001"

// DeployConfig describes the minimal Link config `ibc deploy` needs: chains
// with an RPC endpoint, all sharing one local ECDSA signer as their deployer.
type DeployConfig struct {
	DBPath        string
	SignerAlias   string
	SignerKeyFile string
	Chains        []DeployChain
}

// DeployChain is one chain `ibc deploy` can target. ChainID is the decimal
// EVM chain id.
type DeployChain struct {
	ChainID string
	RPC     string
}

// WriteDeployConfig renders a Link config file declaring every Chain with
// DeployConfig.SignerAlias as its deployer.
func WriteDeployConfig(path string, cfg DeployConfig) error {
	switch {
	case cfg.DBPath == "":
		return errors.New("ibclink: deploy config: db path is required")
	case cfg.SignerAlias == "":
		return errors.New("ibclink: deploy config: signer alias is required")
	case cfg.SignerKeyFile == "":
		return errors.New("ibclink: deploy config: signer key file is required")
	case len(cfg.Chains) == 0:
		return errors.New("ibclink: deploy config: at least one chain is required")
	}

	file := linkconfig.Config{
		Server: linkconfig.ServerConfig{ListenAddress: loopbackAnyPort},
		DB:     linkconfig.DBConfig{Type: linkconfig.DBTypeSQLite, URL: cfg.DBPath},
		Signers: linkconfig.Signers{{
			Alias: cfg.SignerAlias,
			Type:  linkconfig.SignerLocal,
			File:  cfg.SignerKeyFile,
		}},
	}
	for _, chain := range cfg.Chains {
		file.Chains = append(file.Chains, linkconfig.ChainConfig{
			ChainID:  chain.ChainID,
			EVM:      &linkconfig.EVMChainConfig{RPC: chain.RPC, ICS26Router: placeholderRouter},
			Deployer: cfg.SignerAlias,
		})
	}

	data, err := linkconfig.MarshalYAML(file)
	if err != nil {
		return fmt.Errorf("ibclink: encode deploy config: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("ibclink: deploy config dir: %w", err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return fmt.Errorf("ibclink: write deploy config: %w", err)
	}
	return nil
}

// KeyFilePath returns the local path `ibc keys import`/`ibc keys new` uses
// for name under this Driver's home.
func (r *Driver) KeyFilePath(name string) string {
	return filepath.Join(r.configHome, "keys", name+".json")
}

// Deploy runs `ibc deploy <args...>` against the configured home and returns
// stdout. A non-zero exit becomes an *ExitError.
func (r *Driver) Deploy(ctx context.Context, args ...string) ([]byte, error) {
	full := append([]string{"deploy"}, args...)
	full = append(full, r.configArgs()...)
	res, err := r.exec(ctx, r.bin, "deploy", deployCommandTimeout, full...)
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

// KeysImportECDSA imports a raw ECDSA private key under name via `ibc keys
// import ecdsa`, so a DeployConfig signer can reference a funded key.
func (r *Driver) KeysImportECDSA(ctx context.Context, name, privateKeyHex string) error {
	args := append([]string{"keys", "import", "ecdsa", name, "--private-key", privateKeyHex}, r.configArgs()...)
	res, err := r.exec(ctx, r.bin, "keys import ecdsa", defaultCommandTimeout, args...)
	if err != nil {
		return err
	}
	if res.code != 0 {
		return &ExitError{Code: res.code, Class: ErrInternal, Stderr: headSnippet(res.stderr)}
	}
	return nil
}

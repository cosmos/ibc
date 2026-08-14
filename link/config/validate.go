// SPDX-License-Identifier: Apache-2.0

package config

import (
	"os"
	"strings"

	"github.com/pkg/errors"

	"github.com/cosmos/ibc/link/internal/fsutil"
	"github.com/cosmos/ibc/link/internal/network"
)

func (c Config) Validate() error {
	if err := c.Server.Validate(); err != nil {
		return errors.Wrap(err, ".server")
	}

	if err := c.DB.Validate(); err != nil {
		return errors.Wrap(err, ".db")
	}

	chainIDs := make(map[string]struct{})
	for _, chain := range c.Chains {
		if err := chain.Validate(); err != nil {
			return errors.Wrapf(err, ".chains[%s]", chain.ChainID)
		}

		if _, ok := chainIDs[chain.ChainID]; ok {
			return errors.Wrapf(errors.Errorf("duplicate chainId: %q", chain.ChainID), ".chains")
		}
		chainIDs[chain.ChainID] = struct{}{}
	}

	if err := c.Relayer.Validate(); err != nil {
		return errors.Wrap(err, ".relayer")
	}

	if err := c.Attestors.Validate(); err != nil {
		return errors.Wrap(err, ".attestors")
	}

	if err := c.Signers.Validate(); err != nil {
		return errors.Wrap(err, ".signers")
	}

	return c.crossValidate()
}

func (c Config) crossValidate() error {
	signerSet := make(map[string]struct{}, len(c.Signers))
	for _, signer := range c.Signers {
		signerSet[signer.Alias] = struct{}{}
	}

	for i, a := range c.Attestors {
		if a.Type != AttestorTypeLocal {
			continue
		}
		if _, exists := signerSet[a.Signer]; !exists {
			return errors.Errorf(".attestors[%d].signer references unknown signer: %q", i, a.Signer)
		}
	}

	for _, chain := range c.Chains {
		if chain.Deployer == "" {
			continue
		}
		if _, exists := signerSet[chain.Deployer]; !exists {
			return errors.Errorf(".chains[%s].deployer references unknown signer: %q", chain.ChainID, chain.Deployer)
		}
	}

	if err := c.validateChainReferences(); err != nil {
		return err
	}

	if err := c.validateConnectionSigners(signerSet); err != nil {
		return errors.Wrap(err, ".relayer.connections")
	}

	return nil
}

type namedClientEnd struct {
	label string
	cfg   ClientEnd
}

func connectionEnds(conn ConnectionConfig) []namedClientEnd {
	return []namedClientEnd{{"clientA", conn.ClientA}, {"clientB", conn.ClientB}}
}

// validateChainReferences ensures chains referenced by the relayer config are
// declared in the top-level chains block.
func (c Config) validateChainReferences() error {
	for _, chain := range c.Relayer.ChainOverrides {
		if _, ok := c.Chain(chain.ChainID); chain.ChainID != "" && !ok {
			return errors.Errorf(".chainOverrides[%s] chainId not declared in top-level chains", chain.ChainID)
		}
	}

	for _, conn := range c.Relayer.Connections {
		for _, end := range connectionEnds(conn) {
			if _, ok := c.Chain(end.cfg.ChainID); end.cfg.ChainID != "" && !ok {
				return errors.Errorf(
					".connections[%s].%s chainId %q not declared in top-level chains",
					conn.Alias, end.label, end.cfg.ChainID,
				)
			}
		}
	}

	return nil
}

// validateConnectionSigners ensures every client end's signer resolves to a
// configured signer.
func (c Config) validateConnectionSigners(signerSet map[string]struct{}) error {
	for _, conn := range c.Relayer.Connections {
		for _, end := range connectionEnds(conn) {
			if _, exists := signerSet[end.cfg.Signer]; !exists {
				return errors.Errorf(
					"connection %q %s references unknown signer %q",
					conn.Alias, end.label, end.cfg.Signer,
				)
			}
		}
	}

	return nil
}

func (c ChainConfig) Validate() error {
	if c.ChainID == "" {
		return errors.New(".chainId required")
	}

	if c.Type() == ChainTypeEVM && c.EVM.RPC == "" {
		return errors.New(".evm.rpc required")
	}

	return nil
}

func (c ServerConfig) Validate() error {
	if err := network.ValidateListenAddr(c.ListenAddress); err != nil {
		return errors.Wrapf(err, ".listenAddr %q", c.ListenAddress)
	}

	return nil
}

func (c DBConfig) Validate() error {
	switch {
	case c.Type != DBTypeSQLite && c.Type != DBTypePostgres:
		return errors.Errorf(".type must be one of [%q, %q], got %q", DBTypeSQLite, DBTypePostgres, c.Type)
	case c.Type == DBTypeSQLite && c.URL == sqliteInMemory:
		return errors.New(".url must not be :memory: for sqlite")
	case c.URL == "":
		return errors.New(".url must not be empty")
	}

	return nil
}

// Validate validates the attestors list. Allows empty.
func (a Attestors) Validate() error {
	localNames := make(map[string]struct{})
	// keyed by chainId+signer: the same signer backing one operator's local
	// attestor on two different chains is fine, but reusing it for two
	// attestors on the same chain is always a redundant duplicate.
	localChainSigners := make(map[string]struct{})
	for i, attestor := range a {
		if err := attestor.Validate(); err != nil {
			return errors.Wrapf(err, "[%d]", i)
		}

		if attestor.Type != AttestorTypeLocal {
			continue
		}

		if _, exists := localNames[attestor.Name]; exists {
			return errors.Errorf("duplicate local attestor name: %q", attestor.Name)
		}
		localNames[attestor.Name] = struct{}{}

		chainSigner := attestor.ChainID + "/" + attestor.Signer
		if _, exists := localChainSigners[chainSigner]; exists {
			return errors.Errorf("duplicate local attestor signer %q on chain %q", attestor.Signer, attestor.ChainID)
		}
		localChainSigners[chainSigner] = struct{}{}
	}

	return nil
}

func (c AttestorConfig) Validate() error {
	switch {
	case c.Name == "":
		return errors.New(".name required")
	case c.Type != AttestorTypeLocal && c.Type != AttestorTypeRemote:
		return errors.Errorf(".type unknown attestor type: %q", c.Type)
	}

	switch c.Type {
	case AttestorTypeLocal:
		switch {
		case c.ChainID == "":
			return errors.New(".chainId required for local attestors")
		case c.Signer == "":
			return errors.New(".signer required for local attestors")
		case c.GRPC != "":
			return errors.New(".grpc must not be set for local attestors")
		}
	case AttestorTypeRemote:
		switch {
		case c.GRPC == "":
			return errors.New(".grpc required for remote attestors")
		case strings.Contains(c.GRPC, "://"):
			return errors.Errorf(".grpc must be a bare host:port, not a URL: %q", c.GRPC)
		case c.ChainID != "":
			return errors.New(".chainId must not be set for remote attestors")
		case c.Signer != "":
			return errors.New(".signer must not be set for remote attestors")
		case c.FinalityOffset != 0:
			return errors.New(".finalityOffset must not be set for remote attestors")
		}
	}

	return nil
}

func (c Signers) Validate() error {
	set := make(map[string]struct{})

	for i, signer := range c {
		if err := signer.Validate(); err != nil {
			return errors.Wrapf(err, ".signers[%d]", i)
		}

		if _, exists := set[signer.Alias]; exists {
			return errors.Errorf(".signers duplicate alias: %q", signer.Alias)
		}

		set[signer.Alias] = struct{}{}
	}

	return nil
}

func (c SignerConfig) Validate() error {
	switch {
	case c.Alias == "":
		return errors.New(".alias required")
	case c.Type == "":
		return errors.New(".type required")
	case c.Type != SignerLocal && c.Type != SignerRemote:
		return errors.Errorf(".type must be one of [%q, %q], got %q", SignerLocal, SignerRemote, c.Type)
	case c.Type == SignerLocal && c.File == "":
		return errors.New(".file required for local signer")
	case c.Type == SignerRemote && c.GRPC == "":
		return errors.New(".grpc required for remote signer")
	case c.Type == SignerRemote && c.RemoteKeyID == "":
		return errors.New(".remoteKeyId required for remote signer")
	}

	if c.Type == SignerLocal {
		path, err := fsutil.ExpandHome(c.File)
		if err != nil {
			return errors.Wrap(err, ".file")
		}

		fallbacks := fsutil.KeyFileFallbacks(path)

		if err := fileExistsInAny(fallbacks...); err != nil {
			return errors.Wrapf(err, ".file %s", path)
		}
	}

	return nil
}

func fileExistsInAny(path ...string) error {
	for _, p := range path {
		if err := fileExists(p); err == nil {
			return nil
		}
	}

	return errors.New("file not found")
}

func fileExists(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		return err
	}

	if info.IsDir() {
		return errors.Errorf("path is a directory")
	}

	return nil
}

// Validate validates the relayer config. Allows empty blocks.
func (c RelayerConfig) Validate() error {
	if c.DispatchPollInterval != nil && *c.DispatchPollInterval <= 0 {
		return errors.New(".dispatchPollInterval must be positive")
	}
	if err := c.validateChainOverrides(); err != nil {
		return err
	}

	return c.validateConnections()
}

func (c RelayerConfig) validateChainOverrides() error {
	chainIDs := make(map[string]struct{})

	for _, chain := range c.ChainOverrides {
		if err := chain.Validate(); err != nil {
			return errors.Wrapf(err, ".chainOverrides[%s]", chain.ChainID)
		}

		if _, ok := chainIDs[chain.ChainID]; ok {
			return errors.Errorf(".chainOverrides duplicate chainId: %q", chain.ChainID)
		}
		chainIDs[chain.ChainID] = struct{}{}
	}

	return nil
}

func (c RelayerConfig) validateConnections() error {
	aliases := make(map[string]struct{})
	clientEnds := make(map[string]struct{})

	for _, conn := range c.Connections {
		if err := conn.Validate(); err != nil {
			return errors.Wrapf(err, ".connections[%s]", conn.Alias)
		}

		if _, ok := aliases[conn.Alias]; ok {
			return errors.Errorf(".connections duplicate alias: %q", conn.Alias)
		}
		aliases[conn.Alias] = struct{}{}

		for _, end := range []ClientEnd{conn.ClientA, conn.ClientB} {
			key := end.ChainID + "/" + end.ClientID
			if _, ok := clientEnds[key]; ok {
				return errors.Errorf(".connections duplicate client %q on chain %q", end.ClientID, end.ChainID)
			}
			clientEnds[key] = struct{}{}
		}
	}

	return nil
}

func (c ConnectionConfig) Validate() error {
	if c.Alias == "" {
		return errors.New(".alias required")
	}

	if err := c.ClientA.Validate(); err != nil {
		return errors.Wrap(err, ".clientA")
	}
	if err := c.ClientB.Validate(); err != nil {
		return errors.Wrap(err, ".clientB")
	}

	if c.ClientA.ChainID != "" && c.ClientA.ChainID == c.ClientB.ChainID {
		return errors.New(".clientA and .clientB must be on different chains")
	}

	return nil
}

func (c ClientEnd) Validate() error {
	switch {
	case c.ChainID == "":
		return errors.New(".chainId required")
	case c.ClientID == "":
		return errors.New(".clientId required")
	case c.Signer == "":
		return errors.New(".signer required")
	case c.Type != ClientTypeAttestation:
		return errors.Errorf(".type unknown client type: %q", c.Type)
	}

	return nil
}

func (c RelayerChainOverride) Validate() error {
	switch {
	case c.ChainID == "":
		return errors.New(".chainId required")
	case c.TxSubmissionDelay != nil && *c.TxSubmissionDelay < 0:
		return errors.New(".txSubmissionDelay must not be negative")
	case c.PacketBatchSize != nil && *c.PacketBatchSize <= 0:
		return errors.New(".packetBatchSize must be positive")
	case c.PacketBatchTimeout != nil && *c.PacketBatchTimeout <= 0:
		return errors.New(".packetBatchTimeout must be positive")
	}

	if c.EVM != nil {
		if err := c.EVM.Validate(); err != nil {
			return errors.Wrap(err, ".evm")
		}
	}

	return nil
}

func (c RelayerEVMConfig) Validate() error {
	switch {
	case c.GasFeeCapMultiplier != nil && *c.GasFeeCapMultiplier <= 0:
		return errors.New(".gasFeeCapMultiplier must be positive")
	case c.GasTipCapMultiplier != nil && *c.GasTipCapMultiplier <= 0:
		return errors.New(".gasTipCapMultiplier must be positive")
	}

	return nil
}

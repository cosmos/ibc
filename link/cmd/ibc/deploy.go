// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bufio"
	"context"
	"encoding/hex"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/pkg/errors"
	"github.com/spf13/cobra"

	"github.com/cosmos/ibc/link/internal/config"
	"github.com/cosmos/ibc/link/internal/deploy"
	"github.com/cosmos/ibc/link/internal/deploy/evm"
	"github.com/cosmos/ibc/link/internal/deploy/manifest"
	"github.com/cosmos/ibc/link/internal/service/signer"
	"github.com/cosmos/ibc/link/keyfile"
)

var (
	flagDeployManifestDir     string
	flagDeployDeployer        string
	flagDeployDryRun          bool
	flagDeployYes             bool
	flagDeployChain           string
	flagDeployCounterparty    string
	flagDeployClientType      string
	flagDeployClientID        string
	flagDeployCounterpartyCID string
	flagDeployAttestors       []string
	flagDeployThreshold       uint8
	flagDeployHeight          uint64
	flagDeployTimestamp       uint64

	flagDeployIFTName             string
	flagDeployIFTSymbol           string
	flagDeployIFTOwner            string
	flagDeployBridgeClientID      string
	flagDeployBridgeCounterparty  string
	flagDeploySendCallConstructor string
)

var (
	cmdDeploy = &cobra.Command{
		Use:   "deploy",
		Short: "Deployment automation commands",
	}

	cmdDeployCore = &cobra.Command{
		Use:   "core",
		Short: "Deploy the core IBC routing stack on one chain",
		RunE:  deployCore,
	}

	cmdDeployClient = &cobra.Command{
		Use:   "client",
		Short: "Deploy and register a light client tracking a counterparty chain",
		RunE:  deployClient,
	}

	cmdDeployStatus = &cobra.Command{
		Use:   useStatus,
		Short: "Verify recorded deployments against live chain state",
		RunE:  deployStatus,
	}

	cmdDeployRenderConfig = &cobra.Command{
		Use:   "render-config [chainA] [chainB]",
		Short: "Project two deployment manifests into config sections for relaying between them (stdout)",
		Args:  cobra.ExactArgs(2),
		RunE:  deployRenderConfig,
	}

	cmdDeployGMP = &cobra.Command{
		Use:   "gmp",
		Short: "Deploy the ICS27-GMP app on one chain",
		RunE:  deployGMP,
	}

	cmdDeployIFT = &cobra.Command{
		Use:   "ift",
		Short: "Deploy an IFT token on one chain",
		RunE:  deployIFT,
	}

	cmdDeployIFTBridge = &cobra.Command{
		Use:   "ift-bridge",
		Short: "Register an IFT bridge to a counterparty token over a client",
		RunE:  deployIFTBridge,
	}
)

// resolveDeployerAlias prefers the --deployer flag over the chain's config.
func resolveDeployerAlias(chain config.ChainConfig, flagAlias string) string {
	if flagAlias != "" {
		return flagAlias
	}
	return chain.Deployer
}

// deployerKeyHex resolves a local ECDSA signer's private key for the target.
func deployerKeyHex(cfg config.Config, alias string) (string, error) {
	if alias == "" {
		return "", errors.New("no deployer configured: set chains[].deployer or pass --deployer")
	}
	var sc *config.SignerConfig
	for i := range cfg.Signers {
		if cfg.Signers[i].Alias == alias {
			sc = &cfg.Signers[i]
			break
		}
	}
	if sc == nil {
		return "", errors.Errorf("deployer signer %q not found in config", alias)
	}
	if sc.Type != config.SignerLocal {
		return "", errors.Errorf("deployer signer %q must be a local key (deployment tooling needs the raw key)", alias)
	}
	path, err := config.ExpandHome(sc.File)
	if err != nil {
		return "", err
	}
	key, err := signer.LocalKeyFromFile(config.KeyFileFallbacks(path)...)
	if err != nil {
		return "", err
	}
	if key.Type() != keyfile.ECDSA {
		return "", errors.Errorf("deployer signer %q must be an ecdsa key", alias)
	}
	return hex.EncodeToString(key.PrivateKey()), nil
}

// newTarget builds the deploy target for a chain based on its configured
// type. When needSigner is false, the target is built read-only: no local
// deployer key is resolved.
func newTarget(
	ctx context.Context,
	cfg config.Config,
	chainID, deployerFlag string,
	needSigner bool,
) (deploy.Target, error) {
	chain, ok := cfg.Chain(chainID)
	if !ok {
		return nil, errors.Errorf("chain %q not declared in config", chainID)
	}
	var keyHex string
	if needSigner {
		alias := resolveDeployerAlias(chain, deployerFlag)
		var err error
		keyHex, err = deployerKeyHex(cfg, alias)
		if err != nil {
			return nil, err
		}
	}
	switch chain.Type() {
	case config.ChainTypeEVM:
		return evm.New(ctx, evm.Options{
			ChainID:        chainID,
			RPCURL:         chain.EVM.RPC,
			DeployerKeyHex: keyHex,
		})
	default:
		return nil, errors.Errorf("chain %q has no supported deployment target", chainID)
	}
}

func confirmOrAbort(results []deploy.StepResult) error {
	if flagDeployYes || flagDeployDryRun {
		return nil
	}
	pending := 0
	for _, r := range results {
		if r.Action != deploy.ActionSkipped {
			pending++
		}
	}
	if pending == 0 {
		return nil
	}
	fmt.Printf("About to execute %d step(s) that submit transactions. Proceed? [y/N]: ", pending)
	line, err := bufio.NewReader(os.Stdin).ReadString('\n')
	if err != nil {
		return errors.Wrap(err, "confirmation required; rerun with --yes for non-interactive use")
	}
	if answer := strings.ToLower(strings.TrimSpace(line)); answer != "y" && answer != "yes" {
		return errors.New("aborted")
	}
	return nil
}

// planThenRun previews steps in dry-run mode, confirms, then executes.
func planThenRun(ctx context.Context, steps []deploy.Step) error {
	log := slog.Default()
	preview, err := deploy.RunSteps(ctx, log, true, steps)
	if err != nil {
		return err
	}
	if flagDeployDryRun {
		return config.PrintJSON(preview)
	}
	if confirmErr := confirmOrAbort(preview); confirmErr != nil {
		return confirmErr
	}
	results, err := deploy.RunSteps(ctx, log, false, steps)
	if printErr := config.PrintJSON(results); printErr != nil {
		return printErr
	}
	return err
}

func deployCore(cmd *cobra.Command, _ []string) error {
	cfg, err := setupHomeWithConfig()
	if err != nil {
		return err
	}
	if flagDeployChain == "" {
		return errors.New("--chain is required")
	}
	target, err := newTarget(cmd.Context(), cfg, flagDeployChain, flagDeployDeployer, true)
	if err != nil {
		return err
	}
	return planThenRun(cmd.Context(), deploy.CoreSteps(target, flagDeployManifestDir, flagDeployChain))
}

// clientSpec assembles the ClientSpec for --chain tracking --counterparty-chain,
// defaulting trusted state from the counterparty chain head.
func clientSpec(
	ctx context.Context,
	cfg config.Config,
	counterpartyTarget deploy.Target,
	chainID, counterpartyChainID string,
) (deploy.ClientSpec, error) {
	// both ids default to one shared name: client ids are per-chain
	// namespaces, so the same name on each side unambiguously identifies
	// the connection, and the mirrored `deploy client` invocation derives
	// the identical pair
	clientID := flagDeployClientID
	if clientID == "" {
		clientID = defaultClientID(chainID, counterpartyChainID)
	}
	if !deploy.ValidClientID(clientID) {
		return deploy.ClientSpec{}, errors.Errorf("invalid client id %q", clientID)
	}
	counterpartyClientID := flagDeployCounterpartyCID
	if counterpartyClientID == "" {
		counterpartyClientID = defaultClientID(chainID, counterpartyChainID)
	}
	var attestors []string
	if len(flagDeployAttestors) > 0 {
		for _, token := range flagDeployAttestors {
			address, err := resolveAttestorToken(cfg, token)
			if err != nil {
				return deploy.ClientSpec{}, err
			}
			attestors = append(attestors, address)
		}
	} else {
		var err error
		attestors, err = attestorsForChain(cfg, counterpartyChainID)
		if err != nil {
			return deploy.ClientSpec{}, err
		}
	}
	height, timestamp := flagDeployHeight, flagDeployTimestamp
	if height == 0 || timestamp == 0 {
		h, ts, err := counterpartyTarget.Head(ctx)
		if err != nil {
			return deploy.ClientSpec{}, errors.Wrap(err, "fetch counterparty head for initial trusted state")
		}
		if height == 0 {
			// a fresh chain's head is genesis (0), which clients reject as
			// an initial trusted height
			height = max(h, 1)
		}
		if timestamp == 0 {
			timestamp = ts
		}
	}
	spec := deploy.ClientSpec{
		ClientID:             clientID,
		Type:                 flagDeployClientType,
		CounterpartyChainID:  counterpartyChainID,
		CounterpartyClientID: counterpartyClientID,
	}
	switch flagDeployClientType {
	case deploy.ClientTypeAttestation:
		spec.Params = deploy.AttestationParams{
			Attestors:        attestors,
			Threshold:        flagDeployThreshold,
			InitialHeight:    height,
			InitialTimestamp: timestamp,
		}
	default:
		return deploy.ClientSpec{}, fmt.Errorf(
			"cannot construct client spec for unknown client type %s",
			flagDeployClientType,
		)
	}
	return spec, nil
}

// defaultClientID derives the shared client id for a chain pair, stable
// regardless of argument order.
func defaultClientID(a, b string) string {
	ids := []string{a, b}
	sort.Strings(ids)
	return "link-" + ids[0] + "-" + ids[1]
}

func deployClient(cmd *cobra.Command, _ []string) error {
	cfg, err := setupHomeWithConfig()
	if err != nil {
		return err
	}
	if flagDeployChain == "" || flagDeployCounterparty == "" {
		return errors.New("--chain and --counterparty-chain are required")
	}
	target, err := newTarget(cmd.Context(), cfg, flagDeployChain, flagDeployDeployer, true)
	if err != nil {
		return err
	}
	counterpartyTarget, err := newTarget(cmd.Context(), cfg, flagDeployCounterparty, flagDeployDeployer, false)
	if err != nil {
		return errors.Wrapf(err, "counterparty chain %s", flagDeployCounterparty)
	}
	spec, err := clientSpec(cmd.Context(), cfg, counterpartyTarget, flagDeployChain, flagDeployCounterparty)
	if err != nil {
		return err
	}
	return planThenRun(cmd.Context(), deploy.ClientSteps(target, flagDeployManifestDir, flagDeployChain, spec))
}

// statusChains resolves which chains status reports on: an explicit --chain
// validated against the config, or the union of configured chains and
// existing manifests so undeclared-but-deployed chains still surface.
func statusChains(cfg config.Config, manifestDir, explicit string) ([]string, error) {
	if explicit != "" {
		if _, ok := cfg.Chain(explicit); !ok {
			return nil, errors.Errorf("chain %q not declared in config", explicit)
		}
		return []string{explicit}, nil
	}
	seen := map[string]struct{}{}
	var chains []string
	for _, chain := range cfg.Chains {
		seen[chain.ChainID] = struct{}{}
		chains = append(chains, chain.ChainID)
	}
	matches, err := filepath.Glob(filepath.Join(manifestDir, "*.json"))
	if err != nil {
		return nil, err
	}
	for _, path := range matches {
		id := strings.TrimSuffix(filepath.Base(path), ".json")
		if _, ok := seen[id]; !ok {
			chains = append(chains, id)
		}
	}
	sort.Strings(chains)
	return chains, nil
}

// statusError shapes a per-chain failure entry for the status report.
func statusError(err error) map[string]string {
	return map[string]string{useStatus: "error", "error": err.Error()}
}

func deployStatus(cmd *cobra.Command, _ []string) error {
	cfg, err := setupHomeWithConfig()
	if err != nil {
		return err
	}
	chains, err := statusChains(cfg, flagDeployManifestDir, flagDeployChain)
	if err != nil {
		return err
	}
	if len(chains) == 0 {
		return errors.Errorf(
			"nothing to report: no chains declared in config and no manifests in %s; declare chains or pass --chain",
			flagDeployManifestDir,
		)
	}
	failed := false
	out := map[string]any{}
	for _, chainID := range chains {
		m, err := manifest.Load(flagDeployManifestDir, chainID)
		if err != nil {
			return err
		}
		if m == nil {
			out[chainID] = map[string]string{useStatus: "no manifest"}
			continue
		}
		// record per-chain failures (undeclared chain, unreachable RPC)
		// instead of aborting the whole sweep
		target, err := newTarget(cmd.Context(), cfg, chainID, flagDeployDeployer, false)
		if err != nil {
			out[chainID] = statusError(err)
			failed = true
			continue
		}
		report, err := target.Verify(cmd.Context(), m)
		if err != nil {
			out[chainID] = statusError(err)
			failed = true
			continue
		}
		out[chainID] = report
		if len(report.Failed()) > 0 {
			failed = true
		}
	}
	if err := config.PrintJSON(out); err != nil {
		return err
	}
	if failed {
		return errors.New("verification failed")
	}
	return nil
}

// renderedDeployment is the subset of the config schema render-config emits,
// so the output can be merged into ibc.yml without zero-valued sections.
type renderedDeployment struct {
	Chains  []config.ChainConfig `yaml:"chains"`
	Relayer struct {
		Clients []config.ClientConfig `yaml:"clients"`
	} `yaml:"relayer"`
}

// renderedChain copies the operated-on chain's config (if declared) and sets
// its router to the deployed address, so the emitted chains[] entry is
// ready to relay against.
func renderedChain(cfg config.Config, m *manifest.Manifest) config.ChainConfig {
	chain, _ := cfg.Chain(m.ChainID)
	chain.ChainID = m.ChainID
	evm := config.EVMChainConfig{ICS26Router: m.Core.Router}
	if chain.EVM != nil {
		evm.RPC = chain.EVM.RPC
	}
	chain.EVM = &evm
	return chain
}

func renderedClient(m *manifest.Manifest, c manifest.Client) config.ClientConfig {
	client := config.ClientConfig{
		Alias:                m.ChainID + "-" + c.ClientID,
		ClientID:             c.ClientID,
		ChainID:              m.ChainID,
		CounterpartyChainID:  c.CounterpartyChainID,
		CounterpartyClientID: c.CounterpartyClientID,
		Type:                 config.ClientType(c.Type),
	}
	if c.Type == deploy.ClientTypeAttestation {
		// render-config only sees Load()ed manifests, so JSON numbers are
		// float64. Attestor names cannot be derived from the addresses in
		// params; the operator fills them in to match attestor config.
		threshold, _ := c.Params["threshold"].(float64)
		client.AttestorSet = &config.AttestorSetConfig{Threshold: int(threshold)}
	}
	return client
}

// renderRelayConfig projects two deployment manifests into the config
// sections needed to relay between them for every mutual client pair.
func renderRelayConfig(cfg config.Config, a, b *manifest.Manifest) (renderedDeployment, error) {
	var out renderedDeployment
	out.Chains = []config.ChainConfig{renderedChain(cfg, a), renderedChain(cfg, b)}
	for _, ca := range a.Clients {
		if ca.CounterpartyChainID != b.ChainID {
			continue
		}
		cb, ok := b.Client(ca.CounterpartyClientID)
		if !ok || cb.CounterpartyChainID != a.ChainID || cb.CounterpartyClientID != ca.ClientID {
			continue
		}
		out.Relayer.Clients = append(out.Relayer.Clients, renderedClient(a, ca), renderedClient(b, cb))
	}
	if len(out.Relayer.Clients) == 0 {
		return renderedDeployment{}, errors.Errorf(
			"no mutual client pair between chains %s and %s: run `ibc deploy client` on each chain first (chains %s and %s)",
			a.ChainID,
			b.ChainID,
			a.ChainID,
			b.ChainID,
		)
	}
	return out, nil
}

func deployRenderConfig(_ *cobra.Command, args []string) error {
	cfg, err := setupHomeWithConfig()
	if err != nil {
		return err
	}
	manifests := make([]*manifest.Manifest, len(args))
	for i, chainID := range args {
		m, loadErr := manifest.Load(flagDeployManifestDir, chainID)
		if loadErr != nil {
			return loadErr
		}
		if m == nil {
			return errors.Errorf(
				"no manifest for chain %s in %s: run `ibc deploy` against it first",
				chainID, flagDeployManifestDir,
			)
		}
		manifests[i] = m
	}
	out, err := renderRelayConfig(cfg, manifests[0], manifests[1])
	if err != nil {
		return err
	}
	return config.PrintYAML(out)
}

// deployerAddress derives the deployer's EVM address, for use as the default
// IFT owner. Reuses signer.EVMAddressOf (as the attestor path does) rather
// than round-tripping the raw private key.
func deployerAddress(cfg config.Config, alias string) (string, error) {
	if alias == "" {
		return "", errors.New("no deployer configured: set chains[].deployer or pass --deployer")
	}
	sc, ok := cfg.Signer(alias)
	if !ok {
		return "", errors.Errorf("deployer signer %q not found in config", alias)
	}
	return signer.EVMAddressOf(sc)
}

func deployGMP(cmd *cobra.Command, _ []string) error {
	cfg, err := setupHomeWithConfig()
	if err != nil {
		return err
	}
	if flagDeployChain == "" {
		return errors.New("--chain is required")
	}
	target, err := newTarget(cmd.Context(), cfg, flagDeployChain, flagDeployDeployer, true)
	if err != nil {
		return err
	}
	return planThenRun(cmd.Context(), deploy.GMPSteps(target, flagDeployManifestDir, flagDeployChain))
}

func deployIFT(cmd *cobra.Command, _ []string) error {
	cfg, err := setupHomeWithConfig()
	if err != nil {
		return err
	}
	if flagDeployChain == "" {
		return errors.New("--chain is required")
	}
	if flagDeployIFTName == "" || flagDeployIFTSymbol == "" {
		return errors.New("--name and --symbol are required")
	}
	chain, ok := cfg.Chain(flagDeployChain)
	if !ok {
		return errors.Errorf("chain %q not declared in config", flagDeployChain)
	}
	owner := flagDeployIFTOwner
	if owner == "" {
		owner, err = deployerAddress(cfg, resolveDeployerAlias(chain, flagDeployDeployer))
		if err != nil {
			return err
		}
	}
	target, err := newTarget(cmd.Context(), cfg, flagDeployChain, flagDeployDeployer, true)
	if err != nil {
		return err
	}
	spec := deploy.IFTSpec{Owner: owner, Name: flagDeployIFTName, Symbol: flagDeployIFTSymbol}
	return planThenRun(cmd.Context(), deploy.IFTSteps(target, flagDeployManifestDir, flagDeployChain, spec))
}

func deployIFTBridge(cmd *cobra.Command, _ []string) error {
	cfg, err := setupHomeWithConfig()
	if err != nil {
		return err
	}
	if flagDeployChain == "" {
		return errors.New("--chain is required")
	}
	if flagDeployIFTSymbol == "" || flagDeployBridgeClientID == "" || flagDeployBridgeCounterparty == "" {
		return errors.New("--symbol, --client-id and --counterparty-ift are required")
	}
	target, err := newTarget(cmd.Context(), cfg, flagDeployChain, flagDeployDeployer, true)
	if err != nil {
		return err
	}
	spec := deploy.BridgeSpec{ClientID: flagDeployBridgeClientID, CounterpartyIFT: flagDeployBridgeCounterparty}
	return planThenRun(cmd.Context(), deploy.IFTBridgeSteps(
		target, flagDeployManifestDir, flagDeployChain, flagDeployIFTSymbol, flagDeploySendCallConstructor, spec))
}

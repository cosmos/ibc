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
	flagDeployRouter          string
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

	cmdDeployConnect = &cobra.Command{
		Use:   "connect [chainA] [chainB]",
		Short: "Deploy core stacks and clients in both directions between two chains",
		Args:  cobra.ExactArgs(2),
		RunE:  deployConnect,
	}

	cmdDeployImport = &cobra.Command{
		Use:   "import",
		Short: "Generate a manifest by discovering state from an existing router",
		RunE:  deployImport,
	}

	cmdDeployStatus = &cobra.Command{
		Use:   useStatus,
		Short: "Verify recorded deployments against live chain state",
		RunE:  deployStatus,
	}

	cmdDeployRenderConfig = &cobra.Command{
		Use:   "render-config",
		Short: "Project manifests into relayer config sections (stdout)",
		RunE:  deployRenderConfig,
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
// deployer key is resolved, and the returned deployer address is "".
func newTarget(
	ctx context.Context,
	cfg config.Config,
	chainID, deployerFlag string,
	needSigner bool,
) (deploy.Target, string, error) {
	chain, ok := cfg.Chain(chainID)
	if !ok {
		return nil, "", errors.Errorf("chain %q not declared in config", chainID)
	}
	var keyHex string
	if needSigner {
		alias := resolveDeployerAlias(chain, deployerFlag)
		var err error
		keyHex, err = deployerKeyHex(cfg, alias)
		if err != nil {
			return nil, "", err
		}
	}
	switch chain.Type() {
	case config.ChainTypeEVM:
		driver, err := evm.New(ctx, evm.Options{
			ChainID:        chainID,
			RPCURL:         chain.EVM.RPC,
			Home:           ".", // process cwd is --home (setupHomeWithConfig)
			DeployerKeyHex: keyHex,
		})
		if err != nil {
			return nil, "", err
		}
		return driver, driver.DeployerAddress(), nil
	default:
		return nil, "", errors.Errorf("chain %q has no supported deployment target", chainID)
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
	target, deployer, err := newTarget(cmd.Context(), cfg, flagDeployChain, flagDeployDeployer, true)
	if err != nil {
		return err
	}
	return planThenRun(cmd.Context(), deploy.CoreSteps(target, flagDeployManifestDir, flagDeployChain, deployer))
}

// clientSpec assembles the ClientSpec for --chain tracking --counterparty,
// defaulting trusted state from the counterparty chain head.
func clientSpec(
	ctx context.Context,
	counterpartyTarget deploy.Target,
	chainID, counterpartyChainID string,
) (deploy.ClientSpec, error) {
	clientID := flagDeployClientID
	if clientID == "" {
		clientID = deploy.DefaultClientID(counterpartyChainID)
	}
	if !deploy.ValidClientID(clientID) {
		return deploy.ClientSpec{}, errors.Errorf("invalid client id %q", clientID)
	}
	counterpartyClientID := flagDeployCounterpartyCID
	if counterpartyClientID == "" {
		counterpartyClientID = deploy.DefaultClientID(chainID)
	}
	height, timestamp := flagDeployHeight, flagDeployTimestamp
	if height == 0 || timestamp == 0 {
		h, ts, err := counterpartyTarget.Head(ctx)
		if err != nil {
			return deploy.ClientSpec{}, errors.Wrap(err, "fetch counterparty head for initial trusted state")
		}
		if height == 0 {
			height = h
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
	if flagDeployClientType == deploy.ClientTypeAttestation {
		spec.Params = deploy.AttestationParams{
			Attestors:        flagDeployAttestors,
			Threshold:        flagDeployThreshold,
			InitialHeight:    height,
			InitialTimestamp: timestamp,
		}
	}
	return spec, nil
}

func deployClient(cmd *cobra.Command, _ []string) error {
	cfg, err := setupHomeWithConfig()
	if err != nil {
		return err
	}
	if flagDeployChain == "" || flagDeployCounterparty == "" {
		return errors.New("--chain and --counterparty are required")
	}
	target, _, err := newTarget(cmd.Context(), cfg, flagDeployChain, flagDeployDeployer, true)
	if err != nil {
		return err
	}
	counterpartyTarget, _, err := newTarget(cmd.Context(), cfg, flagDeployCounterparty, flagDeployDeployer, false)
	if err != nil {
		return errors.Wrapf(err, "counterparty chain %s", flagDeployCounterparty)
	}
	spec, err := clientSpec(cmd.Context(), counterpartyTarget, flagDeployChain, flagDeployCounterparty)
	if err != nil {
		return err
	}
	return planThenRun(cmd.Context(), deploy.ClientSteps(target, flagDeployManifestDir, flagDeployChain, spec))
}

func deployConnect(cmd *cobra.Command, args []string) error {
	cfg, err := setupHomeWithConfig()
	if err != nil {
		return err
	}
	chainA, chainB := args[0], args[1]
	targetA, deployerA, err := newTarget(cmd.Context(), cfg, chainA, flagDeployDeployer, true)
	if err != nil {
		return err
	}
	targetB, deployerB, err := newTarget(cmd.Context(), cfg, chainB, flagDeployDeployer, true)
	if err != nil {
		return err
	}

	specA, err := clientSpec(cmd.Context(), targetB, chainA, chainB) // on A, tracking B
	if err != nil {
		return err
	}
	specB, err := clientSpec(cmd.Context(), targetA, chainB, chainA) // on B, tracking A
	if err != nil {
		return err
	}
	// connect derives both IDs; per-direction overrides only apply to `deploy client`
	specA.ClientID, specA.CounterpartyClientID = deploy.DefaultClientID(chainB), deploy.DefaultClientID(chainA)
	specB.ClientID, specB.CounterpartyClientID = deploy.DefaultClientID(chainA), deploy.DefaultClientID(chainB)

	var steps []deploy.Step
	steps = append(steps, deploy.CoreSteps(targetA, flagDeployManifestDir, chainA, deployerA)...)
	steps = append(steps, deploy.CoreSteps(targetB, flagDeployManifestDir, chainB, deployerB)...)
	steps = append(steps, deploy.ClientSteps(targetA, flagDeployManifestDir, chainA, specA)...)
	steps = append(steps, deploy.ClientSteps(targetB, flagDeployManifestDir, chainB, specB)...)
	return planThenRun(cmd.Context(), steps)
}

// mergeManifests folds discovered chain state into an existing manifest.
// discovered is the base for chain-derived facts (router, targetData,
// client addresses, counterparty client ids) since it reflects what's
// actually on chain; existing supplies provenance and per-client metadata
// (CounterpartyChainID/Params/Type) that Discover cannot reconstruct,
// wherever discovered left them empty.
func mergeManifests(existing, discovered *manifest.Manifest) *manifest.Manifest {
	if existing == nil {
		return discovered
	}
	merged := *discovered
	merged.Provenance.Deployer = existing.Provenance.Deployer
	merged.Provenance.ContractsVersion = existing.Provenance.ContractsVersion
	merged.Provenance.TxHashes = existing.Provenance.TxHashes
	merged.Provenance.CreatedAt = existing.Provenance.CreatedAt

	merged.Clients = nil
	for _, c := range discovered.Clients {
		if old, ok := existing.Client(c.ClientID); ok {
			if c.CounterpartyChainID == "" {
				c.CounterpartyChainID = old.CounterpartyChainID
			}
			if c.Params == nil {
				c.Params = old.Params
			}
			if c.Type == "" {
				c.Type = old.Type
			}
		}
		merged.Clients = append(merged.Clients, c)
	}
	return &merged
}

func deployImport(cmd *cobra.Command, _ []string) error {
	cfg, err := setupHomeWithConfig()
	if err != nil {
		return err
	}
	if flagDeployChain == "" || flagDeployRouter == "" {
		return errors.New("--chain and --router are required")
	}
	target, _, err := newTarget(cmd.Context(), cfg, flagDeployChain, flagDeployDeployer, false)
	if err != nil {
		return err
	}
	discovered, err := target.Discover(cmd.Context(), flagDeployRouter)
	if err != nil {
		return err
	}
	discovered.ChainID = flagDeployChain
	existing, err := manifest.Load(flagDeployManifestDir, flagDeployChain)
	if err != nil {
		return err
	}
	m := mergeManifests(existing, discovered)
	if err := m.Save(flagDeployManifestDir); err != nil {
		return err
	}
	return config.PrintJSON(m)
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
		target, _, err := newTarget(cmd.Context(), cfg, chainID, flagDeployDeployer, false)
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

// renderedDeployment is the subset of config.Config that `deploy
// render-config` actually populates. config.Config itself isn't used as the
// output type: printed as YAML it would also emit every zero-valued field
// (server:, db:, signers: [], etc.), which would corrupt a working config
// if merged in as printed.
type renderedDeployment struct {
	Chains  []config.ChainConfig `yaml:"chains"`
	Relayer struct {
		Clients []config.ClientConfig `yaml:"clients"`
	} `yaml:"relayer"`
}

// renderDeploymentConfig projects manifests into the existing config schema.
func renderDeploymentConfig(cfg config.Config, manifests []*manifest.Manifest) renderedDeployment {
	var out renderedDeployment
	for _, m := range manifests {
		chain := config.ChainConfig{ChainID: m.ChainID, EVM: &config.EVMChainConfig{ICS26Router: m.Core.Router}}
		if declared, ok := cfg.Chain(m.ChainID); ok && declared.EVM != nil {
			chain.EVM.RPC = declared.EVM.RPC
		}
		out.Chains = append(out.Chains, chain)

		for _, c := range m.Clients {
			client := config.ClientConfig{
				Alias:                m.ChainID + "-" + c.ClientID,
				ClientID:             c.ClientID,
				ChainID:              m.ChainID,
				CounterpartyChainID:  c.CounterpartyChainID,
				CounterpartyClientID: c.CounterpartyClientID,
				Type:                 config.ClientType(c.Type),
			}
			if c.Type == deploy.ClientTypeAttestation {
				set := &config.AttestorSetConfig{}
				if threshold, ok := c.Params["threshold"].(float64); ok {
					set.Threshold = int(threshold)
				}
				// attestor names cannot be derived from addresses; user fills them in
				client.AttestorSet = set
			}
			out.Relayer.Clients = append(out.Relayer.Clients, client)
		}
	}
	return out
}

func deployRenderConfig(_ *cobra.Command, _ []string) error {
	cfg, err := setupHomeWithConfig()
	if err != nil {
		return err
	}
	var manifests []*manifest.Manifest
	for _, chain := range cfg.Chains {
		if flagDeployChain != "" && chain.ChainID != flagDeployChain {
			continue
		}
		m, err := manifest.Load(flagDeployManifestDir, chain.ChainID)
		if err != nil {
			return err
		}
		if m != nil {
			manifests = append(manifests, m)
		}
	}
	if len(manifests) == 0 {
		return errors.New("no manifests found; run `ibc deploy core` or `ibc deploy import` first")
	}
	return config.PrintYAML(renderDeploymentConfig(cfg, manifests))
}

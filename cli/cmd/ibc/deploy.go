// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bufio"
	"context"
	"encoding/hex"
	"fmt"
	"io"
	"log/slog"
	"math/big"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"

	"github.com/cosmos/ibc-contracts/packages/go-abigen/besumsgs"
	"github.com/cosmos/ibc/cli/besu"
	"github.com/cosmos/ibc/cli/internal/config"
	"github.com/cosmos/ibc/cli/internal/deploy"
	"github.com/cosmos/ibc/cli/internal/deploy/evm"
	"github.com/cosmos/ibc/cli/internal/deploy/manifest"
	"github.com/cosmos/ibc/cli/internal/service/signer"
	"github.com/cosmos/ibc/cli/keyfile"
)

const (
	flagNameTrustingPeriod = "trusting-period"
	flagNameMaxClockDrift  = "max-clock-drift"
	flagNameTrustLevel     = "trust-level"
)

var (
	flagDeployManifestDir     string
	flagDeployDeployer        string
	flagDeployDryRun          bool
	flagDeployYes             bool
	flagDeployChain           string
	flagDeployCounterparty    string
	flagDeployClientID        string
	flagDeployCounterpartyCID string
	flagDeployAttestors       []string
	flagDeployThreshold       uint8
	flagDeployHeight          uint64
	flagDeployTimestamp       uint64
	flagDeployTrustingPeriod  time.Duration
	flagDeployMaxClockDrift   time.Duration
	flagDeployTrustLevel      string

	flagDeployRenderSignerA  string
	flagDeployRenderSignerB  string
	flagDeployRenderPopulate bool

	flagDeployIFTName        string
	flagDeployIFTSymbol      string
	flagDeployIFTOwner       string
	flagDeployIFTCapacity    string
	flagDeployIFTWindow      time.Duration
	flagDeployBridgeChainA   string
	flagDeployBridgeIFTA     string
	flagDeployBridgeChainB   string
	flagDeployBridgeIFTB     string
	flagDeployBridgeClientID string
	flagDeployBridgeCtorA    string
	flagDeployBridgeCtorB    string
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
	}

	cmdDeployClientAttestation = &cobra.Command{
		Use:   "attestation",
		Short: "Deploy and register an attestation light client",
		Args:  cobra.NoArgs,
		RunE:  deployAttestationClient,
	}

	cmdDeployClientBesu = &cobra.Command{
		Use:   "besu",
		Short: "Deploy and register a Besu light client for the counterparty's consensus, trusting its head",
		Args:  cobra.NoArgs,
		RunE:  deployBesuClient,
	}

	cmdDeployStatus = &cobra.Command{
		Use:   keyStatus,
		Short: "Verify recorded deployments against live chain state",
		RunE:  deployStatus,
	}

	cmdDeployShow = &cobra.Command{
		Use:   "show [chain-id]",
		Short: "Print the recorded deployment manifest for a chain",
		Args:  cobra.ExactArgs(1),
		RunE:  deployShow,
	}

	cmdDeployRenderConfig = &cobra.Command{
		Use:   "render-config [chainA] [chainB]",
		Short: "Print the existing config plus the settings to relay between two chains",
		Args:  cobra.ExactArgs(2),
		RunE:  deployRenderConfig,
	}

	cmdDeployGMP = &cobra.Command{
		Use:   "gmp",
		Short: "Deploy the ICS27-GMP app on one chain",
		RunE:  deployGMP,
	}

	cmdDeployIFT = &cobra.Command{
		Use:   keyIFT,
		Short: "Deploy an IFT token on one chain",
		Long: "Deploy an IFT token on one chain and set its rate limit, which IFT requires before any transfer. " +
			"Rerunning with a different rate limit updates it. " +
			"With an --owner other than the deployer no limit is set and the rate-limit flags are rejected: " +
			"the owner must call setIFTRateLimit before any transfer.",
		RunE: deployIFT,
	}

	cmdDeployIFTBridge = &cobra.Command{
		Use:   "ift-bridge",
		Short: "Register both sides of an IFT bridge between two chains' tokens",
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
	return confirmPrompt(os.Stdout,
		fmt.Sprintf("About to execute %d step(s) that submit transactions.", pending))
}

func confirmPrompt(w io.Writer, action string) error {
	_, _ = fmt.Fprintf(w, "%s Proceed? [y/N]: ", action)

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

// clientSpec resolves the client ids for --chain tracking
// --counterparty-chain, leaving Params to the caller.
func clientSpec(chainID, counterpartyChainID string) (deploy.ClientSpec, error) {
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
	return deploy.ClientSpec{
		ClientID:             clientID,
		CounterpartyChainID:  counterpartyChainID,
		CounterpartyClientID: counterpartyClientID,
	}, nil
}

// attestationParams resolves the attestor set and the initial trusted head.
func attestationParams(
	ctx context.Context,
	cfg config.Config,
	counterpartyChainID string,
	counterpartyTarget deploy.Target,
) (deploy.ClientParams, error) {
	var attestors []string
	if len(flagDeployAttestors) > 0 {
		for _, token := range flagDeployAttestors {
			address, err := resolveAttestorToken(cfg, token)
			if err != nil {
				return nil, err
			}
			attestors = append(attestors, address)
		}
	} else {
		var err error
		attestors, err = attestorsForChain(cfg, counterpartyChainID)
		if err != nil {
			return nil, err
		}
	}
	height, timestamp := flagDeployHeight, flagDeployTimestamp
	if height == 0 || timestamp == 0 {
		h, ts, err := counterpartyTarget.Head(ctx)
		if err != nil {
			return nil, errors.Wrap(err, "fetch counterparty head for initial trusted state")
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
	return deploy.AttestationParams{
		Attestors:        attestors,
		Threshold:        flagDeployThreshold,
		InitialHeight:    height,
		InitialTimestamp: timestamp,
	}, nil
}

// besuParams reads the Besu client flags and trusts the counterparty head,
// whose commit seals decide the consensus the client verifies.
func besuParams(
	ctx context.Context,
	counterpartyRouter string,
	counterpartyTarget deploy.Target,
) (deploy.BesuParams, error) {
	trustingPeriod, err := wholeSeconds(flagDeployTrustingPeriod)
	if err != nil {
		return deploy.BesuParams{}, errors.Wrapf(err, "--%s", flagNameTrustingPeriod)
	}
	if trustingPeriod == 0 {
		return deploy.BesuParams{}, errors.Errorf("--%s must be positive", flagNameTrustingPeriod)
	}
	maxClockDrift, err := wholeSeconds(flagDeployMaxClockDrift)
	if err != nil {
		return deploy.BesuParams{}, errors.Wrapf(err, "--%s", flagNameMaxClockDrift)
	}
	level, err := trustLevel(flagDeployTrustLevel)
	if err != nil {
		return deploy.BesuParams{}, errors.Wrapf(err, "--%s", flagNameTrustLevel)
	}
	router, err := config.ParseEVMAddress(counterpartyRouter)
	if err != nil {
		return deploy.BesuParams{}, errors.Wrap(err, "counterparty evm.ics26Router")
	}
	source, ok := counterpartyTarget.(deploy.BesuSource)
	if !ok {
		return deploy.BesuParams{}, errors.New("counterparty cannot serve a besu consensus state")
	}
	head, err := source.BesuHead(ctx)
	if err != nil {
		return deploy.BesuParams{}, errors.Wrap(err, "read counterparty head")
	}
	return deploy.BesuParams{
		Consensus:             head.Consensus,
		IBCRouter:             router,
		InitialHeight:         head.Height,
		InitialConsensusState: head.ConsensusState(),
		TrustingPeriod:        trustingPeriod,
		MaxClockDrift:         maxClockDrift,
		TrustLevel:            level,
	}, nil
}

// defaultTrustLevel is the --trust-level default, besu.DefaultTrustLevel as n/d.
var defaultTrustLevel = fmt.Sprintf("%d/%d", besu.DefaultTrustLevel.Numerator, besu.DefaultTrustLevel.Denominator)

// trustLevel parses n/d, which the contract accepts from one third to one.
func trustLevel(s string) (besumsgs.IBesuLightClientMsgsTrustThreshold, error) {
	numerator, denominator, _ := strings.Cut(s, "/")
	n, errN := strconv.ParseUint(numerator, 10, 8)
	d, errD := strconv.ParseUint(denominator, 10, 8)
	switch {
	case errN != nil || errD != nil:
		return besumsgs.IBesuLightClientMsgsTrustThreshold{}, errors.Errorf("want n/d, each at most 255, got %q", s)
	case d == 0:
		return besumsgs.IBesuLightClientMsgsTrustThreshold{}, errors.Errorf("denominator must not be zero, got %q", s)
	case n > d || 3*n < d:
		return besumsgs.IBesuLightClientMsgsTrustThreshold{}, errors.Errorf("must be from 1/3 to 1, got %q", s)
	}
	return besumsgs.IBesuLightClientMsgsTrustThreshold{Numerator: uint8(n), Denominator: uint8(d)}, nil
}

// wholeSeconds converts a duration into the contract's seconds, truncating
// any fraction.
func wholeSeconds(d time.Duration) (uint64, error) {
	if d < 0 {
		return 0, errors.Errorf("must not be negative, got %s", d)
	}
	return uint64(d / time.Second), nil
}

// defaultClientID derives the shared client id for a chain pair, stable
// regardless of argument order.
func defaultClientID(a, b string) string {
	ids := []string{a, b}
	sort.Strings(ids)
	return "cli-" + ids[0] + "-" + ids[1]
}

func deployAttestationClient(cmd *cobra.Command, _ []string) error {
	return deployClient(cmd, attestationParams)
}

func deployBesuClient(cmd *cobra.Command, _ []string) error {
	return deployClient(cmd, func(
		ctx context.Context, cfg config.Config, counterpartyChainID string, counterpartyTarget deploy.Target,
	) (deploy.ClientParams, error) {
		// newTarget already checked that the counterparty is a configured EVM chain
		counterparty, _ := cfg.Chain(counterpartyChainID)
		params, err := besuParams(ctx, counterparty.EVM.ICS26Router, counterpartyTarget)
		return params, errors.Wrapf(err, "besu client tracking chain %s", counterpartyChainID)
	})
}

// deployClient deploys a client on --chain whose params read the
// counterparty.
func deployClient(
	cmd *cobra.Command,
	params func(context.Context, config.Config, string, deploy.Target) (deploy.ClientParams, error),
) error {
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
	counterpartyTarget, err := newTarget(cmd.Context(), cfg, flagDeployCounterparty, flagDeployDeployer, false)
	if err != nil {
		return errors.Wrapf(err, "counterparty chain %s", flagDeployCounterparty)
	}
	spec, err := clientSpec(flagDeployChain, flagDeployCounterparty)
	if err != nil {
		return err
	}
	spec.Params, err = params(cmd.Context(), cfg, flagDeployCounterparty, counterpartyTarget)
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
	return map[string]string{keyStatus: "error", "error": err.Error()}
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
			out[chainID] = map[string]string{keyStatus: "no manifest"}
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

func deployShow(_ *cobra.Command, args []string) error {
	if _, err := setupHomeWithConfig(); err != nil {
		return err
	}

	chainID := args[0]
	m, err := manifest.Load(flagDeployManifestDir, chainID)
	if err != nil {
		return err
	}
	if m == nil {
		return errors.Errorf("no manifest for chain %s in %s: run `ibc deploy` against it first",
			chainID, flagDeployManifestDir)
	}

	return config.PrintJSON(m)
}

// attestorsFromClient projects one client's on-chain attestor addresses into
// attestors
func attestorsFromClient(cfg config.Config, p manifest.AttestationParams, watchedChainID string) config.Attestors {
	out := make(config.Attestors, 0, len(p.Attestors))
	for _, address := range p.Attestors {
		alias, _ := signerAliasForAddress(cfg, address)
		out = append(out, config.AttestorConfig{
			ChainID: watchedChainID,
			Name:    fmt.Sprintf("attestor-%s-%s", watchedChainID, address),
			Type:    config.AttestorTypeLocal,
			Signer:  alias,
			// default to 1 so it doesn't wait for native chain finality;
			// CollectComments still flags it for review.
			FinalityOffset: 1,
		})
	}
	return out
}

func appendUniqueAttestors(
	existing config.Attestors,
	seen map[string]struct{},
	additional config.Attestors,
) config.Attestors {
	for _, a := range additional {
		if _, dup := seen[a.Name]; dup {
			continue
		}
		seen[a.Name] = struct{}{}
		existing = append(existing, a)
	}
	return existing
}

// signerAliasForAddress finds the local signer in cfg whose derived EVM
// address matches address, case-insensitively.
func signerAliasForAddress(cfg config.Config, address string) (string, bool) {
	for _, sc := range cfg.Signers {
		if sc.Type != config.SignerLocal {
			continue
		}
		derived, err := signer.EVMAddressOf(sc)
		if err != nil {
			continue
		}
		if strings.EqualFold(derived, address) {
			return sc.Alias, true
		}
	}
	return "", false
}

// renderedChain copies the operated-on chain's config (if declared) and sets
// its router and multicall to the deployed addresses, so the emitted chains[] entry is
// ready to relay against.
func renderedChain(cfg config.Config, m *manifest.Manifest) config.ChainConfig {
	chain, _ := cfg.Chain(m.ChainID)
	chain.ChainID = m.ChainID

	var evm config.EVMChainConfig
	if chain.EVM != nil {
		evm = *chain.EVM
	}

	evm.ICS26Router = m.Core.Router
	evm.IBCMulticall = m.Core.Multicall
	chain.EVM = &evm

	return chain
}

func renderedClientEnd(m *manifest.Manifest, c manifest.Client, signer string) config.ClientEnd {
	autoRelay := true

	return config.ClientEnd{
		ChainID:   m.ChainID,
		Signer:    signer,
		ClientID:  c.ClientID,
		Type:      c.Type(),
		AutoRelay: config.AutoRelayConfig{Enabled: &autoRelay},
	}
}

// renderRelayConfig projects two deployment manifests into the config
// sections needed to relay between them for every mutual client pair.
func renderRelayConfig(
	cfg config.Config,
	a, b *manifest.Manifest,
	signerA, signerB string,
) (config.DeploymentConfig, error) {
	out := config.DeploymentConfig{Chains: []config.ChainConfig{renderedChain(cfg, a), renderedChain(cfg, b)}}

	baseAlias := a.ChainID + "-" + b.ChainID
	seenAttestors := make(map[string]struct{})
	for _, ca := range a.Clients {
		if ca.CounterpartyChainID != b.ChainID {
			continue
		}
		cb, ok := b.Client(ca.CounterpartyClientID)
		if !ok || cb.CounterpartyChainID != a.ChainID || cb.CounterpartyClientID != ca.ClientID {
			continue
		}
		out.Connections = append(out.Connections, config.ConnectionConfig{
			Alias:   baseAlias,
			ClientA: renderedClientEnd(a, ca, signerA),
			ClientB: renderedClientEnd(b, cb, signerB),
		})
		if ca.Attestation != nil {
			out.Attestors = appendUniqueAttestors(
				out.Attestors, seenAttestors, attestorsFromClient(cfg, *ca.Attestation, b.ChainID))
		}
		if cb.Attestation != nil {
			out.Attestors = appendUniqueAttestors(
				out.Attestors, seenAttestors, attestorsFromClient(cfg, *cb.Attestation, a.ChainID))
		}
	}
	if len(out.Connections) == 0 {
		return config.DeploymentConfig{}, errors.Errorf(
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
	// Load incomplete drafts for repair, but reject ambiguous identities before
	// resolving signers or rendering deployment settings.
	globalFlags.SkipConfigValidation()
	cfg, err := setupHomeWithConfig()
	if err != nil {
		return err
	}
	if err = cfg.ValidateIdentities(); err != nil {
		return err
	}
	signers := []string{flagDeployRenderSignerA, flagDeployRenderSignerB}
	for _, alias := range signers {
		if alias == "" {
			continue
		}
		if _, ok := cfg.Signer(alias); !ok {
			return errors.Errorf("signer %q not found in config", alias)
		}
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
	out, err := renderRelayConfig(cfg, manifests[0], manifests[1], signers[0], signers[1])
	if err != nil {
		return err
	}

	merged, conflicts, err := cfg.ReconcileDeployment(out)
	if err != nil {
		return err
	}

	if err := config.PrintYAMLWithComments(merged, config.CollectComments(merged)); err != nil {
		return err
	}

	if !flagDeployRenderPopulate {
		return nil
	}

	if !flagDeployYes {
		reportConflicts(conflicts)

		configPath, pathErr := globalFlags.ConfigPath()
		if pathErr != nil {
			return pathErr
		}

		if confirmErr := confirmPrompt(os.Stderr, "About to write "+configPath+"."); confirmErr != nil {
			return confirmErr
		}
	}

	return populateRenderedConfig(merged)
}

func reportConflicts(conflicts []config.Conflict) {
	if len(conflicts) == 0 {
		return
	}

	fmt.Fprintln(os.Stderr, "\nThis operation overwrites values for the following:")

	for _, c := range conflicts {
		fmt.Fprintln(os.Stderr, "  "+c.String())
	}

	fmt.Fprintln(os.Stderr)
}

func populateRenderedConfig(merged config.Config) error {
	configPath, err := globalFlags.ConfigPath()
	if err != nil {
		return err
	}

	if err := merged.Validate(); err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "Config is not runnable yet: %v\n", err)
	}

	if storeErr := merged.StoreToFileWithComments(configPath); storeErr != nil {
		return storeErr
	}

	if globalFlags.Quiet {
		return nil
	}

	_, _ = fmt.Fprintf(os.Stderr, "Wrote %s\n", configPath)

	return nil
}

// deployerAddress derives the deployer's EVM address, for use as the default
// IFT owner.
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
	if flagDeployIFTCapacity == "" && flagDeployIFTOwner == "" {
		return errIFTCapacityRequired
	}
	chain, ok := cfg.Chain(flagDeployChain)
	if !ok {
		return errors.Errorf("chain %q not declared in config", flagDeployChain)
	}
	deployer, err := deployerAddress(cfg, resolveDeployerAlias(chain, flagDeployDeployer))
	if err != nil {
		return err
	}
	owner := flagDeployIFTOwner
	if owner == "" {
		owner = deployer
	}
	if !common.IsHexAddress(owner) {
		return errors.Errorf("invalid --owner %q", owner)
	}
	rateLimit, err := iftRateLimit(owner, deployer, cmd.Flags().Changed("rate-limit-window"))
	if err != nil {
		return err
	}
	target, err := newTarget(cmd.Context(), cfg, flagDeployChain, flagDeployDeployer, true)
	if err != nil {
		return err
	}
	spec := deploy.IFTSpec{Owner: owner, Name: flagDeployIFTName, Symbol: flagDeployIFTSymbol, RateLimit: rateLimit}
	return planThenRun(cmd.Context(), deploy.IFTSteps(target, flagDeployManifestDir, flagDeployChain, spec))
}

// maxIFTCapacity is the largest capacity the contract stores (uint208).
var maxIFTCapacity = new(big.Int).Sub(new(big.Int).Lsh(big.NewInt(1), 208), big.NewInt(1))

var errIFTCapacityRequired = errors.New("--rate-limit-capacity is required")

// iftRateLimit resolves the rate limit `deploy ift` sets from its flags. Only
// the owner can set it, so a token owned by someone other than the deployer
// gets none.
func iftRateLimit(owner, deployer string, windowSet bool) (*deploy.IFTRateLimit, error) {
	if common.HexToAddress(owner) != common.HexToAddress(deployer) {
		if flagDeployIFTCapacity != "" || windowSet {
			return nil, errors.Errorf(
				"--owner %s is not the deployer %s: only the owner can set the rate limit", owner, deployer)
		}
		slog.Warn("IFT owner is not the deployer: the owner must call setIFTRateLimit before any transfer succeeds",
			"owner", owner)
		return nil, nil
	}
	if flagDeployIFTCapacity == "" {
		return nil, errIFTCapacityRequired
	}

	window, err := wholeSeconds(flagDeployIFTWindow)
	if err != nil || window == 0 {
		return nil, errors.Errorf("invalid --rate-limit-window %s: must be at least 1s", flagDeployIFTWindow)
	}
	capacity := parseIFTAmount(flagDeployIFTCapacity)
	if capacity == nil || capacity.Sign() == 0 || capacity.Cmp(maxIFTCapacity) > 0 {
		return nil, errors.Errorf("invalid --rate-limit-capacity %q: must be a positive uint208", flagDeployIFTCapacity)
	}
	return &deploy.IFTRateLimit{Capacity: capacity, Window: window}, nil
}

// deployIFTBridge registers both sides of an IFT bridge in one invocation:
// contract --ift-a on --chain-a and contract --ift-b on --chain-b each register
// a bridge over the shared client, pointing at the other. Both chains must be
// EVM chains this tool can deploy to.
func deployIFTBridge(cmd *cobra.Command, _ []string) error {
	cfg, err := setupHomeWithConfig()
	if err != nil {
		return err
	}
	if flagDeployBridgeChainA == flagDeployBridgeChainB {
		return errors.New("--chain-a and --chain-b must differ")
	}
	for _, addr := range []string{flagDeployBridgeIFTA, flagDeployBridgeIFTB} {
		if !common.IsHexAddress(addr) {
			return errors.Errorf("invalid IFT address %q", addr)
		}
	}
	clientID := flagDeployBridgeClientID
	if clientID == "" {
		clientID = defaultClientID(flagDeployBridgeChainA, flagDeployBridgeChainB)
	}
	if !deploy.ValidClientID(clientID) {
		return errors.Errorf("invalid client id %q", clientID)
	}
	targetA, err := newTarget(cmd.Context(), cfg, flagDeployBridgeChainA, flagDeployDeployer, true)
	if err != nil {
		return errors.Wrapf(err, "chain %s", flagDeployBridgeChainA)
	}
	targetB, err := newTarget(cmd.Context(), cfg, flagDeployBridgeChainB, flagDeployDeployer, true)
	if err != nil {
		return errors.Wrapf(err, "chain %s", flagDeployBridgeChainB)
	}
	stepsA := deploy.IFTBridgeSteps(
		targetA, flagDeployManifestDir, flagDeployBridgeChainA, flagDeployBridgeIFTA, flagDeployBridgeCtorA,
		deploy.BridgeSpec{ClientID: clientID, CounterpartyIFT: flagDeployBridgeIFTB})
	stepsB := deploy.IFTBridgeSteps(
		targetB, flagDeployManifestDir, flagDeployBridgeChainB, flagDeployBridgeIFTB, flagDeployBridgeCtorB,
		deploy.BridgeSpec{ClientID: clientID, CounterpartyIFT: flagDeployBridgeIFTA})
	return planThenRun(cmd.Context(), append(stepsA, stepsB...))
}

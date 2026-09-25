// SPDX-License-Identifier: Apache-2.0

package e2e_test

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/cosmos/solidity-ibc-eureka/packages/go-abigen/ics26router"
	ethereum "github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/stretchr/testify/require"

	"github.com/cosmos/ibc/e2e/internal/e2etest"
	"github.com/cosmos/ibc/e2e/internal/harness/environment"
	"github.com/cosmos/ibc/e2e/internal/harness/ibccli"
)

// deployStepResult mirrors cli/internal/deploy.StepResult, which e2e cannot
// import directly (module boundary): "name" and "action" ("skipped",
// "executed", or "planned").
type deployStepResult struct {
	Name   string `json:"name"`
	Action string `json:"action"`
}

// deployManifest is the subset of cli/internal/deploy/manifest.Manifest
// this test needs to observe from the written JSON file.
type deployManifest struct {
	Core struct {
		Router string `json:"router"`
	} `json:"core"`
	GMP *struct {
		Address string `json:"address"`
		Port    string `json:"port"`
	} `json:"gmp"`
	Tokens []struct {
		Symbol  string `json:"symbol"`
		Address string `json:"address"`
	} `json:"tokens"`
	TargetData map[string]string `json:"targetData"`
}

// token looks up a deployed IFT token's address by symbol.
func (m deployManifest) token(symbol string) (string, bool) {
	for _, t := range m.Tokens {
		if t.Symbol == symbol {
			return t.Address, true
		}
	}
	return "", false
}

const deployerAlias = "deployer"

// deployCLI is a temporary CLI home with an imported, funded deployer key on
// two bare managed chains: no protocol resources, the deploy CLI provisions
// IBC itself.
type deployCLI struct {
	driver             *ibccli.Driver
	home, configPath   string
	chainA, chainB     *environment.Chain
	chainAID, chainBID string
	rpcA, rpcB         string
	deployer           common.Address
}

func startDeployCLI(t *testing.T, requirements e2etest.EVMRequirements) *deployCLI {
	t.Helper()
	spec := environment.Spec{
		Chains: e2etest.EVMChains(t, requirements, e2etest.ChainA, e2etest.ChainB),
	}
	env := e2etest.Start(t, spec, environment.Runtime{})

	d := &deployCLI{home: t.TempDir()}
	var err error
	d.chainA, err = env.Chain(e2etest.ChainA)
	require.NoError(t, err)
	d.chainB, err = env.Chain(e2etest.ChainB)
	require.NoError(t, err)
	d.chainAID = strconv.FormatUint(d.chainA.EVMChainID(), 10)
	d.chainBID = strconv.FormatUint(d.chainB.EVMChainID(), 10)

	d.configPath = filepath.Join(d.home, "ibc.yml")
	d.driver, err = ibccli.NewDriver(d.configPath)
	require.NoError(t, err)
	require.NoError(t, env.BindCLI(d.driver))

	deployerKey, err := crypto.GenerateKey()
	require.NoError(t, err)
	d.deployer = crypto.PubkeyToAddress(deployerKey.PublicKey)
	minimum := e2etest.RequiredSignerBalance()
	for _, chain := range []*environment.Chain{d.chainA, d.chainB} {
		funding, fundingErr := chain.Funding()
		require.NoError(t, fundingErr)
		require.NoError(t, funding.EnsureEOABalance(t.Context(), d.deployer, minimum))
	}

	d.rpcA, err = d.driver.ChainRPC(string(e2etest.ChainA))
	require.NoError(t, err)
	d.rpcB, err = d.driver.ChainRPC(string(e2etest.ChainB))
	require.NoError(t, err)
	d.writeConfig(t, "", "")
	require.NoError(
		t,
		d.driver.KeysImportECDSA(t.Context(), deployerAlias, hex.EncodeToString(crypto.FromECDSA(deployerKey))),
	)
	return d
}

// writeConfig declares both chains with the given routers, or placeholders
// before they are deployed.
func (d *deployCLI) writeConfig(t *testing.T, routerA, routerB string) {
	t.Helper()
	require.NoError(t, ibccli.WriteDeployConfig(d.configPath, ibccli.DeployConfig{
		DBPath:        filepath.Join(d.home, "unused.db"),
		SignerAlias:   deployerAlias,
		SignerKeyFile: d.driver.KeyFilePath(deployerAlias),
		Chains: []ibccli.DeployChain{
			{ChainID: d.chainAID, RPC: d.rpcA, ICS26Router: routerA},
			{ChainID: d.chainBID, RPC: d.rpcB, ICS26Router: routerB},
		},
	}))
}

// deploy runs each command and requires every step it reports to have taken
// action ("executed" or "skipped").
func (d *deployCLI) deploy(t *testing.T, commands [][]string, action string) {
	t.Helper()
	for _, args := range commands {
		stdout, err := d.driver.Deploy(t.Context(), args...)
		require.NoErrorf(t, err, "deploy %v", args)
		results := decodeStepResults(t, stdout)
		require.NotEmptyf(t, results, "deploy %v", args)
		for _, r := range results {
			require.Equalf(t, action, r.Action, "step %q of %v", r.Name, args)
		}
	}
}

func (d *deployCLI) manifest(t *testing.T, chainID string) deployManifest {
	t.Helper()
	return readManifest(t, filepath.Join(d.home, "deployments"), chainID)
}

func (d *deployCLI) renderConfig(t *testing.T) string {
	t.Helper()
	rendered, err := d.driver.Deploy(t.Context(), "render-config", d.chainAID, d.chainBID,
		"--signer-a", deployerAlias, "--signer-b", deployerAlias)
	require.NoError(t, err)
	return string(rendered)
}

// TestDeployConnection drives `ibc deploy` as a black box, asserting against
// the CLI's JSON step output and the manifests it writes.
func TestDeployConnection(t *testing.T) {
	t.Parallel()
	d := startDeployCLI(t, e2etest.EVMRequirements{})

	// the connection is four separate idempotent commands: core on each
	// chain, then a client on each chain tracking the other. Both client
	// invocations derive the same shared client id.
	sharedClientID := "cli-" + d.chainAID + "-" + d.chainBID
	client := func(chainID, counterparty string) []string {
		return []string{
			"client", "attestation",
			"--chain", chainID,
			"--counterparty-chain", counterparty,
			"--attestors", d.deployer.Hex(),
			"--yes",
		}
	}
	deployCommands := [][]string{
		{"core", "--chain", d.chainAID, "--yes"},
		{"core", "--chain", d.chainBID, "--yes"},
		client(d.chainAID, d.chainBID),
		client(d.chainBID, d.chainAID),
	}
	d.deploy(t, deployCommands, "executed")

	manifestA := d.manifest(t, d.chainAID)
	manifestB := d.manifest(t, d.chainBID)
	require.NotEmpty(t, manifestA.Core.Router)
	require.NotEmpty(t, manifestB.Core.Router)

	// core provisioning binds the relaying selectors to PUBLIC_ROLE;
	// prove an unrelated address can call recvPacket on each router. The
	// driver's ChainRPC values are env-var templates only the CLI process
	// expands, so dial the chains' real RPC URLs.
	ctx := t.Context()
	assertPublicRelaying(ctx, t, d.chainA.RPCURL(), manifestA)
	assertPublicRelaying(ctx, t, d.chainB.RPCURL(), manifestB)

	// Idempotency: rerunning every identical command skips its step.
	d.deploy(t, deployCommands, "skipped")

	_, err := d.driver.Deploy(ctx, "status")
	require.NoError(t, err)

	// render-config pairs the two manifests into one relayer.connections[]
	// entry: clientA/clientB, each end's counterparty implied by the other.
	rendered := d.renderConfig(t)
	for _, want := range []string{
		"alias: " + d.chainAID + "-" + d.chainBID,
		"clientId: " + sharedClientID,
		`chainId: "` + d.chainAID + `"`,
		`chainId: "` + d.chainBID + `"`,
		"signer: " + deployerAlias,
	} {
		require.Contains(t, rendered, want)
	}
}

// TestDeployBesuQBFTConnection deploys besu-qbft clients through the CLI on
// two Besu chains. Each trusts the other chain's head and needs its router,
// so the config carries the routers core deployed.
func TestDeployBesuQBFTConnection(t *testing.T) {
	t.Parallel()
	d := startDeployCLI(t, e2etest.EVMRequirements{Provider: e2etest.EVMProviderBesu})

	d.deploy(t, [][]string{
		{"core", "--chain", d.chainAID, "--yes"},
		{"core", "--chain", d.chainBID, "--yes"},
	}, "executed")
	d.writeConfig(t, d.manifest(t, d.chainAID).Core.Router, d.manifest(t, d.chainBID).Core.Router)

	client := func(chainID, counterparty string) []string {
		return []string{
			"client", "besu-qbft",
			"--chain", chainID,
			"--counterparty-chain", counterparty,
			"--trusting-period", "336h",
			"--max-clock-drift", "1m",
			"--yes",
		}
	}
	clients := [][]string{client(d.chainAID, d.chainBID), client(d.chainBID, d.chainAID)}
	d.deploy(t, clients, "executed")
	// the heads have moved on, but a rerun compares only the recorded params
	d.deploy(t, clients, "skipped")

	_, err := d.driver.Deploy(t.Context(), "status")
	require.NoError(t, err)
	require.Equal(t, 2, strings.Count(d.renderConfig(t), "type: besu-qbft"))
}

// TestDeployIFTBridge drives the app-layer deploy commands end-to-end: core +
// client + gmp + ift on each of two chains, then a single ift-bridge that
// registers both sides. Asserts executed-then-skipped idempotency and that
// status passes.
func TestDeployIFTBridge(t *testing.T) {
	t.Parallel()
	d := startDeployCLI(t, e2etest.EVMRequirements{})

	// per-chain bring-up: core, client, gmp, ift
	perChain := func(chainID, counterparty string) [][]string {
		return [][]string{
			{"core", "--chain", chainID, "--yes"},
			{
				"client", "attestation",
				"--chain", chainID,
				"--counterparty-chain", counterparty,
				"--attestors", d.deployer.Hex(),
				"--yes",
			},
			{"gmp", "--chain", chainID, "--yes"},
			{"ift", "--chain", chainID, "--name", "Foo", "--symbol", "FOO", "--yes"},
		}
	}
	bringUp := append(perChain(d.chainAID, d.chainBID), perChain(d.chainBID, d.chainAID)...)
	d.deploy(t, bringUp, "executed")

	manifestA := d.manifest(t, d.chainAID)
	manifestB := d.manifest(t, d.chainBID)
	iftA, ok := manifestA.token("FOO")
	require.True(t, ok)
	iftB, ok := manifestB.token("FOO")
	require.True(t, ok)
	require.NotNil(t, manifestA.GMP)
	require.Equal(t, "gmpport", manifestA.GMP.Port)

	// one command registers both sides; the client id defaults to the same
	// sorted name `deploy client` derived.
	bridge := []string{
		"ift-bridge",
		"--chain-a", d.chainAID, "--ift-a", iftA,
		"--chain-b", d.chainBID, "--ift-b", iftB,
		"--yes",
	}
	d.deploy(t, [][]string{bridge}, "executed")

	// idempotency: rerun bring-up + bridge, everything skips
	d.deploy(t, append(bringUp, bridge), "skipped")

	_, err := d.driver.Deploy(t.Context(), "status")
	require.NoError(t, err)
}

func decodeStepResults(t testing.TB, stdout []byte) []deployStepResult {
	t.Helper()
	var results []deployStepResult
	require.NoErrorf(t, json.Unmarshal(stdout, &results), "deploy stdout: %s", stdout)
	return results
}

func readManifest(t testing.TB, manifestDir, chainID string) deployManifest {
	t.Helper()
	path := filepath.Join(manifestDir, chainID+".json")
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	var m deployManifest
	require.NoError(t, json.Unmarshal(data, &m))
	return m
}

// assertPublicRelaying proves the deployed AccessManager lets an arbitrary
// address call the router's recvPacket immediately, via a hand-packed
// canCall(address,address,bytes4) eth_call (the harness carries no
// AccessManager binding).
func assertPublicRelaying(ctx context.Context, t testing.TB, rpcURL string, m deployManifest) {
	t.Helper()
	authority := m.TargetData["accessManager"]
	require.NotEmpty(t, authority, "manifest carries no accessManager in targetData")

	routerABI, err := ics26router.ContractMetaData.GetAbi()
	require.NoError(t, err)
	recvSelector := routerABI.Methods["recvPacket"].ID[:4]

	caller := common.HexToAddress("0x1000000000000000000000000000000000000001")
	router := common.HexToAddress(m.Core.Router)
	data := crypto.Keccak256([]byte("canCall(address,address,bytes4)"))[:4]
	data = append(data, common.LeftPadBytes(caller.Bytes(), 32)...)
	data = append(data, common.LeftPadBytes(router.Bytes(), 32)...)
	data = append(data, common.RightPadBytes(recvSelector, 32)...)

	client, err := ethclient.DialContext(ctx, rpcURL)
	require.NoError(t, err)
	defer client.Close()

	to := common.HexToAddress(authority)
	out, err := client.CallContract(ctx, ethereum.CallMsg{To: &to, Data: data}, nil)
	require.NoError(t, err)
	// returns (bool immediate, uint32 delay)
	require.GreaterOrEqual(t, len(out), 32)
	require.Equal(t, byte(1), out[31], "recvPacket is not publicly callable")
}

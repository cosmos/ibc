// SPDX-License-Identifier: Apache-2.0

package e2e_test

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/cosmos/solidity-ibc-eureka/packages/go-abigen/ics26router"
	ethereum "github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/stretchr/testify/require"

	relayerv2 "github.com/cosmos/ibc/cli/api/v2/relayer"
	"github.com/cosmos/ibc/cli/besu"
	"github.com/cosmos/ibc/e2e/internal/e2etest"
	"github.com/cosmos/ibc/e2e/internal/harness/clientkind"
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

const (
	deployerAlias = "deployer"
	relayerAlias  = "relayer"
)

// deployCLI is a temporary CLI home with an imported, funded deployer key on
// two bare managed chains: no protocol resources, the deploy CLI provisions
// IBC itself.
type deployCLI struct {
	env                *environment.Environment
	driver             *ibccli.Driver
	home, configPath   string
	chainA, chainB     *environment.Chain
	chainAID, chainBID string
	rpcA, rpcB         string
	wsA, wsB           string
	deployer           common.Address
}

func startDeployCLI(t *testing.T, requirements e2etest.EVMRequirements) *deployCLI {
	t.Helper()
	spec := environment.Spec{
		Chains: e2etest.EVMChains(t, requirements, e2etest.ChainA, e2etest.ChainB),
	}
	env := e2etest.Start(t, spec, environment.Runtime{})

	d := &deployCLI{env: env, home: t.TempDir()}
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

	d.rpcA, err = d.driver.ChainRPC(string(e2etest.ChainA))
	require.NoError(t, err)
	d.rpcB, err = d.driver.ChainRPC(string(e2etest.ChainB))
	require.NoError(t, err)
	d.wsA, err = d.driver.ChainWS(string(e2etest.ChainA))
	require.NoError(t, err)
	d.wsB, err = d.driver.ChainWS(string(e2etest.ChainB))
	require.NoError(t, err)
	d.writeConfig(t, "", "")
	d.deployer = d.importFundedKey(t, deployerAlias)
	return d
}

// writeConfig declares both chains with the given routers, or placeholders
// before they are deployed.
func (d *deployCLI) writeConfig(t *testing.T, routerA, routerB string) {
	t.Helper()
	require.NoError(t, ibccli.WriteDeployConfig(d.configPath, ibccli.DeployConfig{
		DBPath:        filepath.Join(d.home, "relayer.db"),
		SignerAlias:   deployerAlias,
		SignerKeyFile: d.driver.KeyFilePath(deployerAlias),
		Chains: []ibccli.DeployChain{
			{ChainID: d.chainAID, RPC: d.rpcA, WS: d.wsA, ICS26Router: routerA},
			{ChainID: d.chainBID, RPC: d.rpcB, WS: d.wsB, ICS26Router: routerB},
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

// clientID is the client id `ibc deploy client` derives for the chain pair:
// both chain ids in sorted order.
func (d *deployCLI) clientID() string {
	ids := []string{d.chainAID, d.chainBID}
	slices.Sort(ids)
	return "cli-" + ids[0] + "-" + ids[1]
}

// importFundedKey funds a fresh ECDSA key on both chains and imports it as
// alias with `ibc keys import ecdsa` plus extra flags.
func (d *deployCLI) importFundedKey(t *testing.T, alias string, extra ...string) common.Address {
	t.Helper()
	key, err := crypto.GenerateKey()
	require.NoError(t, err)
	address := crypto.PubkeyToAddress(key.PublicKey)
	for _, chain := range []*environment.Chain{d.chainA, d.chainB} {
		funding, fundingErr := chain.Funding()
		require.NoError(t, fundingErr)
		require.NoError(t, funding.EnsureEOABalance(t.Context(), address, e2etest.RequiredSignerBalance()))
	}
	privateKey := hex.EncodeToString(crypto.FromECDSA(key))
	_, err = d.driver.Run(t.Context(), "keys",
		append([]string{"import", "ecdsa", alias, "--private-key", privateKey}, extra...)...)
	require.NoError(t, err)
	return address
}

// iftBalance reads account's balance of the IFT token at ift through
// `ibc query ift balance`.
func (d *deployCLI) iftBalance(t *testing.T, chainID, ift, account string) string {
	t.Helper()
	stdout, err := d.driver.Run(t.Context(), "query", "ift", "balance",
		"--chain", chainID, "--ift", ift, "--address", account)
	require.NoError(t, err)
	var balance struct {
		Balance string `json:"balance"`
	}
	require.NoErrorf(t, json.Unmarshal(stdout, &balance), "query ift balance stdout: %s", stdout)
	return balance.Balance
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
	sharedClientID := d.clientID()
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

// TestDeployBesuQBFTConnection relays an IFT transfer between two Besu QBFT
// chains set up through the CLI alone.
func TestDeployBesuQBFTConnection(t *testing.T) {
	t.Parallel()
	requireBesuDeployedConnection(t, besu.QBFT)
}

// TestDeployBesuIBFT2Connection relays an IFT transfer between two Besu IBFT
// 2.0 chains set up through the CLI alone.
func TestDeployBesuIBFT2Connection(t *testing.T) {
	t.Parallel()
	requireBesuDeployedConnection(t, besu.IBFT2)
}

// requireBesuDeployedConnection deploys a Besu connection between two chains
// running consensus and an IFT bridge, renders the relayer config from the
// manifests, then relays an IFT transfer. Each client trusts the other
// chain's head and needs its router, so the config carries the routers core
// deployed.
func requireBesuDeployedConnection(t *testing.T, consensus besu.Consensus) {
	t.Helper()
	d := startDeployCLI(t, e2etest.EVMRequirements{Provider: e2etest.EVMProviderBesu, BesuConsensus: consensus})
	ctx := t.Context()
	clientType := string(clientkind.Besu(consensus))

	core := [][]string{
		{"core", "--chain", d.chainAID, "--yes"},
		{"core", "--chain", d.chainBID, "--yes"},
	}
	d.deploy(t, core, "executed")
	d.writeConfig(t, d.manifest(t, d.chainAID).Core.Router, d.manifest(t, d.chainBID).Core.Router)

	perChain := func(chainID, counterparty string) [][]string {
		return [][]string{
			{
				"client", clientType,
				"--chain", chainID,
				"--counterparty-chain", counterparty,
				"--trusting-period", "336h",
				"--max-clock-drift", "1m",
				"--yes",
			},
			{"gmp", "--chain", chainID, "--yes"},
			{
				"ift", "--chain", chainID, "--name", "Foo", "--symbol", "FOO",
				"--rate-limit-capacity", "1000000000000000000000", "--yes",
			},
		}
	}
	bringUp := append(perChain(d.chainAID, d.chainBID), perChain(d.chainBID, d.chainAID)...)
	d.deploy(t, bringUp, "executed")

	manifestA := d.manifest(t, d.chainAID)
	iftA, ok := manifestA.token("FOO")
	require.True(t, ok)
	iftB, ok := d.manifest(t, d.chainBID).token("FOO")
	require.True(t, ok)
	bridge := []string{
		"ift-bridge",
		"--chain-a", d.chainAID, "--ift-a", iftA,
		"--chain-b", d.chainBID, "--ift-b", iftB,
		"--yes",
	}
	d.deploy(t, [][]string{bridge}, "executed")

	// the heads have moved on, but a rerun compares only the recorded params
	d.deploy(t, slices.Concat(core, bringUp, [][]string{bridge}), "skipped")

	_, err := d.driver.Deploy(ctx, "status")
	require.NoError(t, err)

	// the relayer signs with its own key so its nonces never race the
	// deployer's sends
	d.importFundedKey(t, relayerAlias, "--populate-config")
	rendered, err := d.driver.Deploy(ctx, "render-config", d.chainAID, d.chainBID,
		"--signer-a", relayerAlias, "--signer-b", relayerAlias, "--populate-config", "--yes")
	require.NoError(t, err)
	require.Equal(t, 2, strings.Count(string(rendered), "type: "+clientType))

	route := e2etest.AtoB(e2etest.ChainA, e2etest.ChainB)
	require.NoError(t, d.driver.ConfigureRelayer(ibccli.RelayerOptions{
		ChainIDs: map[string]string{
			string(e2etest.ChainA): d.chainAID,
			string(e2etest.ChainB): d.chainBID,
		},
		WaitPolicies: map[string]ibccli.WaitPolicy{
			string(route.ID): e2etest.RouteWaitPolicy(d.chainA.Timing(), d.chainB.Timing()),
		},
	}))
	relayer := e2etest.StartRelayer(t, d.driver, d.env)

	const (
		minted    = "100000000000000000000"
		sent      = "10000000000000000000"
		remaining = "90000000000000000000"
	)
	_, err = d.driver.Run(ctx, "tx", "ift", "mint", "--chain", d.chainAID, "--ift", iftA,
		"--to", deployerAlias, "--from", deployerAlias, "--amount", minted)
	require.NoError(t, err)

	recipient, err := e2etest.NewAddress()
	require.NoError(t, err)
	stdout, err := d.driver.Run(ctx, "tx", "ift", "send", "--chain", d.chainAID, "--ift", iftA,
		"--client-id", d.clientID(), "--to", recipient.Hex(), "--from", deployerAlias, "--amount", sent)
	require.NoError(t, err)
	var send struct {
		TxHash string `json:"txHash"`
	}
	require.NoErrorf(t, json.Unmarshal(stdout, &send), "tx ift send stdout: %s", stdout)

	block, sequence := sentPacket(ctx, t, d.chainA.RPCURL(), manifestA.Core.Router, send.TxHash)
	packet := e2etest.PacketTx{
		RouteID:           route.ID,
		Source:            route.Source,
		SourceClientID:    d.clientID(),
		SourceTxHash:      send.TxHash,
		SourceBlockNumber: block,
		Sequence:          sequence,
	}
	_, err = e2etest.AwaitState(ctx, relayer, packet, relayerv2.PacketState_PACKET_STATE_SUCCEEDED)
	require.NoError(t, err)

	require.Equal(t, remaining, d.iftBalance(t, d.chainAID, iftA, deployerAlias))
	require.Equal(t, sent, d.iftBalance(t, d.chainBID, iftB, recipient.Hex()))
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
			{
				"ift", "--chain", chainID, "--name", "Foo", "--symbol", "FOO",
				"--rate-limit-capacity", "1000000000000000000000", "--yes",
			},
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

// sentPacket returns the block and sequence of the one packet the router
// sent in txHash.
func sentPacket(ctx context.Context, t testing.TB, rpcURL, router, txHash string) (uint64, uint64) {
	t.Helper()
	client, err := ethclient.DialContext(ctx, rpcURL)
	require.NoError(t, err)
	defer client.Close()
	receipt, err := client.TransactionReceipt(ctx, common.HexToHash(txHash))
	require.NoError(t, err)
	sequences, err := e2etest.SendPacketSequences(common.HexToAddress(router), receipt)
	require.NoError(t, err)
	require.Lenf(t, sequences, 1, "SendPacket events in %s", txHash)
	return receipt.BlockNumber.Uint64(), sequences[0]
}

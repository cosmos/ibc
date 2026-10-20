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
	"testing"

	"github.com/cosmos/solidity-ibc-eureka/packages/go-abigen/ics26router"
	ethereum "github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/stretchr/testify/require"

	relayerv2 "github.com/cosmos/ibc/cli/api/v2/relayer"
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
	attestorAlias = "attestor"
)

// e18 scales whole tokens to base units, so the minted amount and the
// sender's remaining balance exceed 64 bits on their way through the CLI.
const e18 = "000000000000000000"

var (
	routeAtoB = e2etest.AtoB(e2etest.ChainA, e2etest.ChainB)
	routeBtoA = e2etest.BtoA(e2etest.ChainB, e2etest.ChainA)
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
	// deployed is every command deploy ran, for requireRerunSkipped
	deployed [][]string
}

func startDeployCLI(t *testing.T, requirements e2etest.EVMRequirements) *deployCLI {
	t.Helper()
	spec := environment.Spec{
		Chains: e2etest.EVMChains(t, requirements, e2etest.ChainA, e2etest.ChainB),
	}
	env := e2etest.Start(t, spec, environment.Runtime{})

	d := &deployCLI{env: env, home: t.TempDir()}
	d.chainAID, d.chainA = d.chain(t, e2etest.ChainA)
	d.chainBID, d.chainB = d.chain(t, e2etest.ChainB)

	var err error
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

// chain resolves a harness chain id to its EVM chain id and managed chain.
func (d *deployCLI) chain(t *testing.T, id environment.ChainID) (string, *environment.Chain) {
	t.Helper()
	chain, err := d.env.Chain(id)
	require.NoError(t, err)
	return strconv.FormatUint(chain.EVMChainID(), 10), chain
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

// deploy runs each command, requires every step it reports to have executed,
// and records the command for requireRerunSkipped.
func (d *deployCLI) deploy(t *testing.T, commands ...[]string) {
	t.Helper()
	for _, args := range commands {
		d.requireSteps(t, args, "executed")
	}
	d.deployed = append(d.deployed, commands...)
}

// requireRerunSkipped reruns every command deploy ran since startDeployCLI
// and requires every step to be skipped. Chain heads have moved on by then,
// but a rerun compares only the recorded params.
func (d *deployCLI) requireRerunSkipped(t *testing.T) {
	t.Helper()
	for _, args := range d.deployed {
		d.requireSteps(t, args, "skipped")
	}
}

// requireSteps runs one deploy command and requires every step it reports to
// have taken action ("executed" or "skipped").
func (d *deployCLI) requireSteps(t *testing.T, args []string, action string) {
	t.Helper()
	stdout, err := d.driver.Deploy(t.Context(), args...)
	require.NoErrorf(t, err, "deploy %v", args)
	results := decodeStepResults(t, stdout)
	require.NotEmptyf(t, results, "deploy %v", args)
	for _, r := range results {
		require.Equalf(t, action, r.Action, "step %q of %v", r.Name, args)
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

// importKey imports a fresh ECDSA key as alias with `ibc keys import ecdsa`
// plus extra flags.
func (d *deployCLI) importKey(t *testing.T, alias string, extra ...string) common.Address {
	t.Helper()
	key, err := crypto.GenerateKey()
	require.NoError(t, err)
	privateKey := hex.EncodeToString(crypto.FromECDSA(key))
	_, err = d.driver.Run(t.Context(), "keys",
		append([]string{"import", "ecdsa", alias, "--private-key", privateKey}, extra...)...)
	require.NoError(t, err)
	return crypto.PubkeyToAddress(key.PublicKey)
}

// importFundedKey imports a fresh key like importKey and funds it on both
// chains.
func (d *deployCLI) importFundedKey(t *testing.T, alias string, extra ...string) common.Address {
	t.Helper()
	address := d.importKey(t, alias, extra...)
	for _, chain := range []*environment.Chain{d.chainA, d.chainB} {
		funding, err := chain.Funding()
		require.NoError(t, err)
		require.NoError(t, funding.EnsureEOABalance(t.Context(), address, e2etest.RequiredSignerBalance()))
	}
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

// deployCore deploys core on both chains.
func (d *deployCLI) deployCore(t *testing.T) {
	t.Helper()
	d.deploy(t,
		[]string{"core", "--chain", d.chainAID, "--yes"},
		[]string{"core", "--chain", d.chainBID, "--yes"},
	)
}

// configureRouters points the config at the routers core deployed, which
// `deploy client besu-qbft` reads for its counterparty. Every other deploy
// command reads a chain's own router from its manifest. The rewrite drops
// signers imported since startDeployCLI.
func (d *deployCLI) configureRouters(t *testing.T) {
	t.Helper()
	d.writeConfig(t, d.manifest(t, d.chainAID).Core.Router, d.manifest(t, d.chainBID).Core.Router)
}

// deployIFTConnection deploys clientA and clientB, GMP and a FOO IFT on each
// chain, then bridges the two tokens. It returns the token addresses.
func (d *deployCLI) deployIFTConnection(t *testing.T, clientA, clientB []string) (string, string) {
	t.Helper()
	for _, c := range []struct {
		chainID string
		client  []string
	}{{d.chainAID, clientA}, {d.chainBID, clientB}} {
		d.deploy(t,
			c.client,
			[]string{"gmp", "--chain", c.chainID, "--yes"},
			[]string{
				"ift", "--chain", c.chainID, "--name", "Foo", "--symbol", "FOO",
				"--rate-limit-capacity", "1000" + e18, "--yes",
			},
		)
	}

	iftA, ok := d.manifest(t, d.chainAID).token("FOO")
	require.True(t, ok)
	iftB, ok := d.manifest(t, d.chainBID).token("FOO")
	require.True(t, ok)
	d.deploy(t, []string{
		"ift-bridge",
		"--chain-a", d.chainAID, "--ift-a", iftA,
		"--chain-b", d.chainBID, "--ift-b", iftB,
		"--yes",
	})
	return iftA, iftB
}

// renderConfig runs render-config for the two chains with signer relaying to
// both ends, plus extra flags, and returns what it printed.
func (d *deployCLI) renderConfig(t *testing.T, signer string, extra ...string) ibccli.RenderedConfig {
	t.Helper()
	args := []string{"render-config", d.chainAID, d.chainBID, "--signer-a", signer, "--signer-b", signer}
	stdout, err := d.driver.Deploy(t.Context(), append(args, extra...)...)
	require.NoError(t, err)
	cfg, err := ibccli.ParseRenderedConfig(stdout)
	require.NoErrorf(t, err, "render-config stdout: %s", stdout)
	return cfg
}

// populateRelayConfig renders the relayer config for the two chains into the
// CLI config and returns what render-config printed. The relayer signs with
// its own key so its nonces never race the deployer's sends.
func (d *deployCLI) populateRelayConfig(t *testing.T) ibccli.RenderedConfig {
	t.Helper()
	d.importFundedKey(t, relayerAlias, "--populate-config")
	return d.renderConfig(t, relayerAlias, "--populate-config", "--yes")
}

// wantConnection is the one connection render-config pairs the two clients
// into, auto-relayed with the signer key.
func (d *deployCLI) wantConnection(signer string, typeA, typeB clientkind.Kind) []ibccli.ConnectionFileConfig {
	end := func(chainID string, typ clientkind.Kind) ibccli.ClientEndFileConfig {
		return ibccli.ClientEndFileConfig{
			ChainID:   chainID,
			Signer:    signer,
			ClientID:  d.clientID(),
			Type:      typ,
			AutoRelay: &ibccli.AutoRelayFileConfig{Enabled: true},
		}
	}
	return []ibccli.ConnectionFileConfig{{
		Alias:   d.chainAID + "-" + d.chainBID,
		ClientA: end(d.chainAID, typeA),
		ClientB: end(d.chainBID, typeB),
	}}
}

// wantAttestor is the local attestor render-config binds to the signer alias
// holding address, watching chainID.
func wantAttestor(chainID, signer string, address common.Address) ibccli.AttestorFileConfig {
	return ibccli.AttestorFileConfig{
		Name:           "attestor-" + chainID + "-" + address.Hex(),
		ChainID:        chainID,
		Type:           ibccli.RelayerAttestorLocal,
		Signer:         signer,
		FinalityOffset: 1,
	}
}

// startRelayer starts the relayer on the populated config with wait policies
// for both routes.
func (d *deployCLI) startRelayer(t *testing.T) *ibccli.Relayer {
	t.Helper()
	waitPolicies := make(map[string]ibccli.WaitPolicy)
	for _, route := range []e2etest.Route{routeAtoB, routeBtoA} {
		_, source := d.chain(t, route.Source)
		_, destination := d.chain(t, route.Destination)
		waitPolicies[string(route.ID)] = e2etest.RouteWaitPolicy(source.Timing(), destination.Timing())
	}
	require.NoError(t, d.driver.ConfigureRelayer(ibccli.RelayerOptions{
		ChainIDs: map[string]string{
			string(e2etest.ChainA): d.chainAID,
			string(e2etest.ChainB): d.chainBID,
		},
		WaitPolicies: waitPolicies,
	}))
	return e2etest.StartRelayer(t, d.driver, d.env)
}

// sendIFT sends amount of the IFT token at ift from the deployer on the
// route's source chain to `to` with `ibc tx ift send`, and returns the packet
// it sent.
func (d *deployCLI) sendIFT(t *testing.T, route e2etest.Route, ift, to, amount string) e2etest.PacketTx {
	t.Helper()
	ctx := t.Context()
	chainID, chain := d.chain(t, route.Source)
	stdout, err := d.driver.Run(ctx, "tx", "ift", "send", "--chain", chainID, "--ift", ift,
		"--client-id", d.clientID(), "--to", to, "--from", deployerAlias, "--amount", amount)
	require.NoError(t, err)
	var send struct {
		TxHash string `json:"txHash"`
	}
	require.NoErrorf(t, json.Unmarshal(stdout, &send), "tx ift send stdout: %s", stdout)

	router := d.manifest(t, chainID).Core.Router
	block, sequence := sentPacket(ctx, t, chain.RPCURL(), router, send.TxHash)
	return e2etest.PacketTx{
		RouteID:           route.ID,
		Source:            route.Source,
		SourceClientID:    d.clientID(),
		SourceTxHash:      send.TxHash,
		SourceBlockNumber: block,
		Sequence:          sequence,
	}
}

// besuQBFTClient is the `ibc deploy client besu-qbft` command for a client on
// chainID tracking counterparty.
func besuQBFTClient(chainID, counterparty string) []string {
	return []string{
		"client", "besu-qbft",
		"--chain", chainID,
		"--counterparty-chain", counterparty,
		"--trusting-period", "336h",
		"--max-clock-drift", "1m",
		"--yes",
	}
}

// attestationClient is the `ibc deploy client attestation` command for a
// client on chainID tracking counterparty, trusting attestors (addresses or
// aliases).
func attestationClient(chainID, counterparty, attestors string) []string {
	return []string{
		"client", "attestation",
		"--chain", chainID,
		"--counterparty-chain", counterparty,
		"--attestors", attestors,
		"--yes",
	}
}

// TestDeployConnection drives `ibc deploy` as a black box over attestation
// clients, asserting against the CLI's JSON step output, the manifests it
// writes and the relayer config it renders. Each step is a separate
// idempotent command: core on each chain, a client on each chain tracking the
// other, GMP and an IFT on each chain, and one ift-bridge registering both
// sides. Both client invocations, and the bridge, derive the same shared
// client id.
func TestDeployConnection(t *testing.T) {
	t.Parallel()
	d := startDeployCLI(t, e2etest.EVMRequirements{})
	ctx := t.Context()

	// the config keeps its placeholder routers: no command here may read a
	// router from config
	d.deployCore(t)
	d.deployIFTConnection(t,
		attestationClient(d.chainAID, d.chainBID, d.deployer.Hex()),
		attestationClient(d.chainBID, d.chainAID, d.deployer.Hex()))

	// core provisioning binds the relaying selectors to PUBLIC_ROLE;
	// prove an unrelated address can call recvPacket on each router. The
	// driver's ChainRPC values are env-var templates only the CLI process
	// expands, so dial the chains' real RPC URLs.
	manifestA := d.manifest(t, d.chainAID)
	assertPublicRelaying(ctx, t, d.chainA.RPCURL(), manifestA)
	assertPublicRelaying(ctx, t, d.chainB.RPCURL(), d.manifest(t, d.chainBID))
	require.NotNil(t, manifestA.GMP)
	require.Equal(t, "gmpport", manifestA.GMP.Port)

	d.requireRerunSkipped(t)

	_, err := d.driver.Deploy(ctx, "status")
	require.NoError(t, err)

	// render-config pairs the two manifests into one relayer.connections[]
	// entry, and each attestation end gets the deployer as a local attestor
	// watching the chain it tracks
	cfg := d.renderConfig(t, deployerAlias)
	require.Equal(t, d.wantConnection(deployerAlias, clientkind.Attestation, clientkind.Attestation), cfg.Connections)
	require.ElementsMatch(t, []ibccli.AttestorFileConfig{
		wantAttestor(d.chainAID, deployerAlias, d.deployer),
		wantAttestor(d.chainBID, deployerAlias, d.deployer),
	}, cfg.Attestors)
}

// TestDeployBesuQBFTConnection relays an IFT transfer between two Besu chains
// set up through the CLI alone: deploy a besu-qbft connection and an IFT
// bridge, render the relayer config from the manifests, then run the relayer.
// Each client trusts the other chain's head and needs its router, so the
// config carries the routers core deployed (configureRouters).
func TestDeployBesuQBFTConnection(t *testing.T) {
	t.Parallel()
	d := startDeployCLI(t, e2etest.EVMRequirements{Provider: e2etest.EVMProviderBesu})
	ctx := t.Context()

	d.deployCore(t)
	d.configureRouters(t)
	iftA, iftB := d.deployIFTConnection(t,
		besuQBFTClient(d.chainAID, d.chainBID), besuQBFTClient(d.chainBID, d.chainAID))
	d.requireRerunSkipped(t)

	_, err := d.driver.Deploy(ctx, "status")
	require.NoError(t, err)

	cfg := d.populateRelayConfig(t)
	require.Equal(t, d.wantConnection(relayerAlias, clientkind.BesuQBFT, clientkind.BesuQBFT), cfg.Connections)
	require.Empty(t, cfg.Attestors)
	relayer := d.startRelayer(t)

	_, err = d.driver.Run(ctx, "tx", "ift", "mint", "--chain", d.chainAID, "--ift", iftA,
		"--to", deployerAlias, "--from", deployerAlias, "--amount", "100"+e18)
	require.NoError(t, err)

	recipient, err := e2etest.NewAddress()
	require.NoError(t, err)
	packet := d.sendIFT(t, routeAtoB, iftA, recipient.Hex(), "10"+e18)
	_, err = e2etest.AwaitState(ctx, relayer, packet, relayerv2.PacketState_PACKET_STATE_SUCCEEDED)
	require.NoError(t, err)

	require.Equal(t, "90"+e18, d.iftBalance(t, d.chainAID, iftA, deployerAlias))
	require.Equal(t, "10"+e18, d.iftBalance(t, d.chainBID, iftB, recipient.Hex()))
}

// TestDeployMixedBesuQBFTAttestationConnection relays an IFT round trip over
// the production shape of a Besu connection, set up through the CLI alone:
// chain A tracks Besu chain B with a besu-qbft client, and B tracks A with an
// attestation client. render-config binds the attestation end's attestor to
// its imported key as a local attestor the relayer hosts, so each direction
// proves its receive with one client type and its ack with the other.
func TestDeployMixedBesuQBFTAttestationConnection(t *testing.T) {
	t.Parallel()
	d := startDeployCLI(t, e2etest.EVMRequirements{Provider: e2etest.EVMProviderBesu})
	ctx := t.Context()

	d.deployCore(t)
	d.configureRouters(t)

	// the attestor only signs, so it needs no funds. Importing it into the
	// config lets --attestors resolve the alias and render-config bind it as
	// the attestor's signer; it goes after configureRouters, which drops other
	// signers.
	attestor := d.importKey(t, attestorAlias, "--populate-config")
	iftA, iftB := d.deployIFTConnection(t,
		besuQBFTClient(d.chainAID, d.chainBID), attestationClient(d.chainBID, d.chainAID, attestorAlias))
	d.requireRerunSkipped(t)

	_, err := d.driver.Deploy(ctx, "status")
	require.NoError(t, err)

	// only the attestation end gets an attestor, watching the chain it tracks
	cfg := d.populateRelayConfig(t)
	require.Equal(t, d.wantConnection(relayerAlias, clientkind.BesuQBFT, clientkind.Attestation), cfg.Connections)
	require.Equal(t, []ibccli.AttestorFileConfig{wantAttestor(d.chainAID, attestorAlias, attestor)}, cfg.Attestors)
	relayer := d.startRelayer(t)

	_, err = d.driver.Run(ctx, "tx", "ift", "mint", "--chain", d.chainAID, "--ift", iftA,
		"--to", deployerAlias, "--from", deployerAlias, "--amount", "100"+e18)
	require.NoError(t, err)

	// A -> B: the attestor proves the receive on B, the besu-qbft client the
	// ack on A
	packet := d.sendIFT(t, routeAtoB, iftA, d.deployer.Hex(), "10"+e18)
	_, err = e2etest.AwaitState(ctx, relayer, packet, relayerv2.PacketState_PACKET_STATE_SUCCEEDED)
	require.NoError(t, err)
	require.Equal(t, "90"+e18, d.iftBalance(t, d.chainAID, iftA, deployerAlias))
	require.Equal(t, "10"+e18, d.iftBalance(t, d.chainBID, iftB, deployerAlias))

	// B -> A: the besu-qbft client proves the receive on A, the attestor the
	// ack on B
	recipient, err := e2etest.NewAddress()
	require.NoError(t, err)
	packet = d.sendIFT(t, routeBtoA, iftB, recipient.Hex(), "4"+e18)
	_, err = e2etest.AwaitState(ctx, relayer, packet, relayerv2.PacketState_PACKET_STATE_SUCCEEDED)
	require.NoError(t, err)
	require.Equal(t, "6"+e18, d.iftBalance(t, d.chainBID, iftB, deployerAlias))
	require.Equal(t, "4"+e18, d.iftBalance(t, d.chainAID, iftA, recipient.Hex()))
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

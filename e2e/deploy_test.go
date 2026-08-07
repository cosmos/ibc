package e2e_test

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/cosmos/solidity-ibc-eureka/packages/go-abigen/ics26router"
	ethereum "github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/stretchr/testify/require"

	"github.com/cosmos/ibc/e2e/internal/e2etest"
	"github.com/cosmos/ibc/e2e/internal/harness/environment"
	"github.com/cosmos/ibc/e2e/internal/harness/ibclink"
)

// deployStepResult mirrors link/internal/deploy.StepResult, which e2e cannot
// import directly (module boundary): "name" and "action" ("skipped",
// "executed", or "planned").
type deployStepResult struct {
	Name   string `json:"name"`
	Action string `json:"action"`
}

// deployManifest is the subset of link/internal/deploy/manifest.Manifest
// this test needs to observe from the written JSON file.
type deployManifest struct {
	Core struct {
		Router string `json:"router"`
	} `json:"core"`
	TargetData map[string]string `json:"targetData"`
}

// TestDeployConnection drives `ibc deploy` as a black box: two bare managed
// chains (no protocol resources — the deploy CLI provisions IBC itself),
// a temporary CLI home with an imported deployer key, and assertions against
// the CLI's JSON step output and the manifests it writes.
func TestDeployConnection(t *testing.T) {
	t.Parallel()

	spec := environment.Spec{
		Chains: e2etest.EVMChains(t, e2etest.EVMRequirements{}, e2etest.ChainA, e2etest.ChainB),
	}
	env := e2etest.Start(t, spec, environment.Runtime{})

	chainA, err := env.Chain(e2etest.ChainA)
	require.NoError(t, err)
	chainB, err := env.Chain(e2etest.ChainB)
	require.NoError(t, err)
	chainAID := strconv.FormatUint(chainA.EVMChainID(), 10)
	chainBID := strconv.FormatUint(chainB.EVMChainID(), 10)

	home := t.TempDir()
	configPath := filepath.Join(home, "ibc.yml")
	driver, err := ibclink.NewDriver(configPath)
	require.NoError(t, err)
	require.NoError(t, env.BindIBCLink(driver))

	deployerKey, err := crypto.GenerateKey()
	require.NoError(t, err)
	deployerHex := hex.EncodeToString(crypto.FromECDSA(deployerKey))
	deployerAddress := crypto.PubkeyToAddress(deployerKey.PublicKey)

	minimum := e2etest.RequiredSignerBalance()
	for _, chain := range []*environment.Chain{chainA, chainB} {
		funding, fundingErr := chain.Funding()
		require.NoError(t, fundingErr)
		require.NoError(t, funding.EnsureEOABalance(t.Context(), deployerAddress, minimum))
	}

	rpcA, err := driver.ChainRPC(string(e2etest.ChainA))
	require.NoError(t, err)
	rpcB, err := driver.ChainRPC(string(e2etest.ChainB))
	require.NoError(t, err)

	const deployerAlias = "deployer"
	err = ibclink.WriteDeployConfig(configPath, ibclink.DeployConfig{
		DBPath:        filepath.Join(home, "unused.db"),
		SignerAlias:   deployerAlias,
		SignerKeyFile: driver.KeyFilePath(deployerAlias),
		Chains: []ibclink.DeployChain{
			{ChainID: chainAID, RPC: rpcA},
			{ChainID: chainBID, RPC: rpcB},
		},
	})
	require.NoError(t, err)

	ctx := t.Context()
	require.NoError(t, driver.KeysImportECDSA(ctx, deployerAlias, deployerHex))

	// the connection is four separate idempotent commands: core on each
	// chain, then a client on each chain tracking the other. Both client
	// invocations derive the same shared client id.
	sharedClientID := "link-" + chainAID + "-" + chainBID
	deployCommands := [][]string{
		{"core", "--chain", chainAID, "--yes"},
		{"core", "--chain", chainBID, "--yes"},
		{
			"client",
			"--chain",
			chainAID,
			"--counterparty-chain",
			chainBID,
			"--attestors",
			deployerAddress.Hex(),
			"--yes",
		},
		{
			"client",
			"--chain",
			chainBID,
			"--counterparty-chain",
			chainAID,
			"--attestors",
			deployerAddress.Hex(),
			"--yes",
		},
	}

	for _, args := range deployCommands {
		stdout, deployErr := driver.Deploy(ctx, args...)
		require.NoErrorf(t, deployErr, "deploy %v", args)
		results := decodeStepResults(t, stdout)
		require.NotEmpty(t, results)
		for _, r := range results {
			require.Equalf(t, "executed", r.Action, "step %q", r.Name)
		}
	}

	manifestDir := filepath.Join(home, "deployments")
	manifestA := readManifest(t, manifestDir, chainAID)
	manifestB := readManifest(t, manifestDir, chainBID)
	require.NotEmpty(t, manifestA.Core.Router)
	require.NotEmpty(t, manifestB.Core.Router)

	// core provisioning binds the relaying selectors to PUBLIC_ROLE;
	// prove an unrelated address can call recvPacket on each router. The
	// driver's ChainRPC values are env-var templates only the CLI process
	// expands, so dial the chains' real RPC URLs.
	assertPublicRelaying(ctx, t, chainA.RPCURL(), manifestA)
	assertPublicRelaying(ctx, t, chainB.RPCURL(), manifestB)

	// Idempotency: rerunning every identical command skips its step.
	for _, args := range deployCommands {
		stdout, deployErr := driver.Deploy(ctx, args...)
		require.NoErrorf(t, deployErr, "rerun %v", args)
		rerun := decodeStepResults(t, stdout)
		require.NotEmpty(t, rerun)
		for _, r := range rerun {
			require.Equalf(t, "skipped", r.Action, "step %q", r.Name)
		}
	}

	_, err = driver.Deploy(ctx, "status")
	require.NoError(t, err)

	// render-config pairs the two manifests into relayer config sections
	rendered, err := driver.Deploy(ctx, "render-config", chainAID, chainBID)
	require.NoError(t, err)
	for _, want := range []string{
		"clientId: " + sharedClientID,
		"counterpartyClientId: " + sharedClientID,
	} {
		require.Contains(t, string(rendered), want)
	}
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

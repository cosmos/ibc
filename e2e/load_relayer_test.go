package e2e_test

import (
	"context"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	catalystevm "github.com/skip-mev/catalyst/chains/ethereum/types"
	catalyst "github.com/skip-mev/catalyst/chains/types"
	"github.com/stretchr/testify/require"

	"github.com/cosmos/ibc/e2e/internal/e2etest"
	"github.com/cosmos/ibc/e2e/internal/harness/environment"
	"github.com/cosmos/ibc/e2e/internal/harness/ibccli"
	"github.com/cosmos/ibc/e2e/internal/loadtest"
)

// E2E_LOAD=true, E2E_LOAD=1, or -e2e.load required
// Note these tests are NOT parallel on purpose.

const (
	baseMnemonic       = "rotate stumble once topic possible message powder recall turkey legend depart brick"
	packetBatchSize    = 64
	packetBatchTimeout = 1 * time.Second

	// the gas balance and IFT amount given to each load-test wallet.
	walletFundingCoins = 10
)

type relayerLoadIFT struct {
	totalPackets    int
	numWallets      int
	packetsPerBlock int
}

type loadTestRelayer struct {
	t          *testing.T
	env        *environment.Environment
	deployment *e2etest.Deployment
	sender     e2etest.Signer
	relayer    *ibccli.Relayer
}

func TestLoad_RelayerBurst(t *testing.T) {
	testLoadRelayer(t, relayerLoadIFT{
		totalPackets:    1000,
		numWallets:      200,
		packetsPerBlock: 100,
	})
}

func TestLoad_RelayerLongRun(t *testing.T) {
	testLoadRelayer(t, relayerLoadIFT{
		totalPackets:    8000,
		numWallets:      200,
		packetsPerBlock: 20,
	})
}

func testLoadRelayer(t *testing.T, load relayerLoadIFT) {
	t.Helper()
	// ARRANGE
	numWallets := load.numWallets
	totalPackets := load.totalPackets
	packetsPerBlock := load.packetsPerBlock

	// Given test suite
	ts := newLoadTestRelayer(t)

	ctx := t.Context()

	// Given IFT load spec for A->B
	routeAB := e2etest.AtoB(e2etest.ChainA, e2etest.ChainB)
	evmEndpointA, tokenA := ts.catalystSource(routeAB)
	loadSpecAB, err := loadtest.NewSpecIFT(
		numWallets, totalPackets/2, packetsPerBlock,
		baseMnemonic,
		evmEndpointA,
		tokenA,
	)
	require.NoError(t, err)
	require.NotNil(t, loadSpecAB)

	// Given IFT load spec for B->A
	routeBA := e2etest.BtoA(e2etest.ChainB, e2etest.ChainA)
	evmEndpointB, tokenB := ts.catalystSource(routeBA)
	loadSpecBA, err := loadtest.NewSpecIFT(
		numWallets, totalPackets/2, packetsPerBlock,
		baseMnemonic,
		evmEndpointB,
		tokenB,
	)
	require.NoError(t, err)
	require.NotNil(t, loadSpecBA)

	// Given N wallets
	wallets, err := loadSpecAB.Wallets()
	require.NoError(t, err)
	require.Len(t, wallets, numWallets)

	// Fund wallets with gas on both chains
	// Fund wallets with IFT on both chains
	var wg sync.WaitGroup

	wg.Go(func() { ts.fundGas(e2etest.ChainA, wallets) })
	wg.Go(func() { ts.fundGas(e2etest.ChainB, wallets) })
	wg.Go(func() { ts.fundIFT(e2etest.ChainA, wallets) })
	wg.Go(func() { ts.fundIFT(e2etest.ChainB, wallets) })

	wg.Wait()

	// ACT
	// Run two catalysts in parallel
	timeout := loadtest.TestTimeout(t)
	t.Logf("Running Catalyst load test with timeout %s", timeout.String())
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	var (
		resultAB, resultBA catalyst.LoadTestResult
		txsAB, txsBA       []*catalystevm.SentTx
		errAB, errBA       error
	)

	wg.Go(func() { resultAB, txsAB, errAB = loadSpecAB.Run(ctx, t) })
	wg.Go(func() { resultBA, txsBA, errBA = loadSpecBA.Run(ctx, t) })

	wg.Wait()

	// ASSERT
	require.NoError(t, errAB, "Catalyst A->B")
	require.NoError(t, errBA, "Catalyst B->A")
	require.Equal(t, totalPackets/2, resultAB.Overall.TotalTransactions, "Catalyst A->B")
	require.Equal(t, totalPackets/2, resultBA.Overall.TotalTransactions, "Catalyst B->A")

	// ASSERT #2
	var errAwaitAB, errAwaitBA error

	wg.Go(func() {
		errAwaitAB = loadtest.AwaitPacketsFromCatalyst(ctx, t, routeAB, ts.relayer, txsAB)
	})
	wg.Go(func() {
		errAwaitBA = loadtest.AwaitPacketsFromCatalyst(ctx, t, routeBA, ts.relayer, txsBA)
	})

	wg.Wait()

	require.NoError(t, errAwaitAB, "Await A->B")
	require.NoError(t, errAwaitBA, "Await B->A")

	// view Grafana dashboard to track latencies
	// todo: FOU-1740: expose timestamps for tx lifecycle
}

func newLoadTestRelayer(t *testing.T) *loadTestRelayer {
	// ARRANGE
	// Given attested setup
	spec, runtime := attestedMesh(e2etest.EVMChains(t, e2etest.EVMRequirements{}, e2etest.ChainA, e2etest.ChainB))
	env := e2etest.Start(t, spec, runtime)

	// Given signers
	sender := e2etest.NewSigner(t)
	relayerSigner := e2etest.NewSigner(t)

	// Given relayer deployment with clearing enabled and two routes
	routeAB := e2etest.AtoB(e2etest.ChainA, e2etest.ChainB)
	routeBA := e2etest.BtoA(e2etest.ChainB, e2etest.ChainA)

	withConfig := func(cfg *ibccli.RelayerConfig) {
		cfg.ClearInterval = 1 * time.Second
		cfg.ClearOnStart = true

		for i := range cfg.Chains {
			cfg.Chains[i].PacketBatchSize = packetBatchSize
			cfg.Chains[i].PacketBatchTimeout = packetBatchTimeout
		}
	}

	driver, deployment := e2etest.DeployWithRelayerConfig(t, env, sender, relayerSigner, withConfig, routeAB, routeBA)

	// Given a relayer
	relayer := e2etest.StartRelayer(t, driver, env)

	return &loadTestRelayer{
		t:          t,
		env:        env,
		deployment: deployment,
		sender:     sender,
		relayer:    relayer,
	}
}

func (ts *loadTestRelayer) catalystSource(route e2etest.Route) (loadtest.EVMEndpoint, loadtest.IFTToken) {
	ts.t.Helper()

	chain, err := ts.env.Chain(route.Source)
	require.NoError(ts.t, err)

	apps, ok := ts.deployment.Chain(route.Source)
	require.True(ts.t, ok, "e2etest: chain %q was not deployed", route.Source)

	clients, ok := ts.deployment.RouteClients(route.ID)
	require.True(ts.t, ok, "e2etest: route %q has no clients", route.ID)

	evmEndpoint := loadtest.EVMEndpoint{
		ChainID:   strconv.FormatUint(chain.EVMChainID(), 10),
		RPC:       chain.RPCURL(),
		Websocket: chain.WSURL(),
	}

	token := loadtest.IFTToken{
		ClientID: clients.SourceClientID,
		Address:  apps.IFT.Hex(),
	}

	return evmEndpoint, token
}

func (ts *loadTestRelayer) fundGas(chainID environment.ChainID, wallets []common.Address) {
	chain, err := ts.env.Chain(chainID)
	require.NoError(ts.t, err)

	funding, err := chain.Funding()
	require.NoError(ts.t, err)

	ctx := ts.t.Context()
	desiredBalance := e2etest.Coins(walletFundingCoins)

	ts.t.Logf("Funding %d wallets on chain %s", len(wallets), chainID)
	start := time.Now()

	for _, wallet := range wallets {
		err := funding.EnsureEOABalance(ctx, wallet, desiredBalance)
		require.NoError(ts.t, err)
	}

	elapsed := time.Since(start)
	ts.t.Logf("Funded %d wallets on chain %s in %s", len(wallets), chainID, elapsed.String())
}

func (ts *loadTestRelayer) fundIFT(chainID environment.ChainID, wallets []common.Address) {
	var route e2etest.Route
	switch chainID {
	case e2etest.ChainA:
		route = e2etest.AtoB(e2etest.ChainA, e2etest.ChainB)
	case e2etest.ChainB:
		route = e2etest.BtoA(e2etest.ChainB, e2etest.ChainA)
	default:
		ts.t.Fatalf("fund IFT: unsupported chain %q", chainID)
	}

	app := e2etest.NewIFT(ts.t, ts.env, ts.deployment, ts.sender, route)

	amount := e2etest.Coins(walletFundingCoins)
	requests := make([]e2etest.ERCTransferRequest, len(wallets))
	for i, wallet := range wallets {
		requests[i] = e2etest.ERCTransferRequest{Address: wallet, Amount: amount}
	}

	ts.t.Logf("Sending %s IFT to %d wallets on chain %s", amount, len(wallets), chainID)

	start := time.Now()
	receipts, err := app.TransferBatch(ts.t.Context(), requests)
	require.NoError(ts.t, err)

	ts.t.Logf(
		"Sent IFT to %d wallets on chain %s in %s (%d txs)",
		len(wallets),
		chainID,
		time.Since(start).String(),
		len(receipts),
	)
}

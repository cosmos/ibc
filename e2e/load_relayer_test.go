package e2e_test

import (
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/stretchr/testify/require"

	"github.com/cosmos/ibc/e2e/internal/e2etest"
	"github.com/cosmos/ibc/e2e/internal/harness/environment"
	"github.com/cosmos/ibc/e2e/internal/harness/ibccli"
	"github.com/cosmos/ibc/e2e/internal/loadtest"
)

const baseMnemonic = "rotate stumble once topic possible message powder recall turkey legend depart brick"

func TestLoad_RelayerBurst(t *testing.T) {
	// ARRANGE
	// const (
	// 	totalPackets    = 1_000
	// 	numWallets      = 200
	// 	packetsPerBlock = 50
	// )
	const (
		totalPackets    = 200
		numWallets      = 200
		packetsPerBlock = 5
	)

	// Given test suite
	ts := newLoadTestRelayer(t)

	// Given IFT load spec for A->B
	evmEndpointA, tokenA := ts.catalystSource(e2etest.AtoB(e2etest.ChainA, e2etest.ChainB))
	loadSpecAB, err := loadtest.NewSpecIFT(
		numWallets, totalPackets/2, packetsPerBlock,
		baseMnemonic,
		evmEndpointA,
		tokenA,
	)
	require.NoError(t, err)
	require.NotNil(t, loadSpecAB)

	// Given IFT load spec for B->A
	evmEndpointB, tokenB := ts.catalystSource(e2etest.BtoA(e2etest.ChainB, e2etest.ChainA))
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

	// Fund wallets with gas on chain A and chain B
	wg := sync.WaitGroup{}
	wg.Add(2)
	go func() {
		ts.fundGas(e2etest.ChainA, wallets)
		wg.Done()
	}()
	go func() {
		ts.fundGas(e2etest.ChainB, wallets)
		wg.Done()
	}()
	wg.Wait()

	// todo for each wallet:
	// todo  - fund wallet with IFT on chain A
	// todo  - fund wallet with IFT on chain B
	// todo run catalyst
	// todo -- update protos
	// todo -- update abi
	// todo -- update allow non-zero gas
	// todo -- expose txSent
	// todo -- wait for both catalysts to return
	// todo -- wait for all packets to be FINALIZED
	// todo -- collect simple stats from catalyst
	// todo -- collect SQL stats from the relayer
}

func TestLoad_RelayerLongRun(t *testing.T) {
	t.Log("wip")
}

type loadTestRelayer struct {
	t          *testing.T
	env        *environment.Environment
	deployment *e2etest.Deployment
	relayer    *ibccli.Relayer
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

	driver, deployment := e2etest.Deploy(t, env, sender, relayerSigner, routeAB, routeBA)

	// Given a relayer
	relayer := e2etest.StartRelayer(t, driver, env)

	return &loadTestRelayer{
		t:          t,
		env:        env,
		deployment: deployment,
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
	desiredBalance := e2etest.GasCoins(10)

	ts.t.Logf("funding %d wallets on chain %s", len(wallets), chainID)
	start := time.Now()

	for _, wallet := range wallets {
		err := funding.EnsureEOABalance(ctx, wallet, desiredBalance)
		require.NoError(ts.t, err)
	}

	elapsed := time.Since(start)
	ts.t.Logf("funded %d wallets on chain %s in %s", len(wallets), chainID, elapsed.String())
}

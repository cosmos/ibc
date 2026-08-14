// SPDX-License-Identifier: Apache-2.0

package e2etest

import (
	"context"
	"fmt"
	"math/big"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/cosmos/solidity-ibc-eureka/packages/go-abigen/erc1967proxy"
	"github.com/cosmos/solidity-ibc-eureka/packages/go-abigen/ift"
	"github.com/ethereum/go-ethereum/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cosmos/ibc/e2e/internal/harness/chain/evm"
	"github.com/cosmos/ibc/e2e/internal/harness/environment"
	"github.com/cosmos/ibc/e2e/internal/harness/environment/solidityibc/counter"
	"github.com/cosmos/ibc/e2e/internal/harness/environment/solidityibc/iftbatchtransfershim"
	"github.com/cosmos/ibc/e2e/internal/harness/environment/solidityibc/iftsendcallconstructor"
	"github.com/cosmos/ibc/e2e/internal/harness/environment/solidityibc/testerc20"
	"github.com/cosmos/ibc/e2e/internal/harness/ibclink"
	linkconfig "github.com/cosmos/ibc/link/config"
)

const (
	relayerStopTimeout = 15 * time.Second
)

const weiPerEther int64 = 1_000_000_000_000_000_000

var initialTokenSupply = mustBigInt("1000000000000000000000000")

// RequiredSignerBalance is the minimum balance external Chains must provision
// for each test actor before deployment.
func RequiredSignerBalance() *big.Int {
	return new(big.Int).Mul(big.NewInt(1_000), big.NewInt(weiPerEther))
}

// ProtocolAuthorityAddress is the deployer funded for IBC Instance realization.
// Attached Chains must provision it out of band before Start.
func ProtocolAuthorityAddress() common.Address {
	account, err := evm.AccountFromHex(protocolAuthorityKeyHex)
	if err != nil {
		panic(fmt.Sprintf("e2etest: protocol authority: %v", err))
	}
	return account.Address()
}

type Route struct {
	ID          RouteID
	Source      environment.ChainID
	Destination environment.ChainID
	Manual      bool
	// SkipDestinationIFTBridge leaves the destination end's IFT bridge unregistered.
	SkipDestinationIFTBridge bool
}

const routeAtoB RouteID = "route-a-to-b"

func AtoB(a, b environment.ChainID) Route {
	return Route{ID: routeAtoB, Source: a, Destination: b}
}

func ManualAtoB(a, b environment.ChainID) Route {
	return Route{ID: routeAtoB, Source: a, Destination: b, Manual: true}
}

// ChainDeployment holds per-chain application contracts deployed for traffic.
type ChainDeployment struct {
	Token                  common.Address
	Counter                common.Address
	IFT                    common.Address
	IFTBatchShim           common.Address
	IFTSendCallConstructor common.Address
	ICS20Transfer          common.Address
	ICS27GMP               common.Address
	ICS26Router            common.Address
}

// RouteClients holds the protocol client IDs for one directed route.
type RouteClients struct {
	SourceClientID string
	DestClientID   string
}

// Deployment is the e2e traffic-layer view of protocol apps and test tokens.
type Deployment struct {
	chains map[environment.ChainID]ChainDeployment
	routes map[RouteID]RouteClients
}

func (d *Deployment) Chain(id environment.ChainID) (ChainDeployment, bool) {
	if d == nil {
		return ChainDeployment{}, false
	}
	apps, ok := d.chains[id]
	return apps, ok
}

func (d *Deployment) RouteClients(id RouteID) (RouteClients, bool) {
	if d == nil {
		return RouteClients{}, false
	}
	clients, ok := d.routes[id]
	return clients, ok
}

// Deploy writes a temporary black-box configuration, runs its migration, and
// deploys TestERC20 + Counter on each Chain. It does not start the relayer.
func Deploy(
	t testing.TB,
	env *environment.Environment,
	deployer Signer,
	relayer Signer,
	routes ...Route,
) (*ibclink.Driver, *Deployment) {
	t.Helper()
	return DeployWithRelayerConfig(t, env, deployer, relayer, nil, routes...)
}

// DeployWithRelayerConfig is Deploy with a hook that adjusts the resolved
// relayer configuration after environment Attestors are assigned.
func DeployWithRelayerConfig(
	t testing.TB,
	env *environment.Environment,
	deployer Signer,
	relayer Signer,
	configure func(*ibclink.RelayerConfig),
	routes ...Route,
) (*ibclink.Driver, *Deployment) {
	t.Helper()
	require.NotNil(t, env, "e2etest: Environment is required")
	require.NotNil(t, deployer.key, "e2etest: deployer signer is required")
	require.NotNil(t, relayer.key, "e2etest: relayer signer is required")
	ensureSignerBalances(t, env, deployer, relayer)

	dir := t.TempDir()
	signerKeyPath := filepath.Join(dir, "keys", relayerSignerAlias+".json")
	configPath := filepath.Join(dir, "ibc-link.config.yaml")
	driver, err := ibclink.NewDriver(configPath)
	require.NoError(t, err, "e2etest: create driver")
	require.NoError(t, env.BindIBCLink(driver), "e2etest: bind IBC Link process")

	deployment := deployApps(t, env, deployer, routes)
	config, options := buildConfig(t, env, driver, routes, deployment, signerKeyPath, filepath.Join(dir, "relayer.db"))
	if configure != nil {
		configure(&config)
	}
	if config.SignerType == "" || config.SignerType == linkconfig.SignerLocal {
		require.NoError(t, relayer.storeKey(config.SignerKeyFile), "e2etest: store relayer signer key")
	}
	require.NoError(t, ibclink.WriteRelayerConfig(configPath, config), "e2etest: write config")
	if err := driver.ConfigureRelayer(options); err != nil {
		t.Fatalf("e2etest: configure relayer: %v", err)
	}

	require.NoError(t, driver.MigrateUp(t.Context()), "e2etest: migrate database")
	return driver, deployment
}

// StartRelayer starts the test relayer and registers idempotent teardown.
func StartRelayer(
	t testing.TB,
	driver *ibclink.Driver,
	env *environment.Environment,
) *ibclink.Relayer {
	t.Helper()
	require.NotNil(t, driver, "e2etest: driver is required")
	require.NotNil(t, env, "e2etest: Environment is required")

	relayer, err := driver.StartRelayer(t.Context())
	require.NoError(t, err, "e2etest: start relayer")
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), relayerStopTimeout)
		defer cancel()
		assert.NoError(t, relayer.Stop(ctx), "e2etest: stop relayer")
	})

	connected := make(map[string]struct{}, len(relayer.Ready().ChainsConnected))
	for _, id := range relayer.Ready().ChainsConnected {
		connected[id] = struct{}{}
	}
	for _, id := range env.Chains() {
		chain, chainErr := env.Chain(id)
		require.NoError(t, chainErr, "e2etest: resolve Chain %q", id)
		require.Contains(
			t,
			connected,
			strconv.FormatUint(chain.EVMChainID(), 10),
			"e2etest: relayer did not connect to Chain %q",
			id,
		)
	}
	return relayer
}

func deployApps(
	t testing.TB,
	env *environment.Environment,
	deployer Signer,
	routes []Route,
) *Deployment {
	t.Helper()
	deployment := &Deployment{
		chains: make(map[environment.ChainID]ChainDeployment, len(env.Chains())),
		routes: make(map[RouteID]RouteClients, len(routes)),
	}
	for _, id := range env.Chains() {
		chain, err := env.Chain(id)
		require.NoError(t, err, "e2etest: resolve Chain %q", id)
		instance, err := env.IBCInstanceForChain(id)
		require.NoError(t, err, "e2etest: resolve IBC Instance on Chain %q", id)
		evmAccess, err := chain.EVM()
		require.NoError(t, err, "e2etest: resolve EVM access on Chain %q", id)

		ics20 := common.HexToAddress(string(instance.ICS20TransferAddress()))
		ics27 := common.HexToAddress(string(instance.ICS27GMPAddress()))
		router := common.HexToAddress(string(instance.Locator()))
		require.False(
			t,
			ics20 == (common.Address{}) || ics27 == (common.Address{}) || router == (common.Address{}),
			"e2etest: Chain %q is missing the ICS20/ICS27 app stack",
			id,
		)

		token, err := deployAndMintToken(t.Context(), evmAccess, deployer.account)
		require.NoError(t, err, "e2etest: deploy TestERC20 on Chain %q", id)
		counterAddr, err := deployContract(
			t.Context(),
			evmAccess,
			deployer.account,
			counter.CounterMetaData,
		)
		require.NoError(t, err, "e2etest: deploy Counter on Chain %q", id)
		iftToken, err := deployIFTToken(t.Context(), evmAccess, deployer.account, ics27)
		require.NoError(t, err, "e2etest: deploy IFT on Chain %q", id)
		iftSendCallConstructor, err := deployContract(
			t.Context(),
			evmAccess,
			deployer.account,
			iftsendcallconstructor.EVMIFTSendCallConstructorMetaData,
		)
		require.NoError(t, err, "e2etest: deploy IFT send-call constructor on Chain %q", id)

		iftBatchShim, err := deployIFTBatchShim(
			t.Context(),
			evmAccess,
			deployer.account,
			iftToken,
			initialTokenSupply,
		)
		require.NoError(t, err, "e2etest: deploy IFT batch transfer shim on Chain %q", id)

		deployment.chains[id] = ChainDeployment{
			Token:                  token,
			Counter:                counterAddr,
			IFT:                    iftToken,
			IFTBatchShim:           iftBatchShim,
			IFTSendCallConstructor: iftSendCallConstructor,
			ICS20Transfer:          ics20,
			ICS27GMP:               ics27,
			ICS26Router:            router,
		}
	}

	for _, route := range routes {
		sourceClient, destClient, err := resolveRouteClients(env, route)
		require.NoError(t, err, "e2etest: resolve clients for route %q", route.ID)
		deployment.routes[route.ID] = RouteClients{
			SourceClientID: sourceClient,
			DestClientID:   destClient,
		}
	}

	registerIFTBridges(t, env, deployer, deployment, routes)
	return deployment
}

// registerIFTBridges points each route end's IFT at the counterparty IFT so
// cross-chain mints authenticate.
func registerIFTBridges(
	t testing.TB,
	env *environment.Environment,
	deployer Signer,
	deployment *Deployment,
	routes []Route,
) {
	t.Helper()
	for _, route := range routes {
		clients := deployment.routes[route.ID]
		ends := []struct {
			chain         environment.ChainID
			client        string
			counterparty  environment.ChainID
			isDestination bool
		}{
			{chain: route.Source, client: clients.SourceClientID, counterparty: route.Destination},
			{chain: route.Destination, client: clients.DestClientID, counterparty: route.Source, isDestination: true},
		}
		for _, end := range ends {
			if end.isDestination && route.SkipDestinationIFTBridge {
				continue
			}
			chain, err := env.Chain(end.chain)
			require.NoError(t, err, "e2etest: resolve Chain %q", end.chain)
			evmAccess, err := chain.EVM()
			require.NoError(t, err, "e2etest: resolve EVM access on Chain %q", end.chain)
			iftToken := deployment.chains[end.chain].IFT
			counterpartyIFT := deployment.chains[end.counterparty].IFT
			registerIFTBridge(
				t, evmAccess, deployer, end.chain,
				iftToken, end.client, counterpartyIFT,
				deployment.chains[end.chain].IFTSendCallConstructor,
			)
		}
	}
}

func registerIFTBridge(
	t testing.TB,
	evmAccess *environment.EVM,
	deployer Signer,
	chain environment.ChainID,
	iftToken common.Address,
	client string,
	counterpartyIFT common.Address,
	callConstructor common.Address,
) {
	t.Helper()
	data, err := iftABI.Pack("registerIFTBridge", client, counterpartyIFT.Hex(), callConstructor)
	require.NoError(t, err, "e2etest: pack IFT registerIFTBridge")
	_, err = evmAccess.BroadcastTx(t.Context(), deployer.account, &iftToken, data, nil)
	require.NoError(t, err, "e2etest: register IFT bridge for client %q on Chain %q", client, chain)
}

func buildConfig(
	t testing.TB,
	env *environment.Environment,
	driver *ibclink.Driver,
	routes []Route,
	deployment *Deployment,
	signerKeyPath string,
	dbPath string,
) (ibclink.RelayerConfig, ibclink.RelayerOptions) {
	t.Helper()
	config := ibclink.RelayerConfig{
		DBPath:         dbPath,
		SignerAlias:    relayerSignerAlias,
		SignerKeyFile:  signerKeyPath,
		FinalityOffset: ibclink.HarnessFinalityOffset,
	}
	options := ibclink.RelayerOptions{
		ChainIDs:     make(map[string]string, len(env.Chains())),
		ManualRoutes: make(map[string]bool, len(routes)),
		WaitPolicies: make(map[string]ibclink.WaitPolicy, len(routes)),
	}
	for _, id := range env.Chains() {
		chain, err := env.Chain(id)
		require.NoError(t, err, "e2etest: resolve Chain %q", id)
		rpc, err := driver.ChainRPC(string(id))
		require.NoError(t, err, "e2etest: resolve Chain %q process binding", id)
		apps, ok := deployment.Chain(id)
		require.True(t, ok, "e2etest: deployment has no Chain %q", id)
		options.ChainIDs[string(id)] = strconv.FormatUint(chain.EVMChainID(), 10)
		config.Chains = append(config.Chains, ibclink.RelayerChain{
			ChainID:     options.ChainIDs[string(id)],
			RPC:         rpc,
			ICS26Router: apps.ICS26Router.Hex(),
		})
	}
	for _, id := range env.Attestors() {
		attestor, err := env.Attestor(id)
		if err != nil {
			t.Fatalf("e2etest: resolve Attestor %q: %v", id, err)
		}
		config.Attestors = append(config.Attestors, ibclink.RelayerAttestor{
			Name: string(attestor.ID()), Type: linkconfig.AttestorTypeRemote, GRPC: attestor.Endpoint(),
		})
	}

	connections := map[string]bool{}
	for _, route := range routes {
		clients, ok := deployment.RouteClients(route.ID)
		require.True(t, ok, "e2etest: deployment has no route %q", route.ID)
		options.ManualRoutes[string(route.ID)] = route.Manual
		source, err := env.Chain(route.Source)
		if err != nil {
			t.Fatalf("e2etest: resolve route %q source Chain %q: %v", route.ID, route.Source, err)
		}
		destination, err := env.Chain(route.Destination)
		if err != nil {
			t.Fatalf("e2etest: resolve route %q destination Chain %q: %v", route.ID, route.Destination, err)
		}
		options.WaitPolicies[string(route.ID)] = routeWaitPolicy(source.Timing(), destination.Timing())

		sourceChain := options.ChainIDs[string(route.Source)]
		destinationChain := options.ChainIDs[string(route.Destination)]
		connection := ibclink.RelayerConnection{
			ChainA:  sourceChain,
			ClientA: clients.SourceClientID,
			ChainB:  destinationChain,
			ClientB: clients.DestClientID,
		}
		if connection.ChainB+"/"+connection.ClientB < connection.ChainA+"/"+connection.ClientA {
			connection.ChainA, connection.ClientA, connection.ChainB, connection.ClientB = connection.ChainB, connection.ClientB, connection.ChainA, connection.ClientA
		}
		key := connection.ChainA + "/" + connection.ClientA
		if !connections[key] {
			connections[key] = true
			config.Connections = append(config.Connections, connection)
		}
	}
	return config, options
}

func routeWaitPolicy(source, destination environment.Timing) ibclink.WaitPolicy {
	return ibclink.WaitPolicy{
		CompletionBudget: source.CompletionBudget + destination.CompletionBudget,
		StatusPoll:       max(source.PollInterval, destination.PollInterval),
		StabilityWindow: max(
			1500*time.Millisecond,
			2*source.BlockInterval,
			2*destination.BlockInterval,
		),
	}
}

func resolveRouteClients(
	env *environment.Environment,
	route Route,
) (string, string, error) {
	for _, id := range env.Connections() {
		connection, err := env.Connection(id)
		if err != nil {
			return "", "", err
		}
		aChain := connection.A().IBCInstance().Chain().ID()
		bChain := connection.B().IBCInstance().Chain().ID()
		switch {
		case aChain == route.Source && bChain == route.Destination:
			return connection.A().ID(), connection.B().ID(), nil
		case bChain == route.Source && aChain == route.Destination:
			return connection.B().ID(), connection.A().ID(), nil
		}
	}
	return "", "", fmt.Errorf(
		"no IBC Connection links Chain %q to Chain %q",
		route.Source,
		route.Destination,
	)
}

func deployAndMintToken(
	ctx context.Context,
	client *environment.EVM,
	sender evm.Account,
) (common.Address, error) {
	token, err := deployContract(ctx, client, sender, testerc20.TestERC20MetaData, "Test Token", "TST")
	if err != nil {
		return common.Address{}, err
	}
	data, err := mustABI(testerc20.TestERC20MetaData).Pack("mint", sender.Address(), initialTokenSupply)
	if err != nil {
		return common.Address{}, fmt.Errorf("e2etest: pack TestERC20.mint: %w", err)
	}
	if _, err := client.BroadcastTx(ctx, sender, &token, data, nil); err != nil {
		return common.Address{}, fmt.Errorf("e2etest: mint TestERC20: %w", err)
	}
	return token, nil
}

// deployIFTToken deploys the IFT token (a UUPS implementation behind an
// ERC1967 proxy) and mints the initial supply to the sender.
func deployIFTToken(
	ctx context.Context,
	client *environment.EVM,
	sender evm.Account,
	ics27 common.Address,
) (common.Address, error) {
	implementation, err := deployContract(ctx, client, sender, ift.ContractMetaData)
	if err != nil {
		return common.Address{}, fmt.Errorf("e2etest: deploy IFT implementation: %w", err)
	}
	initialize, err := iftABI.Pack("initialize", sender.Address(), "IFT Token", "IFT", ics27)
	if err != nil {
		return common.Address{}, fmt.Errorf("e2etest: pack IFT initialize: %w", err)
	}
	token, err := deployContract(ctx, client, sender, erc1967proxy.ContractMetaData, implementation, initialize)
	if err != nil {
		return common.Address{}, fmt.Errorf("e2etest: deploy IFT proxy: %w", err)
	}
	mint, err := iftABI.Pack("mint", sender.Address(), initialTokenSupply)
	if err != nil {
		return common.Address{}, fmt.Errorf("e2etest: pack IFT mint: %w", err)
	}
	if _, err := client.BroadcastTx(ctx, sender, &token, mint, nil); err != nil {
		return common.Address{}, fmt.Errorf("e2etest: mint IFT supply: %w", err)
	}
	return token, nil
}

// deployIFTBatchShim deploys the IFT batch-transfer shim and funds it with
// IFT balance
func deployIFTBatchShim(
	ctx context.Context,
	client *environment.EVM,
	sender evm.Account,
	iftToken common.Address,
	amount *big.Int,
) (common.Address, error) {
	shim, err := deployContract(ctx, client, sender, iftbatchtransfershim.IFTBatchTransferShimMetaData)
	if err != nil {
		return common.Address{}, fmt.Errorf("e2etest: deploy IFT batch transfer shim: %w", err)
	}
	mint, err := iftABI.Pack("mint", shim, amount)
	if err != nil {
		return common.Address{}, fmt.Errorf("e2etest: pack IFT mint for batch shim: %w", err)
	}
	if _, err := client.BroadcastTx(ctx, sender, &iftToken, mint, nil); err != nil {
		return common.Address{}, fmt.Errorf("e2etest: mint IFT supply to batch shim: %w", err)
	}
	return shim, nil
}

func ensureSignerBalances(t testing.TB, env *environment.Environment, signers ...Signer) {
	t.Helper()
	minimum := RequiredSignerBalance()
	for _, id := range env.Chains() {
		chain, err := env.Chain(id)
		require.NoError(t, err, "e2etest: resolve Chain %q", id)
		funding, err := chain.Funding()
		if err == nil {
			for _, signer := range signers {
				require.NoError(
					t,
					funding.EnsureEOABalance(t.Context(), signer.Address(), minimum),
					"e2etest: fund signer %s on Chain %q",
					signer.Address(),
					id,
				)
			}
			continue
		}
		require.ErrorIs(t, err, environment.ErrCapabilityUnavailable, "e2etest: resolve funding on Chain %q", id)
		evmAccess, evmErr := chain.EVM()
		require.NoError(t, evmErr, "e2etest: resolve EVM access on attached Chain %q", id)
		for _, signer := range signers {
			balance, balanceErr := evmAccess.BalanceAt(t.Context(), signer.Address(), nil)
			require.NoError(
				t,
				balanceErr,
				"e2etest: query signer %s balance on attached Chain %q",
				signer.Address(),
				id,
			)
			require.GreaterOrEqual(
				t,
				balance.Cmp(minimum),
				0,
				"e2etest: signer %s on attached Chain %q has balance %s, need at least %s; provision it out of band",
				signer.Address(),
				id,
				balance,
				minimum,
			)
		}
	}
}

func mustBigInt(s string) *big.Int {
	v, ok := new(big.Int).SetString(s, 10)
	if !ok {
		panic(fmt.Sprintf("e2etest: invalid big.Int literal %q", s))
	}
	return v
}

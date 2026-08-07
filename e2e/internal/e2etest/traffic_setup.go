package e2etest

import (
	"context"
	"errors"
	"fmt"
	"math/big"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/cosmos/solidity-ibc-eureka/packages/go-abigen/erc1967proxy"
	"github.com/cosmos/solidity-ibc-eureka/packages/go-abigen/ift"
	"github.com/ethereum/go-ethereum/common"

	"github.com/cosmos/ibc/e2e/internal/harness/chain/evm"
	"github.com/cosmos/ibc/e2e/internal/harness/environment"
	"github.com/cosmos/ibc/e2e/internal/harness/environment/solidityibc/counter"
	"github.com/cosmos/ibc/e2e/internal/harness/environment/solidityibc/iftbatchtransfershim"
	"github.com/cosmos/ibc/e2e/internal/harness/environment/solidityibc/iftsendcallconstructor"
	"github.com/cosmos/ibc/e2e/internal/harness/environment/solidityibc/testerc20"
	"github.com/cosmos/ibc/e2e/internal/harness/ibclink"
)

const (
	relayerStopTimeout = 15 * time.Second
	blockNudgeInterval = 500 * time.Millisecond
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
	Token         common.Address
	Counter       common.Address
	IFT           common.Address
	IFTBatchShim  common.Address
	ICS20Transfer common.Address
	ICS27GMP      common.Address
	ICS26Router   common.Address
}

// RouteClients holds the realized client locators for one directed route.
type RouteClients struct {
	SourceClient string
	DestClient   string
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
	signers Signers,
	routes ...Route,
) (*ibclink.Driver, *Deployment) {
	t.Helper()
	return DeployWithRelayerConfig(t, env, signers, nil, routes...)
}

// DeployWithRelayerConfig is Deploy with a hook that adjusts the resolved
// relayer configuration after environment Attestors are assigned.
func DeployWithRelayerConfig(
	t testing.TB,
	env *environment.Environment,
	signers Signers,
	configure func(*ibclink.RelayerConfig),
	routes ...Route,
) (*ibclink.Driver, *Deployment) {
	t.Helper()
	if env == nil {
		t.Fatal("e2etest: Environment is required")
	}
	if err := signers.validate(); err != nil {
		t.Fatalf("e2etest: invalid signers: %v", err)
	}
	ensureSignerBalances(t, env, signers)

	dir := t.TempDir()
	signerKeyPath := filepath.Join(dir, "keys", relayerSignerAlias+".json")
	configPath := filepath.Join(dir, "ibc-link.config.yaml")
	driver, err := ibclink.NewDriver(configPath)
	if err != nil {
		t.Fatalf("e2etest: create driver: %v", err)
	}
	if bindErr := env.BindIBCLink(driver); bindErr != nil {
		t.Fatalf("e2etest: bind IBC Link process: %v", bindErr)
	}

	deployment := deployApps(t, env, signers, routes)
	config, options := buildConfig(t, env, driver, routes, deployment, signerKeyPath, filepath.Join(dir, "relayer.db"))
	if configure != nil {
		configure(&config)
	}
	if config.SignerType == "" || config.SignerType == ibclink.RelayerSignerLocal {
		if err := signers.storeRelayerKey(config.SignerKeyFile); err != nil {
			t.Fatalf("e2etest: store signers: %v", err)
		}
	}
	if writeErr := ibclink.WriteRelayerConfig(configPath, config); writeErr != nil {
		t.Fatalf("e2etest: write config: %v", writeErr)
	}
	driver.ConfigureRelayer(options)

	if migrationErr := driver.MigrateUp(t.Context()); migrationErr != nil {
		t.Fatalf("e2etest: migrate database: %v", migrationErr)
	}
	return driver, deployment
}

// StartRelayer starts the test relayer and registers idempotent teardown.
func StartRelayer(
	t testing.TB,
	driver *ibclink.Driver,
	env *environment.Environment,
) *ibclink.Relayer {
	t.Helper()
	if driver == nil {
		t.Fatal("e2etest: driver is required")
	}
	if env == nil {
		t.Fatal("e2etest: Environment is required")
	}

	relayer, err := driver.StartRelayer(t.Context())
	if err != nil {
		t.Fatalf("e2etest: start relayer: %v", err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), relayerStopTimeout)
		defer cancel()
		if err := relayer.Stop(ctx); err != nil {
			t.Errorf("e2etest: stop relayer: %v", err)
		}
	})

	connected := make(map[string]struct{}, len(relayer.Ready().ChainsConnected))
	for _, id := range relayer.Ready().ChainsConnected {
		connected[id] = struct{}{}
	}
	for _, id := range env.Chains() {
		chain, chainErr := env.Chain(id)
		if chainErr != nil {
			t.Fatalf("e2etest: resolve Chain %q: %v", id, chainErr)
		}
		if _, ok := connected[strconv.FormatUint(chain.EVMChainID(), 10)]; !ok {
			t.Fatalf("e2etest: relayer did not connect to Chain %q", id)
		}
	}
	startBlockNudger(t, env)
	return relayer
}

// startBlockNudger keeps instamine chains producing blocks while the relayer
// waits for "latest" minus relayFinalityOffset to cover packet heights: it
// broadcasts empty self-transfers from the otherwise idle protocol authority.
// Lanes whose chains produce blocks on their own don't need it.
func startBlockNudger(t testing.TB, env *environment.Environment) {
	t.Helper()
	if selectedLaneName(t) != laneAnvil {
		return
	}
	authority, err := evm.AccountFromHex(protocolAuthorityKeyHex)
	if err != nil {
		t.Fatalf("e2etest: block nudger authority: %v", err)
	}
	address := authority.Address()
	accesses := make([]*environment.EVM, 0, len(env.Chains()))
	for _, id := range env.Chains() {
		chain, chainErr := env.Chain(id)
		if chainErr != nil {
			t.Fatalf("e2etest: resolve Chain %q: %v", id, chainErr)
		}
		evmAccess, evmErr := chain.EVM()
		if evmErr != nil {
			t.Fatalf("e2etest: resolve EVM access on Chain %q: %v", id, evmErr)
		}
		accesses = append(accesses, evmAccess)
	}
	ctx := t.Context()
	for _, evmAccess := range accesses {
		// One goroutine per chain with a bounded broadcast: a chain whose
		// mining is paused must not wedge nudging elsewhere or hold its
		// access lease past the next tick.
		go func() {
			ticker := time.NewTicker(blockNudgeInterval)
			defer ticker.Stop()
			for {
				select {
				case <-ctx.Done():
					return
				case <-ticker.C:
				}
				nudgeCtx, cancel := context.WithTimeout(ctx, blockNudgeInterval)
				// Best effort: a failed nudge only delays the next block.
				_, _ = evmAccess.BroadcastTx(nudgeCtx, authority, &address, nil, nil)
				cancel()
			}
		}()
	}
}

func deployApps(
	t testing.TB,
	env *environment.Environment,
	signers Signers,
	routes []Route,
) *Deployment {
	t.Helper()
	deployment := &Deployment{
		chains: make(map[environment.ChainID]ChainDeployment, len(env.Chains())),
		routes: make(map[RouteID]RouteClients, len(routes)),
	}
	callConstructors := make(map[environment.ChainID]common.Address, len(env.Chains()))

	for _, id := range env.Chains() {
		chain, err := env.Chain(id)
		if err != nil {
			t.Fatalf("e2etest: resolve Chain %q: %v", id, err)
		}
		instance, err := env.IBCInstanceForChain(id)
		if err != nil {
			t.Fatalf("e2etest: resolve IBC Instance on Chain %q: %v", id, err)
		}
		evmAccess, err := chain.EVM()
		if err != nil {
			t.Fatalf("e2etest: resolve EVM access on Chain %q: %v", id, err)
		}

		ics20 := common.HexToAddress(string(instance.ICS20TransferAddress()))
		ics27 := common.HexToAddress(string(instance.ICS27GMPAddress()))
		router := common.HexToAddress(string(instance.Locator()))
		if ics20 == (common.Address{}) || ics27 == (common.Address{}) || router == (common.Address{}) {
			t.Fatalf("e2etest: Chain %q is missing the ICS20/ICS27 app stack", id)
		}

		token, err := deployAndMintToken(t.Context(), evmAccess, signers.application.account)
		if err != nil {
			t.Fatalf("e2etest: deploy TestERC20 on Chain %q: %v", id, err)
		}
		counterAddr, err := deployContract(
			t.Context(),
			evmAccess,
			signers.application.account,
			counter.CounterMetaData,
		)
		if err != nil {
			t.Fatalf("e2etest: deploy Counter on Chain %q: %v", id, err)
		}
		iftToken, callConstructor, err := deployIFTToken(t.Context(), evmAccess, signers.application.account, ics27)
		if err != nil {
			t.Fatalf("e2etest: deploy IFT on Chain %q: %v", id, err)
		}
		callConstructors[id] = callConstructor

		iftBatchShim, err := deployIFTBatchShim(
			t.Context(),
			evmAccess,
			signers.application.account,
			iftToken,
			initialTokenSupply,
		)
		if err != nil {
			t.Fatalf("e2etest: deploy IFT batch transfer shim on Chain %q: %v", id, err)
		}

		deployment.chains[id] = ChainDeployment{
			Token:         token,
			Counter:       counterAddr,
			IFT:           iftToken,
			IFTBatchShim:  iftBatchShim,
			ICS20Transfer: ics20,
			ICS27GMP:      ics27,
			ICS26Router:   router,
		}
	}

	for _, route := range routes {
		sourceClient, destClient, err := resolveRouteClients(env, route)
		if err != nil {
			t.Fatalf("e2etest: resolve clients for route %q: %v", route.ID, err)
		}
		deployment.routes[route.ID] = RouteClients{
			SourceClient: sourceClient,
			DestClient:   destClient,
		}
	}

	registerIFTBridges(t, env, signers, deployment, callConstructors, routes)
	return deployment
}

// registerIFTBridges points each route end's IFT at the counterparty IFT so
// cross-chain mints authenticate.
func registerIFTBridges(
	t testing.TB,
	env *environment.Environment,
	signers Signers,
	deployment *Deployment,
	callConstructors map[environment.ChainID]common.Address,
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
			{chain: route.Source, client: clients.SourceClient, counterparty: route.Destination},
			{chain: route.Destination, client: clients.DestClient, counterparty: route.Source, isDestination: true},
		}
		for _, end := range ends {
			if end.isDestination && route.SkipDestinationIFTBridge {
				continue
			}
			chain, err := env.Chain(end.chain)
			if err != nil {
				t.Fatalf("e2etest: resolve Chain %q: %v", end.chain, err)
			}
			evmAccess, err := chain.EVM()
			if err != nil {
				t.Fatalf("e2etest: resolve EVM access on Chain %q: %v", end.chain, err)
			}
			iftToken := deployment.chains[end.chain].IFT
			counterpartyIFT := deployment.chains[end.counterparty].IFT
			data, err := iftABI.Pack(
				"registerIFTBridge",
				end.client,
				counterpartyIFT.Hex(),
				callConstructors[end.chain],
			)
			if err != nil {
				t.Fatalf("e2etest: pack IFT registerIFTBridge: %v", err)
			}
			if _, err := evmAccess.BroadcastTx(
				t.Context(),
				signers.application.account,
				&iftToken,
				data,
				nil,
			); err != nil {
				t.Fatalf("e2etest: register IFT bridge for client %q on Chain %q: %v", end.client, end.chain, err)
			}
		}
	}
}

// relayFinalityOffset treats "latest" minus one block as final everywhere:
// the dev chains behind the harness never serve a moving "finalized" tag.
// The instamine anvil lane needs the block nudger for heights to keep moving.
const relayFinalityOffset = 1

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
		FinalityOffset: relayFinalityOffset,
	}
	options := ibclink.RelayerOptions{
		ChainIDs:     make(map[string]string, len(env.Chains())),
		ManualRoutes: make(map[string]bool, len(routes)),
	}
	for _, id := range env.Chains() {
		chain, err := env.Chain(id)
		if err != nil {
			t.Fatalf("e2etest: resolve Chain %q: %v", id, err)
		}
		rpc, err := driver.ChainRPC(string(id))
		if err != nil {
			t.Fatalf("e2etest: resolve Chain %q process binding: %v", id, err)
		}
		apps, ok := deployment.Chain(id)
		if !ok {
			t.Fatalf("e2etest: deployment has no Chain %q", id)
		}
		options.ChainIDs[string(id)] = strconv.FormatUint(chain.EVMChainID(), 10)
		config.Chains = append(config.Chains, ibclink.RelayerChain{
			ChainID:     options.ChainIDs[string(id)],
			RPC:         rpc,
			ICS26Router: apps.ICS26Router.Hex(),
		})
	}
	attestorIDs := env.Attestors()
	attestorSets := make(map[string]*ibclink.RelayerAttestorSet, len(attestorIDs))
	for _, id := range attestorIDs {
		attestor, err := env.Attestor(id)
		if err != nil {
			t.Fatalf("e2etest: resolve Attestor %q: %v", id, err)
		}
		client := attestor.IBCClient()
		chainID := options.ChainIDs[string(client.IBCInstance().Chain().ID())]
		key := chainID + "/" + string(client.Locator())
		set := attestorSets[key]
		if set == nil {
			set = &ibclink.RelayerAttestorSet{Threshold: int(client.MinRequiredSignatures())}
			attestorSets[key] = set
		}
		set.Attestors = append(set.Attestors, ibclink.RelayerAttestor{
			Name: string(attestor.ID()), Type: ibclink.RelayerAttestorRemote, GRPC: attestor.Endpoint(),
		})
	}

	connections := map[string]bool{}
	for _, route := range routes {
		clients, ok := deployment.RouteClients(route.ID)
		if !ok {
			t.Fatalf("e2etest: deployment has no route %q", route.ID)
		}
		options.ManualRoutes[string(route.ID)] = route.Manual

		sourceChain := options.ChainIDs[string(route.Source)]
		destinationChain := options.ChainIDs[string(route.Destination)]
		connection := ibclink.RelayerConnection{
			ChainA:  sourceChain,
			ClientA: clients.SourceClient,
			ChainB:  destinationChain,
			ClientB: clients.DestClient,
		}
		if connection.ChainB+"/"+connection.ClientB < connection.ChainA+"/"+connection.ClientA {
			connection.ChainA, connection.ClientA, connection.ChainB, connection.ClientB = connection.ChainB, connection.ClientB, connection.ChainA, connection.ClientA
		}
		key := connection.ChainA + "/" + connection.ClientA
		if !connections[key] {
			connections[key] = true
			connection.AttestorSetA = attestorSets[key]
			connection.AttestorSetB = attestorSets[connection.ChainB+"/"+connection.ClientB]
			config.Connections = append(config.Connections, connection)
		}

		config.Routes = append(config.Routes, ibclink.RelayerRoute{
			SourceChain:  sourceChain,
			SourceClient: clients.SourceClient,
		})
	}
	return config, options
}

func resolveRouteClients(env *environment.Environment, route Route) (string, string, error) {
	for _, id := range env.Connections() {
		connection, err := env.Connection(id)
		if err != nil {
			return "", "", err
		}
		aChain := connection.A().IBCInstance().Chain().ID()
		bChain := connection.B().IBCInstance().Chain().ID()
		switch {
		case aChain == route.Source && bChain == route.Destination:
			return string(connection.A().Locator()), string(connection.B().Locator()), nil
		case bChain == route.Source && aChain == route.Destination:
			return string(connection.B().Locator()), string(connection.A().Locator()), nil
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
// ERC1967 proxy) plus its send-call constructor, and mints the initial supply
// to the application signer.
func deployIFTToken(
	ctx context.Context,
	client *environment.EVM,
	sender evm.Account,
	ics27 common.Address,
) (common.Address, common.Address, error) {
	implementation, err := deployContract(ctx, client, sender, ift.ContractMetaData)
	if err != nil {
		return common.Address{}, common.Address{}, fmt.Errorf("e2etest: deploy IFT implementation: %w", err)
	}
	initialize, err := iftABI.Pack("initialize", sender.Address(), "IFT Token", "IFT", ics27)
	if err != nil {
		return common.Address{}, common.Address{}, fmt.Errorf("e2etest: pack IFT initialize: %w", err)
	}
	token, err := deployContract(ctx, client, sender, erc1967proxy.ContractMetaData, implementation, initialize)
	if err != nil {
		return common.Address{}, common.Address{}, fmt.Errorf("e2etest: deploy IFT proxy: %w", err)
	}
	callConstructor, err := deployContract(
		ctx,
		client,
		sender,
		iftsendcallconstructor.EVMIFTSendCallConstructorMetaData,
	)
	if err != nil {
		return common.Address{}, common.Address{}, fmt.Errorf("e2etest: deploy IFT send-call constructor: %w", err)
	}
	mint, err := iftABI.Pack("mint", sender.Address(), initialTokenSupply)
	if err != nil {
		return common.Address{}, common.Address{}, fmt.Errorf("e2etest: pack IFT mint: %w", err)
	}
	if _, err := client.BroadcastTx(ctx, sender, &token, mint, nil); err != nil {
		return common.Address{}, common.Address{}, fmt.Errorf("e2etest: mint IFT supply: %w", err)
	}
	return token, callConstructor, nil
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

func ensureSignerBalances(t testing.TB, env *environment.Environment, signers Signers) {
	t.Helper()
	addresses := signers.Addresses()
	actors := []struct {
		role    string
		address common.Address
	}{
		{role: "application", address: addresses.Application},
		{role: "relayer", address: addresses.Relayer},
	}
	minimum := RequiredSignerBalance()
	for _, id := range env.Chains() {
		chain, err := env.Chain(id)
		if err != nil {
			t.Fatalf("e2etest: resolve Chain %q: %v", id, err)
		}
		funding, err := chain.Funding()
		if err == nil {
			for _, actor := range actors {
				if fundErr := funding.EnsureEOABalance(t.Context(), actor.address, minimum); fundErr != nil {
					t.Fatalf("e2etest: fund %s signer on Chain %q: %v", actor.role, id, fundErr)
				}
			}
			continue
		}
		if !errors.Is(err, environment.ErrCapabilityUnavailable) {
			t.Fatalf("e2etest: resolve funding on Chain %q: %v", id, err)
		}
		evmAccess, evmErr := chain.EVM()
		if evmErr != nil {
			t.Fatalf("e2etest: resolve EVM access on attached Chain %q: %v", id, evmErr)
		}
		for _, actor := range actors {
			balance, balanceErr := evmAccess.BalanceAt(t.Context(), actor.address, nil)
			if balanceErr != nil {
				t.Fatalf("e2etest: query %s signer balance on attached Chain %q: %v", actor.role, id, balanceErr)
			}
			if balance.Cmp(minimum) < 0 {
				t.Fatalf(
					"e2etest: %s signer %s on attached Chain %q has balance %s, need at least %s; provision it out of band",
					actor.role,
					actor.address,
					id,
					balance,
					minimum,
				)
			}
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

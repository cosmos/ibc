// SPDX-License-Identifier: Apache-2.0

package traffic

import (
	"context"
	"errors"
	"fmt"
	"math/big"
	"path/filepath"
	"strconv"
	"time"

	"github.com/cosmos/solidity-ibc-eureka/packages/go-abigen/erc1967proxy"
	"github.com/cosmos/solidity-ibc-eureka/packages/go-abigen/evmiftsendcall"
	"github.com/cosmos/solidity-ibc-eureka/packages/go-abigen/ift"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"

	"github.com/cosmos/ibc/e2e/internal/harness/chain/evm"
	"github.com/cosmos/ibc/e2e/internal/harness/clientkind"
	"github.com/cosmos/ibc/e2e/internal/harness/environment"
	"github.com/cosmos/ibc/e2e/internal/harness/ibccli"
	"github.com/cosmos/ibc/gen/go/solidity-abi/counter"
	"github.com/cosmos/ibc/gen/go/solidity-abi/iftbatchtransfershim"
	"github.com/cosmos/ibc/gen/go/solidity-abi/testerc20"
)

var testERC20Transactor = mustBinding(testerc20.NewTestERC20Transactor(common.Address{}, nil))

// StopTimeout bounds how long stopping a relayer or prover may take.
const StopTimeout = 15 * time.Second

const weiPerEther int64 = 1_000_000_000_000_000_000

// Coins returns n whole tokens (n * 10^18 base units) of an 18-decimal token.
func Coins(n int64) *big.Int {
	return new(big.Int).Mul(big.NewInt(n), big.NewInt(weiPerEther))
}

var initialTokenSupply = Coins(1_000_000)

// RequiredSignerBalance is the minimum balance external Chains must provision
// for each test actor before deployment.
func RequiredSignerBalance() *big.Int {
	return Coins(1_000)
}

// ProtocolAuthorityAddress is the deployer funded for IBC Instance realization.
// Attached Chains must provision it out of band before Start.
func ProtocolAuthorityAddress() common.Address {
	account, err := evm.AccountFromHex(protocolAuthorityKeyHex)
	if err != nil {
		panic(fmt.Sprintf("traffic: protocol authority: %v", err))
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

// routeID names the route from source to destination, so routes between
// different Chain pairs never share an ID.
func routeID(source, destination environment.ChainID) RouteID {
	return RouteID(fmt.Sprintf("route-%s-to-%s", source, destination))
}

// distinctRoutes rejects a route given twice, which would otherwise replace
// the first one's clients.
func distinctRoutes(routes []Route) error {
	seen := make(map[RouteID]bool, len(routes))
	for _, route := range routes {
		if seen[route.ID] {
			return fmt.Errorf("traffic: route %q is given twice", route.ID)
		}
		seen[route.ID] = true
	}
	return nil
}

// NewRoute is the relayed route from source to destination.
func NewRoute(source, destination environment.ChainID) Route {
	return Route{ID: routeID(source, destination), Source: source, Destination: destination}
}

// NewManualRoute is NewRoute with automatic relaying off.
func NewManualRoute(source, destination environment.ChainID) Route {
	route := NewRoute(source, destination)
	route.Manual = true
	return route
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

// RouteClients holds the protocol client IDs and light client kinds for one
// directed route.
type RouteClients struct {
	SourceClientID   string
	DestClientID     string
	SourceClientKind clientkind.Kind
	DestClientKind   clientkind.Kind
}

// Deployment is the traffic-layer view of protocol apps and test tokens.
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

// Deploy deploys the test applications on each Chain (DeployApps), writes a
// black-box relayer configuration under dir, and runs its migration. It does
// not start the relayer. configure, when not nil, adjusts the resolved
// relayer configuration after environment Attestors are assigned. The caller
// owns dir: the returned driver's configuration, keys, and database live
// there, so remove it only after every process started from the driver has
// stopped.
func Deploy(
	ctx context.Context,
	env *environment.Environment,
	dir string,
	deployer Signer,
	relayer Signer,
	configure func(*ibccli.RelayerConfig),
	routes ...Route,
) (*ibccli.Driver, *Deployment, error) {
	switch {
	case env == nil:
		return nil, nil, errors.New("traffic: Environment is required")
	case dir == "":
		return nil, nil, errors.New("traffic: deployment directory is required")
	case relayer.key == nil:
		return nil, nil, errors.New("traffic: relayer signer is required")
	}
	// Before deploying, so an unfunded relayer costs no deployment.
	if err := ensureSignerBalances(ctx, env, relayer); err != nil {
		return nil, nil, err
	}
	deployment, err := DeployApps(ctx, env, deployer, routes...)
	if err != nil {
		return nil, nil, err
	}

	signerKeyPath := filepath.Join(dir, "keys", relayerSignerAlias+".json")
	configPath := filepath.Join(dir, "ibc-cli.config.yaml")
	driver, err := ibccli.NewDriver(configPath)
	if err != nil {
		return nil, nil, fmt.Errorf("traffic: create driver: %w", err)
	}
	if err = env.BindCLI(driver); err != nil {
		return nil, nil, fmt.Errorf("traffic: bind IBC CLI process: %w", err)
	}
	config, options, err := buildConfig(
		env, driver, routes, deployment, signerKeyPath, filepath.Join(dir, "relayer.db"),
	)
	if err != nil {
		return nil, nil, err
	}
	if configure != nil {
		configure(&config)
	}
	if config.SignerType == "" || config.SignerType == ibccli.RelayerSignerLocal {
		if err = relayer.storeKey(config.SignerKeyFile); err != nil {
			return nil, nil, fmt.Errorf("traffic: store relayer signer key: %w", err)
		}
	}
	if err = ibccli.WriteRelayerConfig(configPath, config); err != nil {
		return nil, nil, fmt.Errorf("traffic: write config: %w", err)
	}
	if err = driver.ConfigureRelayer(options); err != nil {
		return nil, nil, fmt.Errorf("traffic: configure relayer: %w", err)
	}

	if err = driver.MigrateUp(ctx); err != nil {
		return nil, nil, fmt.Errorf("traffic: migrate database: %w", err)
	}
	return driver, deployment, nil
}

// StartRelayer starts the relayer and checks that it connected to every
// Chain. The caller owns the returned relayer and stops it with Stop before
// closing the Environment or removing the deployment directory. When the
// connection check fails, StartRelayer stops the relayer before returning.
func StartRelayer(
	ctx context.Context,
	driver *ibccli.Driver,
	env *environment.Environment,
) (*ibccli.Relayer, error) {
	if driver == nil {
		return nil, errors.New("traffic: driver is required")
	}
	if env == nil {
		return nil, errors.New("traffic: Environment is required")
	}

	relayer, err := driver.StartRelayer(ctx)
	if err != nil {
		return nil, fmt.Errorf("traffic: start relayer: %w", err)
	}
	if err := verifyRelayerConnected(relayer, env); err != nil {
		stopCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), StopTimeout)
		defer cancel()
		return nil, errors.Join(err, relayer.Stop(stopCtx))
	}
	return relayer, nil
}

func verifyRelayerConnected(relayer *ibccli.Relayer, env *environment.Environment) error {
	connected := make(map[string]struct{}, len(relayer.Ready().ChainsConnected))
	for _, id := range relayer.Ready().ChainsConnected {
		connected[id] = struct{}{}
	}
	for _, id := range env.Chains() {
		chain, err := env.Chain(id)
		if err != nil {
			return fmt.Errorf("traffic: resolve Chain %q: %w", id, err)
		}
		if _, ok := connected[strconv.FormatUint(chain.EVMChainID(), 10)]; !ok {
			return fmt.Errorf("traffic: relayer did not connect to Chain %q", id)
		}
	}
	return nil
}

// DeployApps funds deployer and deploys the test applications on each Chain
// for routes: no relayer configuration, keys, or database. The deployer holds
// the minted token supplies.
func DeployApps(
	ctx context.Context,
	env *environment.Environment,
	deployer Signer,
	routes ...Route,
) (*Deployment, error) {
	switch {
	case env == nil:
		return nil, errors.New("traffic: Environment is required")
	case deployer.key == nil:
		return nil, errors.New("traffic: deployer signer is required")
	}
	if err := distinctRoutes(routes); err != nil {
		return nil, err
	}
	if err := ensureSignerBalances(ctx, env, deployer); err != nil {
		return nil, err
	}
	deployment := &Deployment{
		chains: make(map[environment.ChainID]ChainDeployment, len(env.Chains())),
		routes: make(map[RouteID]RouteClients, len(routes)),
	}
	for _, id := range env.Chains() {
		chain, err := env.Chain(id)
		if err != nil {
			return nil, fmt.Errorf("traffic: resolve Chain %q: %w", id, err)
		}
		instance, err := env.IBCInstanceForChain(id)
		if err != nil {
			return nil, fmt.Errorf("traffic: resolve IBC Instance on Chain %q: %w", id, err)
		}
		evmAccess, err := chain.EVM()
		if err != nil {
			return nil, fmt.Errorf("traffic: resolve EVM access on Chain %q: %w", id, err)
		}

		ics20 := common.HexToAddress(string(instance.ICS20TransferAddress()))
		ics27 := common.HexToAddress(string(instance.ICS27GMPAddress()))
		router := common.HexToAddress(string(instance.Locator()))
		if ics20 == (common.Address{}) || ics27 == (common.Address{}) || router == (common.Address{}) {
			return nil, fmt.Errorf("traffic: Chain %q is missing the ICS20/ICS27 app stack", id)
		}

		token, err := deployAndMintToken(ctx, evmAccess, deployer.account)
		if err != nil {
			return nil, fmt.Errorf("traffic: deploy TestERC20 on Chain %q: %w", id, err)
		}
		counterAddr, err := deployContract(ctx, evmAccess, deployer.account, counter.CounterMetaData)
		if err != nil {
			return nil, fmt.Errorf("traffic: deploy Counter on Chain %q: %w", id, err)
		}
		iftToken, err := deployIFTToken(ctx, evmAccess, deployer.account, ics27)
		if err != nil {
			return nil, fmt.Errorf("traffic: deploy IFT on Chain %q: %w", id, err)
		}
		iftSendCallConstructor, err := deployContract(
			ctx,
			evmAccess,
			deployer.account,
			evmiftsendcall.ContractMetaData,
		)
		if err != nil {
			return nil, fmt.Errorf("traffic: deploy IFT send-call constructor on Chain %q: %w", id, err)
		}

		iftBatchShim, err := deployIFTBatchShim(
			ctx,
			evmAccess,
			deployer.account,
			iftToken,
			initialTokenSupply,
		)
		if err != nil {
			return nil, fmt.Errorf("traffic: deploy IFT batch transfer shim on Chain %q: %w", id, err)
		}

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
		if err != nil {
			return nil, fmt.Errorf("traffic: resolve clients for route %q: %w", route.ID, err)
		}
		deployment.routes[route.ID] = RouteClients{
			SourceClientID:   sourceClient.ID(),
			DestClientID:     destClient.ID(),
			SourceClientKind: sourceClient.Kind(),
			DestClientKind:   destClient.Kind(),
		}
	}

	if err := registerIFTBridges(ctx, env, deployer, deployment, routes); err != nil {
		return nil, err
	}
	return deployment, nil
}

// registerIFTBridges points each route end's IFT at the counterparty IFT so
// cross-chain mints authenticate.
func registerIFTBridges(
	ctx context.Context,
	env *environment.Environment,
	deployer Signer,
	deployment *Deployment,
	routes []Route,
) error {
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
			if err != nil {
				return fmt.Errorf("traffic: resolve Chain %q: %w", end.chain, err)
			}
			evmAccess, err := chain.EVM()
			if err != nil {
				return fmt.Errorf("traffic: resolve EVM access on Chain %q: %w", end.chain, err)
			}
			iftToken := deployment.chains[end.chain].IFT
			counterpartyIFT := deployment.chains[end.counterparty].IFT
			if err := registerIFTBridge(
				ctx, evmAccess, deployer, end.chain,
				iftToken, end.client, counterpartyIFT,
				deployment.chains[end.chain].IFTSendCallConstructor,
			); err != nil {
				return err
			}
		}
	}
	return nil
}

func registerIFTBridge(
	ctx context.Context,
	evmAccess *environment.EVM,
	deployer Signer,
	chain environment.ChainID,
	iftToken common.Address,
	client string,
	counterpartyIFT common.Address,
	callConstructor common.Address,
) error {
	data, err := calldata(func(opts *bind.TransactOpts) (*types.Transaction, error) {
		return iftTransactor.RegisterIFTBridge(opts, client, counterpartyIFT.Hex(), callConstructor)
	})
	if err != nil {
		return fmt.Errorf("traffic: pack IFT registerIFTBridge: %w", err)
	}
	if _, err := evmAccess.BroadcastTx(ctx, deployer.account, &iftToken, data, nil); err != nil {
		return fmt.Errorf("traffic: register IFT bridge for client %q on Chain %q: %w", client, chain, err)
	}
	return nil
}

// baseConfig is what the relayer and prover configurations share: the signer,
// the database, the finality offset, the Environment's observability, and its
// Attestors.
func baseConfig(env *environment.Environment, signerKeyPath, dbPath string) (ibccli.RelayerConfig, error) {
	config := ibccli.RelayerConfig{
		DBPath:         dbPath,
		SignerAlias:    relayerSignerAlias,
		SignerKeyFile:  signerKeyPath,
		FinalityOffset: ibccli.HarnessFinalityOffset,
		Observability:  env.Observability(),
	}
	for _, id := range env.Attestors() {
		attestor, err := env.Attestor(id)
		if err != nil {
			return config, fmt.Errorf("traffic: resolve Attestor %q: %w", id, err)
		}
		config.Attestors = append(config.Attestors, ibccli.RelayerAttestor{
			Name: string(attestor.ID()), Type: ibccli.RelayerAttestorRemote, GRPC: attestor.Endpoint(),
		})
	}
	return config, nil
}

func buildConfig(
	env *environment.Environment,
	driver *ibccli.Driver,
	routes []Route,
	deployment *Deployment,
	signerKeyPath string,
	dbPath string,
) (ibccli.RelayerConfig, ibccli.RelayerOptions, error) {
	config, err := baseConfig(env, signerKeyPath, dbPath)
	if err != nil {
		return config, ibccli.RelayerOptions{}, err
	}
	config.ClearInterval = 5 * time.Second
	options := ibccli.RelayerOptions{
		ChainIDs:     make(map[string]string, len(env.Chains())),
		WaitPolicies: make(map[string]ibccli.WaitPolicy, len(routes)),
		RunID:        env.RunID(),
	}
	for _, id := range env.Chains() {
		chain, err := env.Chain(id)
		if err != nil {
			return config, options, fmt.Errorf("traffic: resolve Chain %q: %w", id, err)
		}
		rpc, err := driver.ChainRPC(string(id))
		if err != nil {
			return config, options, fmt.Errorf("traffic: resolve Chain %q process binding: %w", id, err)
		}
		apps, ok := deployment.Chain(id)
		if !ok {
			return config, options, fmt.Errorf("traffic: deployment has no Chain %q", id)
		}
		options.ChainIDs[string(id)] = strconv.FormatUint(chain.EVMChainID(), 10)
		relayerChain := ibccli.RelayerChain{
			ChainID:     options.ChainIDs[string(id)],
			RPC:         rpc,
			ICS26Router: apps.ICS26Router.Hex(),
		}
		if chain.WSURL() != "" {
			ws, wsErr := driver.ChainWS(string(id))
			if wsErr != nil {
				return config, options, fmt.Errorf("traffic: resolve Chain %q websocket binding: %w", id, wsErr)
			}
			relayerChain.WS = ws
		}
		config.Chains = append(config.Chains, relayerChain)
	}
	connections := map[[2]string]int{}
	for _, route := range routes {
		clients, ok := deployment.RouteClients(route.ID)
		if !ok {
			return config, options, fmt.Errorf("traffic: deployment has no route %q", route.ID)
		}
		source, err := env.Chain(route.Source)
		if err != nil {
			return config, options, fmt.Errorf(
				"traffic: resolve route %q source Chain %q: %w", route.ID, route.Source, err,
			)
		}
		destination, err := env.Chain(route.Destination)
		if err != nil {
			return config, options, fmt.Errorf(
				"traffic: resolve route %q destination Chain %q: %w", route.ID, route.Destination, err,
			)
		}
		options.WaitPolicies[string(route.ID)] = routeWaitPolicy(source.Timing(), destination.Timing())

		sourceChain := options.ChainIDs[string(route.Source)]
		destinationChain := options.ChainIDs[string(route.Destination)]
		connection := ibccli.RelayerConnection{
			A: ibccli.RelayerClientEnd{
				ChainID:    sourceChain,
				ClientID:   clients.SourceClientID,
				ClientType: clients.SourceClientKind,
				AutoRelay:  !route.Manual,
			},
			B: ibccli.RelayerClientEnd{
				ChainID:    destinationChain,
				ClientID:   clients.DestClientID,
				ClientType: clients.DestClientKind,
			},
		}
		mergeRelayerConnection(&config, connections, connection)
	}
	return config, options, nil
}

// mergeRelayerConnection combines directional routes into one reciprocal pair.
func mergeRelayerConnection(
	config *ibccli.RelayerConfig,
	indices map[[2]string]int,
	connection ibccli.RelayerConnection,
) {
	if connection.B.ChainID < connection.A.ChainID ||
		(connection.B.ChainID == connection.A.ChainID && connection.B.ClientID < connection.A.ClientID) {
		connection.A, connection.B = connection.B, connection.A
	}
	key := [2]string{connection.A.ChainID, connection.A.ClientID}
	if index, seen := indices[key]; seen {
		existing := &config.Connections[index]
		existing.A.AutoRelay = existing.A.AutoRelay || connection.A.AutoRelay
		existing.B.AutoRelay = existing.B.AutoRelay || connection.B.AutoRelay
		return
	}
	indices[key] = len(config.Connections)
	config.Connections = append(config.Connections, connection)
}

func routeWaitPolicy(source, destination environment.Timing) ibccli.WaitPolicy {
	return ibccli.WaitPolicy{
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
) (*environment.IBCClient, *environment.IBCClient, error) {
	for _, id := range env.Connections() {
		connection, err := env.Connection(id)
		if err != nil {
			return nil, nil, err
		}
		aChain := connection.A().IBCInstance().Chain().ID()
		bChain := connection.B().IBCInstance().Chain().ID()
		switch {
		case aChain == route.Source && bChain == route.Destination:
			return connection.A(), connection.B(), nil
		case bChain == route.Source && aChain == route.Destination:
			return connection.B(), connection.A(), nil
		}
	}
	return nil, nil, fmt.Errorf(
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
	data, err := calldata(func(opts *bind.TransactOpts) (*types.Transaction, error) {
		return testERC20Transactor.Mint(opts, sender.Address(), initialTokenSupply)
	})
	if err != nil {
		return common.Address{}, fmt.Errorf("traffic: pack TestERC20.mint: %w", err)
	}
	if _, err := client.BroadcastTx(ctx, sender, &token, data, nil); err != nil {
		return common.Address{}, fmt.Errorf("traffic: mint TestERC20: %w", err)
	}
	return token, nil
}

// IFT rate limit settings, generous so that e2e flows never hit them. IFT
// rejects every transfer until a rate limit is set.
var (
	iftRateLimitCapacity = new(big.Int).Sub(new(big.Int).Lsh(big.NewInt(1), 128), big.NewInt(1))
	iftRateLimitWindow   = big.NewInt(24 * 60 * 60)
)

// deployIFTToken deploys the IFT token (a UUPS implementation behind an
// ERC1967 proxy), sets its rate limit and mints the initial supply to the
// sender.
func deployIFTToken(
	ctx context.Context,
	client *environment.EVM,
	sender evm.Account,
	ics27 common.Address,
) (common.Address, error) {
	implementation, err := deployContract(ctx, client, sender, ift.ContractMetaData)
	if err != nil {
		return common.Address{}, fmt.Errorf("traffic: deploy IFT implementation: %w", err)
	}
	initialize, err := calldata(func(opts *bind.TransactOpts) (*types.Transaction, error) {
		return iftTransactor.Initialize(opts, sender.Address(), "IFT Token", "IFT", ics27)
	})
	if err != nil {
		return common.Address{}, fmt.Errorf("traffic: pack IFT initialize: %w", err)
	}
	token, err := deployContract(ctx, client, sender, erc1967proxy.ContractMetaData, implementation, initialize)
	if err != nil {
		return common.Address{}, fmt.Errorf("traffic: deploy IFT proxy: %w", err)
	}
	rateLimit, err := calldata(func(opts *bind.TransactOpts) (*types.Transaction, error) {
		return iftTransactor.SetIFTRateLimit(opts, iftRateLimitCapacity, iftRateLimitWindow)
	})
	if err != nil {
		return common.Address{}, fmt.Errorf("traffic: pack IFT setIFTRateLimit: %w", err)
	}
	if _, broadcastErr := client.BroadcastTx(ctx, sender, &token, rateLimit, nil); broadcastErr != nil {
		return common.Address{}, fmt.Errorf("traffic: set IFT rate limit: %w", broadcastErr)
	}
	mint, err := calldata(func(opts *bind.TransactOpts) (*types.Transaction, error) {
		return iftTransactor.Mint(opts, sender.Address(), initialTokenSupply)
	})
	if err != nil {
		return common.Address{}, fmt.Errorf("traffic: pack IFT mint: %w", err)
	}
	if _, err := client.BroadcastTx(ctx, sender, &token, mint, nil); err != nil {
		return common.Address{}, fmt.Errorf("traffic: mint IFT supply: %w", err)
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
		return common.Address{}, fmt.Errorf("traffic: deploy IFT batch transfer shim: %w", err)
	}
	mint, err := calldata(func(opts *bind.TransactOpts) (*types.Transaction, error) {
		return iftTransactor.Mint(opts, shim, amount)
	})
	if err != nil {
		return common.Address{}, fmt.Errorf("traffic: pack IFT mint for batch shim: %w", err)
	}
	if _, err := client.BroadcastTx(ctx, sender, &iftToken, mint, nil); err != nil {
		return common.Address{}, fmt.Errorf("traffic: mint IFT supply to batch shim: %w", err)
	}
	return shim, nil
}

// ensureSignerBalances makes every signer hold at least RequiredSignerBalance
// on each Chain of env. Managed Chains fund them through their funding
// capability; on an attached Chain the balance must already be provisioned
// out of band, and a shortfall is an error.
func ensureSignerBalances(ctx context.Context, env *environment.Environment, signers ...Signer) error {
	minimum := RequiredSignerBalance()
	for _, id := range env.Chains() {
		chain, err := env.Chain(id)
		if err != nil {
			return fmt.Errorf("traffic: resolve Chain %q: %w", id, err)
		}
		funding, err := chain.Funding()
		if err == nil {
			for _, signer := range signers {
				if fundErr := funding.EnsureEOABalance(ctx, signer.Address(), minimum); fundErr != nil {
					return fmt.Errorf("traffic: fund signer %s on Chain %q: %w", signer.Address(), id, fundErr)
				}
			}
			continue
		}
		if !errors.Is(err, environment.ErrCapabilityUnavailable) {
			return fmt.Errorf("traffic: resolve funding on Chain %q: %w", id, err)
		}
		evmAccess, evmErr := chain.EVM()
		if evmErr != nil {
			return fmt.Errorf("traffic: resolve EVM access on attached Chain %q: %w", id, evmErr)
		}
		for _, signer := range signers {
			balance, balanceErr := evmAccess.BalanceAt(ctx, signer.Address(), nil)
			if balanceErr != nil {
				return fmt.Errorf(
					"traffic: query signer %s balance on attached Chain %q: %w",
					signer.Address(),
					id,
					balanceErr,
				)
			}
			if balance.Cmp(minimum) < 0 {
				return fmt.Errorf(
					"traffic: signer %s on attached Chain %q has balance %s, need at least %s; "+
						"provision it out of band",
					signer.Address(),
					id,
					balance,
					minimum,
				)
			}
		}
	}
	return nil
}

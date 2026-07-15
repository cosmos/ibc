package environment

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"math/big"
	"path/filepath"
	"strconv"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"

	"github.com/cosmos/ibc/e2e/internal/harness/chain/evm"
	"github.com/cosmos/ibc/e2e/internal/harness/environment/solidityibc"
	"github.com/cosmos/ibc/e2e/internal/harness/ibclink"
)

type connectionDependencies struct {
	instances       map[IBCInstanceID]*IBCInstance
	attestorSpecs   map[ClientID][]AttestorSpec
	existingClients map[ClientID]*IBCClient
	preparedClients map[ClientID]*solidityibc.PreparedClient
}

type attestorDependencies struct {
	client   *IBCClient
	observed *IBCInstance
}

func acquireIBCInstances(
	ctx context.Context,
	declarations []IBCInstanceSpec,
	chains map[ChainID]*Chain,
	runtime Runtime,
	d drivers,
) (map[IBCInstanceID]*IBCInstance, error) {
	instances := make(map[IBCInstanceID]*IBCInstance, len(declarations))
	for _, declaration := range declarations {
		id := declaration.ibcInstanceID()
		instance, err := d.acquireIBCInstance(
			ctx,
			declaration,
			chains[declaration.ibcInstanceChain()],
			runtime,
		)
		if err != nil {
			return nil, fmt.Errorf("start IBC Instance %q failed: %w", id, err)
		}
		if instance == nil {
			return nil, fmt.Errorf("environment: IBC Instance %q adapter returned no resolved value", id)
		}
		instances[id] = instance
	}
	return instances, nil
}

func acquireConnections(
	ctx context.Context,
	spec Spec,
	instances map[IBCInstanceID]*IBCInstance,
	runtime Runtime,
	d drivers,
) (
	map[ConnectionID]*Connection,
	map[ClientID]*IBCClient,
	error,
) {
	connections := make(map[ConnectionID]*Connection, len(spec.Connections))
	clients := make(map[ClientID]*IBCClient, 2*len(spec.Connections))
	attestorUse := make(map[EVMAddress]ClientID)
	attestorSpecsByClient := make(map[ClientID][]AttestorSpec)
	for _, declaration := range spec.Attestors {
		attestorSpecsByClient[declaration.Client] = append(
			attestorSpecsByClient[declaration.Client],
			declaration,
		)
	}
	dependencies := connectionDependencies{
		instances: instances, attestorSpecs: attestorSpecsByClient,
	}
	if d.prepareConnections != nil {
		prepared, err := d.prepareConnections(ctx, spec, dependencies, runtime)
		if err != nil {
			return nil, nil, fmt.Errorf("prepare IBC Connections failed: %w", err)
		}
		dependencies = prepared
	}

	for _, declaration := range spec.Connections {
		connection, err := d.acquireConnection(ctx, declaration, dependencies, runtime)
		if err != nil {
			return nil, nil, fmt.Errorf("start IBC Connection %q failed: %w", declaration.ID, err)
		}
		if connection == nil || connection.a == nil || connection.b == nil {
			return nil, nil, fmt.Errorf(
				"environment: IBC Connection %q adapter returned an incomplete resolved value",
				declaration.ID,
			)
		}
		if err := recordResolvedAttestorUse(attestorUse, connection.a, connection.b); err != nil {
			return nil, nil, err
		}
		connections[declaration.ID] = connection
		clients[connection.a.id] = connection.a
		clients[connection.b.id] = connection.b
	}
	return connections, clients, nil
}

func recordResolvedAttestorUse(seen map[EVMAddress]ClientID, clients ...*IBCClient) error {
	for _, client := range clients {
		for _, address := range client.attestors {
			if previous, exists := seen[address]; exists {
				return fmt.Errorf(
					"attestor signer %s is reused by resolved IBC Clients %q and %q",
					address,
					previous,
					client.id,
				)
			}
			seen[address] = client.id
		}
	}
	return nil
}

func acquireAttestors(
	ctx context.Context,
	spec Spec,
	instances map[IBCInstanceID]*IBCInstance,
	clients map[ClientID]*IBCClient,
	runtime Runtime,
	ws workspace,
	d drivers,
	effects *effectJournal,
) (map[AttestorID]*Attestor, error) {
	attestors := make(map[AttestorID]*Attestor, len(spec.Attestors))
	for _, declaration := range spec.Attestors {
		observed, err := observedInstanceForClient(spec.Connections, declaration.Client, instances)
		if err != nil {
			return nil, err
		}
		acquisition, err := d.acquireAttestor(ctx, declaration, attestorDependencies{
			client: clients[declaration.Client], observed: observed,
		}, runtime, ws)
		if err != nil {
			if acquisition.release != nil {
				effects.append(cleanupEffect{
					description: acquisition.description,
					release:     acquisition.release,
				})
			}
			return nil, fmt.Errorf("start Attestor %q failed: %w", declaration.ID, err)
		}
		if acquisition.attestor == nil || acquisition.release == nil {
			return nil, fmt.Errorf(
				"environment: Attestor %q adapter returned an incomplete acquisition",
				declaration.ID,
			)
		}
		effects.append(cleanupEffect{
			description: acquisition.description,
			release:     acquisition.release,
		})
		attestors[declaration.ID] = acquisition.attestor
	}
	return attestors, nil
}

func observedInstanceForClient(
	connections []ConnectionSpec,
	clientID ClientID,
	instances map[IBCInstanceID]*IBCInstance,
) (*IBCInstance, error) {
	for _, connection := range connections {
		a := clientIdentity(connection.A)
		b := clientIdentity(connection.B)
		switch clientID {
		case a.ID:
			return instances[b.IBCInstance], nil
		case b.ID:
			return instances[a.IBCInstance], nil
		}
	}
	return nil, fmt.Errorf("environment: no counterparty IBC Instance for Client %q", clientID)
}

func acquireIBCInstance(
	ctx context.Context,
	declaration IBCInstanceSpec,
	chain *Chain,
	runtime Runtime,
) (*IBCInstance, error) {
	setup, err := solidityIBCSetup(ctx, chain)
	if err != nil {
		return nil, err
	}
	switch instance := declaration.(type) {
	case NewIBCInstance:
		authority, err := runtime.evmAccount(instance.Authority)
		if err != nil {
			return nil, err
		}
		if fundingErr := ensureProtocolAuthorityFunded(ctx, chain, authority); fundingErr != nil {
			return nil, fmt.Errorf("fund IBC Instance %q authority: %w", instance.ID, fundingErr)
		}
		resolved, err := setup.DeployInstance(ctx, authority)
		if err != nil {
			return nil, err
		}
		return &IBCInstance{
			id:            instance.ID,
			chain:         chain,
			locator:       IBCInstanceLocator(resolved.Router.Hex()),
			accessManager: EVMAddress(resolved.AccessManager.Hex()),
		}, nil
	case ExistingIBCInstance:
		resolved, err := setup.AttachInstance(ctx, common.HexToAddress(string(instance.Locator)))
		if err != nil {
			return nil, err
		}
		return &IBCInstance{
			id:            instance.ID,
			chain:         chain,
			locator:       IBCInstanceLocator(resolved.Router.Hex()),
			accessManager: EVMAddress(resolved.AccessManager.Hex()),
		}, nil
	default:
		return nil, fmt.Errorf("unsupported IBC Instance declaration %T", declaration)
	}
}

func prepareConnections(
	ctx context.Context,
	spec Spec,
	dependencies connectionDependencies,
	runtime Runtime,
) (connectionDependencies, error) {
	dependencies.existingClients = make(map[ClientID]*IBCClient)
	dependencies.preparedClients = make(map[ClientID]*solidityibc.PreparedClient)

	for _, connection := range spec.Connections {
		ends := []struct {
			label        string
			declaration  ClientSpec
			counterparty ClientSpec
		}{
			{label: "A", declaration: connection.A, counterparty: connection.B},
			{label: "B", declaration: connection.B, counterparty: connection.A},
		}
		locators := map[string]IBCClientLocator{
			"A": clientLocator(connection.ID, "A", connection.A),
			"B": clientLocator(connection.ID, "B", connection.B),
		}
		for _, end := range ends {
			identity := clientIdentity(end.declaration)
			switch client := end.declaration.(type) {
			case ExistingClient:
				resolved, err := acquireIBCClient(
					ctx,
					end.label,
					client,
					locators,
					dependencies,
					runtime,
				)
				if err != nil {
					return dependencies, fmt.Errorf(
						"prepare existing IBC Client %q: %w",
						identity.ID,
						err,
					)
				}
				dependencies.existingClients[identity.ID] = resolved
			case NewClient:
				instance := dependencies.instances[identity.IBCInstance]
				setup, err := solidityIBCSetup(ctx, instance.chain)
				if err != nil {
					return dependencies, err
				}
				authority, _ := runtime.evmAccount(client.Authority)
				counterparty := clientIdentity(end.counterparty)
				header, err := evmHeader(ctx, dependencies.instances[counterparty.IBCInstance].chain)
				if err != nil {
					return dependencies, fmt.Errorf(
						"prepare IBC Client %q counterparty header: %w",
						identity.ID,
						err,
					)
				}
				attestors := make([]common.Address, 0, len(dependencies.attestorSpecs[identity.ID]))
				for _, declaration := range dependencies.attestorSpecs[identity.ID] {
					account, _ := runtime.evmAccount(declaration.Authority)
					attestors = append(attestors, account.Address())
				}
				prepared, err := setup.PrepareClient(
					ctx,
					authority,
					common.HexToAddress(string(instance.locator)),
					solidityibc.AttestationClientConfig{
						ID:                    string(locators[end.label]),
						CounterpartyClientID:  string(locators[counterpartyEnd(end.label)]),
						Attestors:             attestors,
						MinRequiredSignatures: client.MinRequiredSignatures,
						InitialHeight:         header.Number.Uint64(),
						InitialTimestamp:      header.Time,
					},
				)
				if err != nil {
					return dependencies, fmt.Errorf("prepare IBC Client %q: %w", identity.ID, err)
				}
				dependencies.preparedClients[identity.ID] = prepared
			}
		}
	}
	return dependencies, nil
}

func acquireConnection(
	ctx context.Context,
	declaration ConnectionSpec,
	dependencies connectionDependencies,
	runtime Runtime,
) (*Connection, error) {
	locators := map[string]IBCClientLocator{
		"A": clientLocator(declaration.ID, "A", declaration.A),
		"B": clientLocator(declaration.ID, "B", declaration.B),
	}

	a, err := acquireIBCClient(
		ctx, "A", declaration.A, locators, dependencies, runtime,
	)
	if err != nil {
		return nil, err
	}
	b, err := acquireIBCClient(
		ctx, "B", declaration.B, locators, dependencies, runtime,
	)
	if err != nil {
		return nil, err
	}
	if a.counterparty != b.locator || b.counterparty != a.locator {
		return nil, fmt.Errorf("resolved IBC Clients are not reciprocal")
	}
	return &Connection{id: declaration.ID, a: a, b: b}, nil
}

func acquireIBCClient(
	ctx context.Context,
	end string,
	declaration ClientSpec,
	locators map[string]IBCClientLocator,
	dependencies connectionDependencies,
	runtime Runtime,
) (*IBCClient, error) {
	identity := clientIdentity(declaration)
	instance := dependencies.instances[identity.IBCInstance]
	counterpartyEnd := counterpartyEnd(end)
	counterpartyLocator := locators[counterpartyEnd]
	if resolved := dependencies.existingClients[identity.ID]; resolved != nil {
		return resolved, nil
	}

	var (
		resolved solidityibc.Client
		err      error
	)
	switch client := declaration.(type) {
	case ExistingClient:
		setup, setupErr := solidityIBCSetup(ctx, instance.chain)
		if setupErr != nil {
			return nil, setupErr
		}
		resolved, err = setup.AttachClient(
			ctx,
			common.HexToAddress(string(instance.locator)),
			string(client.Locator),
			string(counterpartyLocator),
		)
		if err != nil {
			return nil, err
		}
		if attestorErr := requireDeclaredAttestors(
			identity.ID,
			resolved.Attestors,
			dependencies.attestorSpecs[identity.ID],
			runtime,
		); attestorErr != nil {
			return nil, attestorErr
		}
	case NewClient:
		prepared := dependencies.preparedClients[identity.ID]
		if prepared == nil {
			return nil, fmt.Errorf("IBC Client %q was not prepared", identity.ID)
		}
		authority, err := runtime.evmAccount(client.Authority)
		if err != nil {
			return nil, err
		}
		if fundingErr := ensureProtocolAuthorityFunded(ctx, instance.chain, authority); fundingErr != nil {
			return nil, fmt.Errorf("fund IBC Client %q authority: %w", identity.ID, fundingErr)
		}
		resolved, err = prepared.Deploy(ctx)
		if err != nil {
			return nil, err
		}
	default:
		return nil, fmt.Errorf("unsupported IBC Client declaration %T", declaration)
	}

	attestors := make([]EVMAddress, len(resolved.Attestors))
	for i, address := range resolved.Attestors {
		attestors[i] = EVMAddress(address.Hex())
	}
	return &IBCClient{
		id:                    identity.ID,
		instance:              instance,
		locator:               IBCClientLocator(resolved.ID),
		lightClient:           EVMAddress(resolved.Address.Hex()),
		counterparty:          IBCClientLocator(resolved.CounterpartyClientID),
		attestors:             attestors,
		minRequiredSignatures: resolved.MinRequiredSignatures,
	}, nil
}

func ensureProtocolAuthorityFunded(ctx context.Context, chain *Chain, authority evm.Account) error {
	if chain == nil {
		return fmt.Errorf("missing resolved Chain")
	}
	if chain.funding == nil {
		return nil
	}
	funding, err := chain.Funding()
	if err != nil {
		return err
	}
	// Replenishing to the same minimum before each protocol mutation keeps the
	// contract independent of how much gas earlier setup transactions consumed.
	minimum := new(big.Int).Mul(big.NewInt(100), big.NewInt(1_000_000_000_000_000_000))
	return funding.EnsureEOABalance(ctx, authority.Address(), minimum)
}

func counterpartyEnd(end string) string {
	if end == "A" {
		return "B"
	}
	return "A"
}

func acquireAttestor(
	ctx context.Context,
	declaration AttestorSpec,
	dependencies attestorDependencies,
	runtime Runtime,
	ws workspace,
) (attestorAcquisition, error) {
	authority, err := runtime.evmAccount(declaration.Authority)
	if err != nil {
		return attestorAcquisition{}, err
	}
	process, err := ibclink.StartAttestor(ctx, ibclink.AttestorLaunch{
		BinaryPath:    ibclink.ResolvedBin(),
		WorkDir:       filepath.Join(ws.privateDir, "attestor-"+resourcePathToken(string(declaration.ID))),
		Name:          string(declaration.ID),
		ChainID:       strconv.FormatUint(dependencies.observed.chain.evmChainID, 10),
		PrivateKeyHex: runtime.Authorities[declaration.Authority].PrivateKeyHex,
	})
	if err != nil {
		if process != nil {
			return attestorAcquisition{
				description: fmt.Sprintf("stop Attestor %q", declaration.ID),
				release:     process.Stop,
			}, err
		}
		return attestorAcquisition{}, err
	}
	if process.SignerAddress() != authority.Address() {
		return attestorAcquisition{
			description: fmt.Sprintf("stop Attestor %q", declaration.ID),
			release:     process.Stop,
		}, fmt.Errorf("Attestor signer address does not match its runtime authority")
	}
	return attestorAcquisition{
		attestor: &Attestor{
			id:       declaration.ID,
			client:   dependencies.client,
			observed: dependencies.observed,
			signer:   EVMAddress(process.SignerAddress().Hex()),
		},
		description: fmt.Sprintf("stop Attestor %q", declaration.ID),
		release:     process.Stop,
	}, nil
}

func solidityIBCSetup(ctx context.Context, chain *Chain) (*solidityibc.Setup, error) {
	if chain == nil {
		return nil, fmt.Errorf("missing resolved Chain")
	}
	var setup *solidityibc.Setup
	ok, err := evm.WithChainClient(chain.impl, func(client *evm.EVMClient) error {
		var setupErr error
		setup, setupErr = solidityibc.NewSetup(ctx, client.Client())
		return setupErr
	})
	if !ok {
		return nil, fmt.Errorf("Chain %q has no EVM client", chain.id)
	}
	return setup, err
}

func evmHeader(ctx context.Context, chain *Chain) (*types.Header, error) {
	var header *types.Header
	ok, err := evm.WithChainClient(chain.impl, func(client *evm.EVMClient) error {
		var headerErr error
		header, headerErr = client.Client().HeaderByNumber(ctx, nil)
		return headerErr
	})
	if !ok {
		return nil, fmt.Errorf("Chain %q has no EVM client", chain.id)
	}
	return header, err
}

func clientLocator(connectionID ConnectionID, end string, declaration ClientSpec) IBCClientLocator {
	if existing, ok := declaration.(ExistingClient); ok {
		return existing.Locator
	}
	client := clientIdentity(declaration)
	hash := sha256.Sum256(
		[]byte(string(connectionID) + "\x00" + end + "\x00" + string(client.ID) + "\x00" + string(client.IBCInstance)),
	)
	return IBCClientLocator("link-" + hex.EncodeToString(hash[:]))
}

func requireDeclaredAttestors(
	clientID ClientID,
	onChain []common.Address,
	declarations []AttestorSpec,
	runtime Runtime,
) error {
	registered := make(map[common.Address]struct{}, len(onChain))
	for _, address := range onChain {
		registered[address] = struct{}{}
	}
	for _, declaration := range declarations {
		account, _ := runtime.evmAccount(declaration.Authority)
		if _, ok := registered[account.Address()]; !ok {
			return fmt.Errorf(
				"Attestor %q signer %s is not registered for existing IBC Client %q",
				declaration.ID,
				account.Address(),
				clientID,
			)
		}
	}
	return nil
}

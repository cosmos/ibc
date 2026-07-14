package environment

import (
	"cmp"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"math/big"
	"path/filepath"
	"slices"
	"strconv"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"

	"github.com/cosmos/ibc/link/harness/chain/evm"
	"github.com/cosmos/ibc/link/harness/ibclink"
	"github.com/cosmos/ibc/link/harness/internal/eureka"

	managedattestor "github.com/cosmos/ibc/link/harness/internal/attestor"
)

type connectionDependencies struct {
	instances       map[IBCInstanceID]*IBCInstance
	attestorSpecs   map[ClientID][]AttestorSpec
	existingClients map[ClientID]*IBCClient
	preparedClients map[ClientID]*eureka.PreparedClient
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
	resources *journal,
	receipts *protocolReceiptJournal,
) (map[IBCInstanceID]*IBCInstance, []FailureRecord, error) {
	instances := make(map[IBCInstanceID]*IBCInstance, len(declarations))
	for _, declaration := range sortedIBCInstanceSpecs(declarations) {
		id := declaration.ibcInstanceID()
		acquisition, err := d.acquireIBCInstance(
			ctx,
			declaration,
			chains[declaration.ibcInstanceChain()],
			runtime,
		)
		if acquisition.receipt != nil && instanceReceiptHasAnyOutput(*acquisition.receipt) {
			receipts.recordInstance(*acquisition.receipt)
		}
		if err != nil {
			if acquisition.receipt != nil && instanceReceiptHasTransactionEvidence(*acquisition.receipt) {
				_ = resources.recordAcquired(
					ResourceKindIBCInstance,
					string(id),
					acquisition.ownership,
					hostDependencies(acquisition.ownership, declaration.ibcInstanceChain())...,
				)
				_ = resources.setResourceState(ResourceKindIBCInstance, string(id), ResourceStateFailed)
			}
			return nil, []FailureRecord{{
				Kind: ResourceKindIBCInstance, ID: string(id),
			}}, fmt.Errorf("start IBC Instance %q failed: %w", id, err)
		}
		if acquisition.instance == nil {
			return nil, []FailureRecord{{
				Kind: ResourceKindIBCInstance, ID: string(id),
			}}, fmt.Errorf("environment: IBC Instance %q adapter returned no resolved value", id)
		}
		if err := resources.recordAcquired(
			ResourceKindIBCInstance,
			string(id),
			acquisition.ownership,
			hostDependencies(acquisition.ownership, declaration.ibcInstanceChain())...,
		); err != nil {
			return nil, []FailureRecord{{
				Kind: ResourceKindIBCInstance, ID: string(id),
			}}, err
		}
		if err := resources.setResourceState(ResourceKindIBCInstance, string(id), ResourceStateReady); err != nil {
			return nil, []FailureRecord{{
				Kind: ResourceKindIBCInstance, ID: string(id),
			}}, err
		}
		instances[id] = acquisition.instance
	}
	return instances, nil, nil
}

func acquireConnections(
	ctx context.Context,
	spec Spec,
	instances map[IBCInstanceID]*IBCInstance,
	runtime Runtime,
	d drivers,
	resources *journal,
	receipts *protocolReceiptJournal,
) (
	map[ConnectionID]*Connection,
	map[ClientID]*IBCClient,
	[]FailureRecord,
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
	for client, declarations := range attestorSpecsByClient {
		attestorSpecsByClient[client] = sortedAttestorSpecs(declarations)
	}
	dependencies := connectionDependencies{
		instances: instances, attestorSpecs: attestorSpecsByClient,
	}
	if d.prepareConnections != nil {
		prepared, failures, err := d.prepareConnections(ctx, spec, dependencies, runtime)
		recordPreparedExistingClients(spec, prepared, receipts)
		if err != nil {
			return nil, nil, failures, fmt.Errorf("prepare IBC Connections failed: %w", err)
		}
		dependencies = prepared
	}

	for _, declaration := range sortedConnectionSpecs(spec.Connections) {
		acquisition, err := d.acquireConnection(ctx, declaration, dependencies, runtime)
		if acquisition.receipt != nil {
			if acquisition.receipt.A != nil {
				receipts.recordConnectionEnd(declaration.ID, "A", *acquisition.receipt.A)
			}
			if acquisition.receipt.B != nil {
				receipts.recordConnectionEnd(declaration.ID, "B", *acquisition.receipt.B)
			}
		}
		if err == nil {
			if acquisition.connection == nil || acquisition.connection.a == nil || acquisition.connection.b == nil {
				return nil, nil, []FailureRecord{{
					Kind: ResourceKindIBCConnection, ID: string(declaration.ID),
				}}, fmt.Errorf("environment: IBC Connection %q adapter returned an incomplete resolved value", declaration.ID)
			}
			err = recordResolvedAttestorUse(attestorUse, acquisition.connection.a, acquisition.connection.b)
		}
		clientFailures, clientRecordCount, clientRecordErr := recordClientAcquisitions(
			resources,
			acquisition,
			instances,
		)
		if clientRecordErr != nil {
			return nil, nil, clientFailures, clientRecordErr
		}
		if err != nil {
			if clientRecordCount != 0 {
				_ = resources.recordAcquired(
					ResourceKindIBCConnection,
					string(declaration.ID),
					acquisition.ownership,
					connectionHostDependencies(declaration, instances, acquisition.ownership)...,
				)
				_ = resources.setResourceState(ResourceKindIBCConnection, string(declaration.ID), ResourceStateFailed)
			}
			return nil, nil, []FailureRecord{{
				Kind: ResourceKindIBCConnection, ID: string(declaration.ID),
			}}, fmt.Errorf("start IBC Connection %q failed: %w", declaration.ID, err)
		}
		if err := resources.recordAcquired(
			ResourceKindIBCConnection,
			string(declaration.ID),
			acquisition.ownership,
			connectionHostDependencies(declaration, instances, acquisition.ownership)...,
		); err != nil {
			return nil, nil, []FailureRecord{{
				Kind: ResourceKindIBCConnection, ID: string(declaration.ID),
			}}, err
		}
		if err := resources.setResourceState(
			ResourceKindIBCConnection,
			string(declaration.ID),
			ResourceStateReady,
		); err != nil {
			return nil, nil, []FailureRecord{{
				Kind: ResourceKindIBCConnection, ID: string(declaration.ID),
			}}, err
		}
		connections[declaration.ID] = acquisition.connection
		clients[acquisition.connection.a.id] = acquisition.connection.a
		clients[acquisition.connection.b.id] = acquisition.connection.b
	}
	return connections, clients, nil, nil
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

func recordClientAcquisitions(
	resources *journal,
	acquisition connectionAcquisition,
	instances map[IBCInstanceID]*IBCInstance,
) ([]FailureRecord, int, error) {
	recorded := 0
	for _, client := range []clientAcquisition{acquisition.a, acquisition.b} {
		if !client.attempted || client.receipt == nil {
			continue
		}
		state := ResourceStateFailed
		if client.client != nil {
			state = ResourceStateReady
		} else if !clientReceiptHasTransactionEvidence(client.receipt) {
			continue
		}
		id := client.receipt.ID
		if err := resources.recordAcquired(
			ResourceKindIBCClient,
			string(id),
			client.ownership,
			clientHostDependencies(client, instances)...,
		); err != nil {
			failure := FailureRecord{Kind: ResourceKindIBCClient, ID: string(id)}
			return []FailureRecord{failure}, recorded, err
		}
		if err := resources.setResourceState(ResourceKindIBCClient, string(id), state); err != nil {
			failure := FailureRecord{Kind: ResourceKindIBCClient, ID: string(id)}
			return []FailureRecord{failure}, recorded, err
		}
		recorded++
	}
	return nil, recorded, nil
}

func recordPreparedExistingClients(
	spec Spec,
	dependencies connectionDependencies,
	receipts *protocolReceiptJournal,
) {
	for _, connection := range sortedConnectionSpecs(spec.Connections) {
		for _, end := range []struct {
			label       string
			declaration ClientSpec
		}{
			{label: "A", declaration: connection.A},
			{label: "B", declaration: connection.B},
		} {
			identity := clientIdentity(end.declaration)
			resolved := dependencies.existingClients[identity.ID]
			if resolved == nil {
				continue
			}
			receipts.recordConnectionEnd(connection.ID, end.label, IBCClientReceipt{
				ID:                 identity.ID,
				IBCInstance:        identity.IBCInstance,
				Locator:            resolved.locator,
				LightClientAddress: resolved.lightClient,
			})
		}
	}
}

func acquireAttestors(
	ctx context.Context,
	spec Spec,
	instances map[IBCInstanceID]*IBCInstance,
	clients map[ClientID]*IBCClient,
	runtime Runtime,
	ws workspace,
	d drivers,
	resources *journal,
	effects *effectJournal,
) (map[AttestorID]*Attestor, []FailureRecord, error) {
	attestors := make(map[AttestorID]*Attestor, len(spec.Attestors))
	for _, declaration := range sortedAttestorSpecs(spec.Attestors) {
		observed, err := observedInstanceForClient(spec.Connections, declaration.Client, instances)
		if err != nil {
			return nil, []FailureRecord{{
				Kind: ResourceKindAttestor, ID: string(declaration.ID),
			}}, err
		}
		acquisition, err := d.acquireAttestor(ctx, declaration, attestorDependencies{
			client: clients[declaration.Client], observed: observed,
		}, runtime, ws)
		if err != nil {
			if acquisition.release != nil {
				key := resourceKey{kind: ResourceKindAttestor, id: string(declaration.ID)}
				if recordErr := resources.recordAcquired(key.kind, key.id, acquisition.ownership); recordErr != nil {
					_ = acquisition.release(context.WithoutCancel(ctx))
					return nil, []FailureRecord{{
						Kind: key.kind, ID: key.id,
					}}, errors.Join(err, recordErr)
				}
				effects.append(cleanupEffect{
					key:       key,
					ownership: acquisition.ownership,
					action:    acquisition.action,
					release:   acquisition.release,
				})
				_ = resources.setResourceState(key.kind, key.id, ResourceStateFailed)
			}
			return nil, []FailureRecord{{
				Kind: ResourceKindAttestor, ID: string(declaration.ID),
			}}, fmt.Errorf("start Attestor %q failed: %w", declaration.ID, err)
		}
		if acquisition.attestor == nil || acquisition.release == nil {
			return nil, []FailureRecord{{
				Kind: ResourceKindAttestor, ID: string(declaration.ID),
			}}, fmt.Errorf("environment: Attestor %q adapter returned an incomplete acquisition", declaration.ID)
		}
		key := resourceKey{kind: ResourceKindAttestor, id: string(declaration.ID)}
		if err := resources.recordAcquired(key.kind, key.id, acquisition.ownership); err != nil {
			_ = acquisition.release(context.WithoutCancel(ctx))
			return nil, []FailureRecord{{
				Kind: key.kind, ID: key.id,
			}}, err
		}
		effects.append(cleanupEffect{
			key: key, ownership: acquisition.ownership, action: acquisition.action, release: acquisition.release,
		})
		if err := resources.setResourceState(key.kind, key.id, ResourceStateReady); err != nil {
			return nil, []FailureRecord{{
				Kind: key.kind, ID: key.id,
			}}, err
		}
		attestors[declaration.ID] = acquisition.attestor
	}
	return attestors, nil, nil
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
) (instanceAcquisition, error) {
	setup, err := eurekaSetup(ctx, chain)
	if err != nil {
		return instanceAcquisition{}, err
	}
	switch instance := declaration.(type) {
	case NewIBCInstance:
		authority, err := runtime.evmAccount(instance.Authority)
		if err != nil {
			return instanceAcquisition{}, err
		}
		if fundingErr := ensureProtocolAuthorityFunded(ctx, chain, authority); fundingErr != nil {
			return instanceAcquisition{}, fmt.Errorf("fund IBC Instance %q authority: %w", instance.ID, fundingErr)
		}
		resolved, evidence, err := setup.DeployInstance(ctx, authority)
		receipt := translateInstanceReceipts(instance, evidence)
		acquisition := instanceAcquisition{
			ownership: protocolCreationOwnership(chain),
			receipt:   &receipt,
		}
		if err != nil {
			return acquisition, err
		}
		acquisition.instance = &IBCInstance{
			id:            instance.ID,
			chain:         chain,
			locator:       IBCInstanceLocator(resolved.Router.Hex()),
			accessManager: EVMAddress(resolved.AccessManager.Hex()),
		}
		return acquisition, nil
	case ExistingIBCInstance:
		if !common.IsHexAddress(string(instance.Locator)) {
			return instanceAcquisition{}, fmt.Errorf("IBC Instance locator %q is not an EVM address", instance.Locator)
		}
		resolved, err := setup.AttachInstance(ctx, common.HexToAddress(string(instance.Locator)))
		if err != nil {
			return instanceAcquisition{}, err
		}
		return instanceAcquisition{
			instance: &IBCInstance{
				id:            instance.ID,
				chain:         chain,
				locator:       IBCInstanceLocator(resolved.Router.Hex()),
				accessManager: EVMAddress(resolved.AccessManager.Hex()),
			},
			ownership: OwnershipBorrowed,
		}, nil
	default:
		return instanceAcquisition{}, fmt.Errorf("unsupported IBC Instance declaration %T", declaration)
	}
}

func prepareConnections(
	ctx context.Context,
	spec Spec,
	dependencies connectionDependencies,
	runtime Runtime,
) (connectionDependencies, []FailureRecord, error) {
	dependencies.existingClients = make(map[ClientID]*IBCClient)
	dependencies.preparedClients = make(map[ClientID]*eureka.PreparedClient)

	for _, connection := range sortedConnectionSpecs(spec.Connections) {
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
			failure := []FailureRecord{{
				Kind: ResourceKindIBCClient, ID: string(identity.ID),
			}}
			switch client := end.declaration.(type) {
			case ExistingClient:
				resolved, _, err := acquireIBCClient(
					ctx,
					end.label,
					client,
					locators,
					dependencies,
					runtime,
				)
				if err != nil {
					return dependencies, failure, fmt.Errorf(
						"prepare existing IBC Client %q: %w",
						identity.ID,
						err,
					)
				}
				dependencies.existingClients[identity.ID] = resolved
			case NewClient:
				instance := dependencies.instances[identity.IBCInstance]
				setup, err := eurekaSetup(ctx, instance.chain)
				if err != nil {
					return dependencies, failure, err
				}
				authority, _ := runtime.evmAccount(client.Authority)
				counterparty := clientIdentity(end.counterparty)
				header, err := evmHeader(ctx, dependencies.instances[counterparty.IBCInstance].chain)
				if err != nil {
					return dependencies, failure, fmt.Errorf(
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
					eureka.AttestationClientConfig{
						ID:                    string(locators[end.label]),
						CounterpartyClientID:  string(locators[counterpartyEnd(end.label)]),
						Attestors:             attestors,
						MinRequiredSignatures: client.MinRequiredSignatures,
						InitialHeight:         header.Number.Uint64(),
						InitialTimestamp:      header.Time,
					},
				)
				if err != nil {
					return dependencies, failure, fmt.Errorf("prepare IBC Client %q: %w", identity.ID, err)
				}
				dependencies.preparedClients[identity.ID] = prepared
			}
		}
	}

	seen := make(map[EVMAddress]ClientID)
	for _, connection := range sortedConnectionSpecs(spec.Connections) {
		for _, declaration := range []ClientSpec{connection.A, connection.B} {
			identity := clientIdentity(declaration)
			resolved := dependencies.existingClients[identity.ID]
			if resolved == nil {
				resolved = &IBCClient{id: identity.ID}
				for _, attestor := range dependencies.attestorSpecs[identity.ID] {
					account, _ := runtime.evmAccount(attestor.Authority)
					resolved.attestors = append(resolved.attestors, EVMAddress(account.Address().Hex()))
				}
			}
			if err := recordResolvedAttestorUse(seen, resolved); err != nil {
				return dependencies, []FailureRecord{{
					Kind: ResourceKindIBCClient, ID: string(identity.ID),
				}}, err
			}
		}
	}
	return dependencies, nil, nil
}

func acquireConnection(
	ctx context.Context,
	declaration ConnectionSpec,
	dependencies connectionDependencies,
	runtime Runtime,
) (connectionAcquisition, error) {
	receipt := IBCConnectionReceipt{ID: declaration.ID}
	aOwnership := clientOwnership(declaration.A, dependencies.instances)
	bOwnership := clientOwnership(declaration.B, dependencies.instances)
	acquisition := connectionAcquisition{
		ownership: connectionOwnership(aOwnership, bOwnership),
		receipt:   &receipt,
		a:         clientAcquisition{ownership: aOwnership},
		b:         clientAcquisition{ownership: bOwnership},
	}
	locators := map[string]IBCClientLocator{
		"A": clientLocator(declaration.ID, "A", declaration.A),
		"B": clientLocator(declaration.ID, "B", declaration.B),
	}

	a, aReceipt, err := acquireIBCClient(
		ctx, "A", declaration.A, locators, dependencies, runtime,
	)
	receipt.A = aReceipt
	acquisition.a.client = a
	acquisition.a.receipt = aReceipt
	acquisition.a.attempted = true
	if err != nil {
		return acquisition, err
	}
	b, bReceipt, err := acquireIBCClient(
		ctx, "B", declaration.B, locators, dependencies, runtime,
	)
	receipt.B = bReceipt
	acquisition.b.client = b
	acquisition.b.receipt = bReceipt
	acquisition.b.attempted = true
	if err != nil {
		return acquisition, err
	}
	if a.counterparty != b.locator || b.counterparty != a.locator {
		return acquisition, fmt.Errorf("resolved IBC Clients are not reciprocal")
	}
	acquisition.connection = &Connection{id: declaration.ID, a: a, b: b}
	return acquisition, nil
}

func acquireIBCClient(
	ctx context.Context,
	end string,
	declaration ClientSpec,
	locators map[string]IBCClientLocator,
	dependencies connectionDependencies,
	runtime Runtime,
) (*IBCClient, *IBCClientReceipt, error) {
	identity := clientIdentity(declaration)
	instance := dependencies.instances[identity.IBCInstance]
	locator := locators[end]
	counterpartyEnd := counterpartyEnd(end)
	counterpartyLocator := locators[counterpartyEnd]
	receipt := &IBCClientReceipt{
		ID: identity.ID, IBCInstance: identity.IBCInstance, Locator: locator,
	}
	if resolved := dependencies.existingClients[identity.ID]; resolved != nil {
		receipt.LightClientAddress = resolved.lightClient
		return resolved, receipt, nil
	}

	var (
		resolved eureka.Client
		err      error
	)
	switch client := declaration.(type) {
	case ExistingClient:
		setup, setupErr := eurekaSetup(ctx, instance.chain)
		if setupErr != nil {
			return nil, receipt, setupErr
		}
		resolved, err = setup.AttachClient(
			ctx,
			common.HexToAddress(string(instance.locator)),
			string(client.Locator),
			string(counterpartyLocator),
		)
		if err != nil {
			return nil, receipt, err
		}
		receipt.LightClientAddress = EVMAddress(resolved.Address.Hex())
		if attestorErr := requireDeclaredAttestors(
			identity.ID,
			resolved.Attestors,
			dependencies.attestorSpecs[identity.ID],
			runtime,
		); attestorErr != nil {
			return nil, receipt, attestorErr
		}
	case NewClient:
		prepared := dependencies.preparedClients[identity.ID]
		if prepared == nil {
			return nil, receipt, fmt.Errorf("IBC Client %q was not prepared", identity.ID)
		}
		authority, err := runtime.evmAccount(client.Authority)
		if err != nil {
			return nil, receipt, err
		}
		if fundingErr := ensureProtocolAuthorityFunded(ctx, instance.chain, authority); fundingErr != nil {
			return nil, receipt, fmt.Errorf("fund IBC Client %q authority: %w", identity.ID, fundingErr)
		}
		var evidence eureka.ClientReceipts
		resolved, evidence, err = prepared.Deploy(ctx)
		translateClientReceipts(receipt, evidence)
		if resolved.Address != (common.Address{}) {
			receipt.LightClientAddress = EVMAddress(resolved.Address.Hex())
		}
		if err != nil {
			return nil, receipt, err
		}
	default:
		return nil, receipt, fmt.Errorf("unsupported IBC Client declaration %T", declaration)
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
	}, receipt, nil
}

func ensureProtocolAuthorityFunded(ctx context.Context, chain *Chain, authority evm.Account) error {
	if chain == nil {
		return fmt.Errorf("missing resolved Chain")
	}
	if chain.ownership == OwnershipBorrowed {
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
	process, err := managedattestor.Start(ctx, managedattestor.Spec{
		BinaryPath:    ibclink.ResolvedRealBin(),
		WorkDir:       filepath.Join(ws.privateDir, "attestor-"+resourcePathToken(string(declaration.ID))),
		Name:          string(declaration.ID),
		ChainID:       strconv.FormatUint(dependencies.observed.chain.evmChainID, 10),
		PrivateKeyHex: runtime.Authorities[declaration.Authority].PrivateKeyHex,
	})
	if err != nil {
		if process != nil {
			return attestorAcquisition{
				ownership: OwnershipOwnedEphemeral,
				action:    CleanupActionStop,
				release:   process.Stop,
			}, err
		}
		return attestorAcquisition{}, err
	}
	if process.SignerAddress() != authority.Address() {
		return attestorAcquisition{
			ownership: OwnershipOwnedEphemeral,
			action:    CleanupActionStop,
			release:   process.Stop,
		}, fmt.Errorf("Attestor signer address does not match its runtime authority")
	}
	return attestorAcquisition{
		attestor: &Attestor{
			id:       declaration.ID,
			client:   dependencies.client,
			observed: dependencies.observed,
			signer:   EVMAddress(process.SignerAddress().Hex()),
		},
		ownership: OwnershipOwnedEphemeral,
		action:    CleanupActionStop,
		release:   process.Stop,
	}, nil
}

func eurekaSetup(ctx context.Context, chain *Chain) (*eureka.Setup, error) {
	if chain == nil {
		return nil, fmt.Errorf("missing resolved Chain")
	}
	var setup *eureka.Setup
	ok, err := evm.WithChainClient(chain.impl, func(client *evm.EVMClient) error {
		var setupErr error
		setup, setupErr = eureka.NewSetup(ctx, client.Client())
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

func protocolCreationOwnership(chain *Chain) Ownership {
	if chain != nil && chain.ownership == OwnershipOwnedEphemeral {
		return OwnershipOwnedHostScoped
	}
	return OwnershipOwnedDurable
}

func clientOwnership(declaration ClientSpec, instances map[IBCInstanceID]*IBCInstance) Ownership {
	if _, existing := declaration.(ExistingClient); existing {
		return OwnershipBorrowed
	}
	identity := clientIdentity(declaration)
	return protocolCreationOwnership(instances[identity.IBCInstance].chain)
}

func connectionOwnership(a, b Ownership) Ownership {
	if a == OwnershipOwnedHostScoped || b == OwnershipOwnedHostScoped {
		return OwnershipOwnedHostScoped
	}
	if a == OwnershipOwnedDurable || b == OwnershipOwnedDurable {
		return OwnershipOwnedDurable
	}
	return OwnershipBorrowed
}

func hostDependencies(ownership Ownership, hosts ...ChainID) []ChainID {
	if ownership != OwnershipOwnedHostScoped {
		return nil
	}
	return hosts
}

func clientHostDependencies(
	client clientAcquisition,
	instances map[IBCInstanceID]*IBCInstance,
) []ChainID {
	if client.ownership != OwnershipOwnedHostScoped || client.receipt == nil {
		return nil
	}
	instance := instances[client.receipt.IBCInstance]
	if instance == nil || instance.chain == nil {
		return nil
	}
	return []ChainID{instance.chain.id}
}

func connectionHostDependencies(
	declaration ConnectionSpec,
	instances map[IBCInstanceID]*IBCInstance,
	ownership Ownership,
) []ChainID {
	if ownership != OwnershipOwnedHostScoped {
		return nil
	}
	hosts := make([]ChainID, 0, 2)
	for _, client := range []ClientSpec{declaration.A, declaration.B} {
		instance := instances[clientIdentity(client).IBCInstance]
		if instance != nil && instance.chain != nil && instance.chain.ownership == OwnershipOwnedEphemeral {
			hosts = append(hosts, instance.chain.id)
		}
	}
	return hosts
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

func translateInstanceReceipts(
	declaration NewIBCInstance,
	in eureka.InstanceReceipts,
) IBCInstanceReceipt {
	return IBCInstanceReceipt{
		ID:                   declaration.ID,
		Chain:                declaration.Chain,
		AccessManager:        translateTransactionEvidence(in.AccessManager),
		RouterImplementation: translateTransactionEvidence(in.RouterImplementation),
		RouterProxy:          translateTransactionEvidence(in.RouterProxy),
	}
}

func translateClientReceipts(out *IBCClientReceipt, in eureka.ClientReceipts) {
	out.LightClientDeployment = translateTransactionEvidence(in.LightClient)
	out.Registration = translateTransactionEvidence(in.Registration)
	if in.LightClient != nil {
		address := in.LightClient.ContractAddress
		if address == (common.Address{}) {
			address = in.LightClient.PredictedContractAddress
		}
		if address != (common.Address{}) {
			out.LightClientAddress = EVMAddress(address.Hex())
		}
	}
}

func translateTransactionEvidence(in *eureka.TransactionEvidence) *EVMTransactionEvidence {
	if in == nil {
		return nil
	}
	out := &EVMTransactionEvidence{
		Hash:        in.Hash.Hex(),
		Submission:  EVMSubmissionStatus(in.Submission),
		Mined:       in.Mined,
		BlockNumber: in.BlockNumber,
		Status:      in.Status,
	}
	if in.PredictedContractAddress != (common.Address{}) {
		out.PredictedContractAddress = EVMAddress(in.PredictedContractAddress.Hex())
	}
	if in.ContractAddress != (common.Address{}) {
		out.ContractAddress = EVMAddress(in.ContractAddress.Hex())
	}
	return out
}

func instanceReceiptHasTransactionEvidence(receipt IBCInstanceReceipt) bool {
	return hasTransactionHash(receipt.AccessManager) ||
		hasTransactionHash(receipt.RouterImplementation) ||
		hasTransactionHash(receipt.RouterProxy)
}

func instanceReceiptHasAnyOutput(receipt IBCInstanceReceipt) bool {
	return receipt.AccessManager != nil || receipt.RouterImplementation != nil || receipt.RouterProxy != nil
}

func clientReceiptHasTransactionEvidence(client *IBCClientReceipt) bool {
	return client != nil &&
		(hasTransactionHash(client.LightClientDeployment) || hasTransactionHash(client.Registration))
}

func hasTransactionHash(evidence *EVMTransactionEvidence) bool {
	return evidence != nil && evidence.Hash != ""
}

func sortedAttestorSpecs(in []AttestorSpec) []AttestorSpec {
	out := slices.Clone(in)
	slices.SortFunc(out, func(a, b AttestorSpec) int { return cmp.Compare(string(a.ID), string(b.ID)) })
	return out
}

func sortedIBCInstanceSpecs(in []IBCInstanceSpec) []IBCInstanceSpec {
	out := slices.Clone(in)
	slices.SortFunc(out, func(a, b IBCInstanceSpec) int {
		return cmp.Compare(string(a.ibcInstanceID()), string(b.ibcInstanceID()))
	})
	return out
}

func sortedConnectionSpecs(in []ConnectionSpec) []ConnectionSpec {
	out := slices.Clone(in)
	slices.SortFunc(out, func(a, b ConnectionSpec) int {
		return cmp.Compare(string(a.ID), string(b.ID))
	})
	return out
}

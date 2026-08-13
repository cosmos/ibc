// SPDX-License-Identifier: Apache-2.0

package environment

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"math/big"
	"path/filepath"
	"strconv"
	"sync"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"

	"github.com/cosmos/ibc/e2e/internal/harness/chain/evm"
	"github.com/cosmos/ibc/e2e/internal/harness/environment/solidityibc"
	"github.com/cosmos/ibc/e2e/internal/harness/ibclink"
)

type connectionDependencies struct {
	instances       map[IBCInstanceID]*IBCInstance
	existingClients map[string]*IBCClient
	preparedClients map[string]*solidityibc.PreparedClient
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
	error,
) {
	connections := make(map[ConnectionID]*Connection, len(spec.Connections))
	attestorUse := make(map[EVMAddress]string)
	dependencies := connectionDependencies{instances: instances}
	if d.prepareConnections != nil {
		prepared, err := d.prepareConnections(ctx, spec, dependencies, runtime)
		if err != nil {
			return nil, fmt.Errorf("prepare IBC Connections failed: %w", err)
		}
		dependencies = prepared
		if err := validatePreparedAttestorUse(spec, dependencies, runtime); err != nil {
			return nil, err
		}
	}

	for _, declaration := range spec.Connections {
		connection, err := d.acquireConnection(ctx, declaration, dependencies, runtime)
		if err != nil {
			return nil, fmt.Errorf("start IBC Connection %q failed: %w", declaration.ID, err)
		}
		if connection == nil || connection.a == nil || connection.b == nil {
			return nil, fmt.Errorf(
				"environment: IBC Connection %q adapter returned an incomplete resolved value",
				declaration.ID,
			)
		}
		if err := recordResolvedAttestorUse(attestorUse, connection.a, connection.b); err != nil {
			return nil, err
		}
		connections[declaration.ID] = connection
	}
	return connections, nil
}

func validatePreparedAttestorUse(spec Spec, dependencies connectionDependencies, runtime Runtime) error {
	seen := make(map[EVMAddress]string)
	for _, connection := range spec.Connections {
		for _, end := range connection.ends() {
			label := clientLabel(connection.ID, end.label)
			client := dependencies.existingClients[label]
			if client == nil {
				client = &IBCClient{label: label}
				for _, attestor := range end.declaration.clientAttestors() {
					account, _ := runtime.evmAccount(attestor.Authority)
					client.attestors = append(client.attestors, EVMAddress(account.Address().Hex()))
				}
			}
			if err := recordResolvedAttestorUse(seen, client); err != nil {
				return err
			}
		}
	}
	return nil
}

func recordResolvedAttestorUse(seen map[EVMAddress]string, clients ...*IBCClient) error {
	for _, client := range clients {
		for _, address := range client.attestors {
			if previous, exists := seen[address]; exists {
				return fmt.Errorf(
					"attestor signer %s is reused by resolved IBC Clients %q and %q",
					address,
					previous,
					client.label,
				)
			}
			seen[address] = client.label
		}
	}
	return nil
}

func acquireAttestors(
	ctx context.Context,
	spec Spec,
	connections map[ConnectionID]*Connection,
	runtime Runtime,
	ws workspace,
	d drivers,
	effects *effectJournal,
) (map[AttestorID]*Attestor, error) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	type attestorJob struct {
		declaration  AttestorSpec
		dependencies attestorDependencies
	}
	var jobs []attestorJob
	for _, declaration := range spec.Connections {
		connection := connections[declaration.ID]
		for _, attestor := range declaration.A.clientAttestors() {
			jobs = append(jobs, attestorJob{
				declaration: attestor,
				dependencies: attestorDependencies{
					client: connection.a, observed: connection.b.instance,
				},
			})
		}
		for _, attestor := range declaration.B.clientAttestors() {
			jobs = append(jobs, attestorJob{
				declaration: attestor,
				dependencies: attestorDependencies{
					client: connection.b, observed: connection.a.instance,
				},
			})
		}
	}

	acquired := make([]attestorAcquisition, len(jobs))
	errs := make([]error, len(jobs))
	var wg sync.WaitGroup
	for i, job := range jobs {
		wg.Go(func() {
			acquisition, err := d.acquireAttestor(ctx, job.declaration, job.dependencies, runtime, ws)
			if err != nil {
				if acquisition.release != nil {
					effects.append(cleanupEffect{
						description: acquisition.description,
						release:     acquisition.release,
					})
				}
				errs[i] = fmt.Errorf("start Attestor %q failed: %w", job.declaration.ID, err)
				cancel()
				return
			}
			if acquisition.attestor == nil || acquisition.release == nil {
				errs[i] = fmt.Errorf(
					"environment: Attestor %q adapter returned an incomplete acquisition",
					job.declaration.ID,
				)
				cancel()
				return
			}
			effects.append(cleanupEffect{
				description: acquisition.description,
				release:     acquisition.release,
			})
			acquired[i] = acquisition
		})
	}
	wg.Wait()

	if err := errors.Join(errs...); err != nil {
		return nil, err
	}
	attestors := make(map[AttestorID]*Attestor, len(acquired))
	for i, acquisition := range acquired {
		attestors[jobs[i].declaration.ID] = acquisition.attestor
	}
	return attestors, nil
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
		apps, deployErr := setup.DeployAppStack(ctx, authority, resolved)
		if deployErr != nil {
			return nil, deployErr
		}
		return &IBCInstance{
			id:            instance.ID,
			chain:         chain,
			locator:       IBCInstanceLocator(resolved.Router.Hex()),
			accessManager: EVMAddress(resolved.AccessManager.Hex()),
			ics20Transfer: EVMAddress(apps.ICS20Transfer.Hex()),
			ics27GMP:      EVMAddress(apps.ICS27GMP.Hex()),
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
	dependencies.existingClients = make(map[string]*IBCClient)
	dependencies.preparedClients = make(map[string]*solidityibc.PreparedClient)

	for _, connection := range spec.Connections {
		ends := connection.ends()
		clientIDs := map[string]string{
			"A": clientID(connection.ID, "A", connection.A),
			"B": clientID(connection.ID, "B", connection.B),
		}
		for index, end := range ends {
			counterpartyEnd := ends[1-index]
			label := clientLabel(connection.ID, end.label)
			instanceID := clientIBCInstance(end.declaration)
			switch client := end.declaration.(type) {
			case ExistingClient:
				resolved, err := acquireIBCClient(
					ctx,
					connection.ID,
					end.label,
					client,
					clientIDs,
					dependencies,
					runtime,
				)
				if err != nil {
					return dependencies, fmt.Errorf(
						"prepare existing IBC Client %q: %w",
						label,
						err,
					)
				}
				dependencies.existingClients[label] = resolved
			case NewClient:
				instance := dependencies.instances[instanceID]
				setup, err := solidityIBCSetup(ctx, instance.chain)
				if err != nil {
					return dependencies, err
				}
				authority, _ := runtime.evmAccount(client.Authority)
				counterparty := clientIBCInstance(counterpartyEnd.declaration)
				header, err := evmHeader(ctx, dependencies.instances[counterparty].chain)
				if err != nil {
					return dependencies, fmt.Errorf(
						"prepare IBC Client %q counterparty header: %w",
						label,
						err,
					)
				}
				attestorSpecs := end.declaration.clientAttestors()
				attestors := make([]common.Address, 0, len(attestorSpecs))
				for _, declaration := range attestorSpecs {
					account, _ := runtime.evmAccount(declaration.Authority)
					attestors = append(attestors, account.Address())
				}
				prepared, err := setup.PrepareClient(
					ctx,
					authority,
					common.HexToAddress(string(instance.locator)),
					solidityibc.AttestationClientConfig{
						ID:                    clientIDs[end.label],
						CounterpartyClientID:  clientIDs[counterpartyEnd.label],
						Attestors:             attestors,
						MinRequiredSignatures: client.MinRequiredSignatures,
						InitialHeight:         header.Number.Uint64(),
						InitialTimestamp:      header.Time,
					},
				)
				if err != nil {
					return dependencies, fmt.Errorf("prepare IBC Client %q: %w", label, err)
				}
				dependencies.preparedClients[label] = prepared
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
	clientIDs := map[string]string{
		"A": clientID(declaration.ID, "A", declaration.A),
		"B": clientID(declaration.ID, "B", declaration.B),
	}

	a, err := acquireIBCClient(
		ctx, declaration.ID, "A", declaration.A, clientIDs, dependencies, runtime,
	)
	if err != nil {
		return nil, err
	}
	b, err := acquireIBCClient(
		ctx, declaration.ID, "B", declaration.B, clientIDs, dependencies, runtime,
	)
	if err != nil {
		return nil, err
	}
	if a.counterpartyID != b.id || b.counterpartyID != a.id {
		return nil, fmt.Errorf("resolved IBC Clients are not reciprocal")
	}
	return &Connection{id: declaration.ID, a: a, b: b}, nil
}

func acquireIBCClient(
	ctx context.Context,
	connectionID ConnectionID,
	end string,
	declaration ClientSpec,
	clientIDs map[string]string,
	dependencies connectionDependencies,
	runtime Runtime,
) (*IBCClient, error) {
	instance := dependencies.instances[clientIBCInstance(declaration)]
	label := clientLabel(connectionID, end)
	counterpartyID := clientIDs[counterpartyEnd(end)]
	if resolved := dependencies.existingClients[label]; resolved != nil {
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
			client.ID,
			counterpartyID,
		)
		if err != nil {
			return nil, err
		}
		if attestorErr := requireDeclaredAttestors(
			label,
			resolved.Attestors,
			declaration.clientAttestors(),
			runtime,
		); attestorErr != nil {
			return nil, attestorErr
		}
	case NewClient:
		prepared := dependencies.preparedClients[label]
		if prepared == nil {
			return nil, fmt.Errorf("IBC Client %q was not prepared", label)
		}
		authority, err := runtime.evmAccount(client.Authority)
		if err != nil {
			return nil, err
		}
		if fundingErr := ensureProtocolAuthorityFunded(ctx, instance.chain, authority); fundingErr != nil {
			return nil, fmt.Errorf("fund IBC Client %q authority: %w", label, fundingErr)
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
		label:                 label,
		instance:              instance,
		id:                    resolved.ID,
		lightClient:           EVMAddress(resolved.Address.Hex()),
		counterpartyID:        resolved.CounterpartyClientID,
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
	binding, _ := runtime.authority(declaration.Authority)
	launch := ibclink.AttestorLaunch{
		BinaryPath:        ibclink.ResolvedBin(),
		WorkDir:           filepath.Join(ws.privateDir, "attestor-"+resourcePathToken(string(declaration.ID))),
		Name:              string(declaration.ID),
		ChainID:           strconv.FormatUint(dependencies.observed.chain.evmChainID, 10),
		SignerGRPC:        binding.SignerGRPC,
		SignerRemoteKeyID: binding.SignerRemoteKeyID,
		RPCURL:            dependencies.observed.chain.rpcURL,
		ICS26Router:       string(dependencies.observed.locator),
	}
	if binding.SignerGRPC == "" {
		launch.PrivateKeyHex = binding.PrivateKeyHex
	}
	process, err := ibclink.StartAttestor(ctx, launch)
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
	attestor := &Attestor{
		id:       declaration.ID,
		client:   dependencies.client,
		observed: dependencies.observed,
		signer:   EVMAddress(process.SignerAddress().Hex()),
		endpoint: process.Endpoint(),
		process:  process,
		launch:   launch,
	}
	return attestorAcquisition{
		attestor:    attestor,
		description: fmt.Sprintf("stop Attestor %q", declaration.ID),
		// Cleanup runs after the lease is closed, so it takes the unleased
		// stop; stopping through the Attestor rather than the initial process
		// keeps a restarted process the one released at Close.
		release: attestor.stopProcess,
	}, nil
}

func solidityIBCSetup(ctx context.Context, chain *Chain) (*solidityibc.Setup, error) {
	if chain == nil {
		return nil, fmt.Errorf("missing resolved Chain")
	}
	var setup *solidityibc.Setup
	ok, err := evm.WithChainClient(chain.impl, func(client *evm.EVMClient) error {
		var setupErr error
		setup, setupErr = solidityibc.NewSetup(ctx, client.Client(), chain.timing.CompletionBudget)
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

func clientID(connectionID ConnectionID, end string, declaration ClientSpec) string {
	if existing, ok := declaration.(ExistingClient); ok {
		return existing.ID
	}
	hash := sha256.Sum256([]byte("environment-ibc-client-v1\x00" + string(connectionID) + "\x00" + end))
	return "link-" + hex.EncodeToString(hash[:])
}

func requireDeclaredAttestors(
	label string,
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
				label,
			)
		}
	}
	return nil
}

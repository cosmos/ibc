package environment

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum/common"

	"github.com/cosmos/ibc/e2e/internal/harness/chain/evm"
	"github.com/cosmos/ibc/e2e/internal/harness/chain/evm/anvil"
	"github.com/cosmos/ibc/e2e/internal/harness/chain/evm/besu"
	"github.com/cosmos/ibc/e2e/internal/harness/ibclink"
)

const (
	besuBlockPeriod = 2 * time.Second
)

// Start realizes every supported declaration and attempts to stop acquired local
// resources before returning an error. On-chain transactions mined before a
// later failure remain on their host chains.
func Start(ctx context.Context, spec Spec, runtime Runtime) (*Environment, error) {
	return start(ctx, spec, runtime, productionDrivers())
}

type drivers struct {
	validatePrerequisites func(Spec, Runtime) error
	acquireChain          func(context.Context, ChainSpec, Runtime, workspace) (chainAcquisition, error)
	acquireIBCInstance    func(context.Context, IBCInstanceSpec, *Chain, Runtime) (*IBCInstance, error)
	prepareConnections    func(context.Context, Spec, connectionDependencies, Runtime) (connectionDependencies, error)
	acquireConnection     func(context.Context, ConnectionSpec, connectionDependencies, Runtime) (*Connection, error)
	acquireAttestor       func(context.Context, AttestorSpec, attestorDependencies, Runtime, workspace) (attestorAcquisition, error)
}

type chainAcquisition struct {
	chain       *Chain
	description string
	release     func(context.Context) error
}

type attestorAcquisition struct {
	attestor    *Attestor
	description string
	release     func(context.Context) error
}

func start(ctx context.Context, spec Spec, runtime Runtime, d drivers) (*Environment, error) {
	spec = spec.snapshot()
	runtime = runtime.snapshot()
	if err := spec.validate(); err != nil {
		return nil, err
	}
	if err := validateRuntime(spec, runtime); err != nil {
		return nil, err
	}
	if d.validatePrerequisites != nil {
		if err := d.validatePrerequisites(spec, runtime); err != nil {
			return nil, err
		}
	}

	ws, err := newWorkspace()
	if err != nil {
		return nil, err
	}
	effects := &effectJournal{}

	chains, startErr := acquireChains(ctx, spec.Chains, runtime, ws, d, effects)
	if startErr != nil {
		return nil, abortStart(ctx, startErr, ws, effects)
	}
	lease := &environmentLease{}
	for _, chain := range chains {
		chain.bindLease(lease)
	}

	instances, startErr := acquireIBCInstances(ctx, spec.IBCInstances, chains, runtime, d)
	if startErr != nil {
		return nil, abortStart(ctx, startErr, ws, effects)
	}

	connections, clients, startErr := acquireConnections(ctx, spec, instances, runtime, d)
	if startErr != nil {
		return nil, abortStart(ctx, startErr, ws, effects)
	}

	attestors, startErr := acquireAttestors(ctx, spec, instances, clients, runtime, ws, d, effects)
	if startErr != nil {
		return nil, abortStart(ctx, startErr, ws, effects)
	}
	for _, attestor := range attestors {
		attestor.bindLease(lease)
	}

	return &Environment{
		chains:      chains,
		instances:   instances,
		connections: connections,
		clients:     clients,
		attestors:   attestors,
		effects:     effects,
		ws:          ws,
		lease:       lease,
	}, nil
}

func abortStart(
	ctx context.Context,
	cause error,
	ws workspace,
	effects *effectJournal,
) error {
	// Adapters provide bounded Stop operations. Let them complete after startup
	// cancellation. Runtime-private files are
	// always removed; only the separately rooted diagnostics directory may be
	// retained when cleanup fails.
	cleanupErrs := effects.cleanup(context.WithoutCancel(ctx))
	if removeErr := ws.removePrivate(); removeErr != nil {
		cleanupErrs = append(
			cleanupErrs,
			fmt.Errorf("environment cleanup private workspace removal failed: %w", removeErr),
		)
	}
	if len(cleanupErrs) == 0 {
		if removeErr := ws.removeDiagnostics(); removeErr != nil {
			cleanupErrs = append(
				cleanupErrs,
				fmt.Errorf("environment cleanup diagnostics removal failed: %w", removeErr),
			)
		}
	}
	diagnosticsDir := ""
	if len(cleanupErrs) != 0 {
		diagnosticsDir = ws.diagnosticsDir
	}
	return newStartError(cause, diagnosticsDir, cleanupErrs...)
}

func validateRuntime(spec Spec, runtime Runtime) error {
	requiredAuthorities := make(map[AuthorityID]struct{})
	for _, declaration := range spec.Chains {
		switch chain := declaration.(type) {
		case ManagedAnvil:
		case ManagedBesu:
		case AttachedEVM:
			endpoint, ok := runtime.endpoint(chain.Endpoint)
			if !ok || endpoint.RPCURL == "" {
				return fmt.Errorf("environment: no runtime endpoint binding for %q", chain.Endpoint)
			}
		default:
			return fmt.Errorf("environment: unsupported Chain declaration %T", declaration)
		}
	}
	for _, declaration := range spec.IBCInstances {
		switch instance := declaration.(type) {
		case NewIBCInstance:
			requiredAuthorities[instance.Authority] = struct{}{}
		case ExistingIBCInstance:
			if !common.IsHexAddress(string(instance.Locator)) {
				return fmt.Errorf("environment: IBC Instance locator %q is not an EVM address", instance.Locator)
			}
		}
	}
	for _, connection := range spec.Connections {
		for _, declaration := range []ClientSpec{connection.A, connection.B} {
			switch client := declaration.(type) {
			case NewClient:
				requiredAuthorities[client.Authority] = struct{}{}
			case DummyClient:
				requiredAuthorities[client.Authority] = struct{}{}
			}
		}
	}
	for _, attestor := range spec.Attestors {
		requiredAuthorities[attestor.Authority] = struct{}{}
	}
	for authority := range requiredAuthorities {
		if _, err := runtime.evmAccount(authority); err != nil {
			return err
		}
	}

	newInstances := make(map[IBCInstanceID]NewIBCInstance)
	for _, declaration := range spec.IBCInstances {
		if instance, ok := declaration.(NewIBCInstance); ok {
			newInstances[instance.ID] = instance
		}
	}
	for _, connection := range spec.Connections {
		for _, declaration := range []ClientSpec{connection.A, connection.B} {
			var (
				clientID    ClientID
				authorityID AuthorityID
				instanceID  IBCInstanceID
			)
			switch client := declaration.(type) {
			case NewClient:
				clientID, authorityID, instanceID = client.ID, client.Authority, client.IBCInstance
			case DummyClient:
				clientID, authorityID, instanceID = client.ID, client.Authority, client.IBCInstance
			default:
				continue
			}
			instance, isNew := newInstances[instanceID]
			if !isNew {
				continue
			}
			instanceAuthority, _ := runtime.evmAccount(instance.Authority)
			clientAuthority, _ := runtime.evmAccount(authorityID)
			if instanceAuthority.Address() != clientAuthority.Address() {
				return fmt.Errorf(
					"environment: new IBC Client %q authority must resolve to the new IBC Instance %q admin address",
					clientID,
					instance.ID,
				)
			}
		}
	}

	type attestorUse struct {
		id     AttestorID
		client ClientID
	}
	// Solidity IBC v3 attestations do not yet include domain separation. Reusing one
	// signer across Clients would allow the same signed bytes to be replayed in
	// another Client context, so signer isolation is a graph-wide invariant.
	attestorAddresses := make(map[string]attestorUse, len(spec.Attestors))
	for _, attestor := range spec.Attestors {
		account, _ := runtime.evmAccount(attestor.Authority)
		address := account.Address().Hex()
		if previous, exists := attestorAddresses[address]; exists {
			return fmt.Errorf(
				"environment: Attestors %q for IBC Client %q and %q for IBC Client %q resolve to the same signer address",
				previous.id,
				previous.client,
				attestor.ID,
				attestor.Client,
			)
		}
		attestorAddresses[address] = attestorUse{id: attestor.ID, client: attestor.Client}
	}
	return nil
}

func acquireChains(
	ctx context.Context,
	declarations []ChainSpec,
	runtime Runtime,
	ws workspace,
	d drivers,
	effects *effectJournal,
) (map[ChainID]*Chain, error) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	acquired := make([]chainAcquisition, len(declarations))
	errs := make([]error, len(declarations))
	var wg sync.WaitGroup
	for i, declaration := range declarations {
		wg.Add(1)
		go func() {
			defer wg.Done()
			acquisition, err := d.acquireChain(ctx, declaration, runtime, ws)
			if err != nil {
				errs[i] = fmt.Errorf("start Chain %q failed: %w", declaration.chainID(), err)
				cancel()
				return
			}

			effects.append(cleanupEffect{
				description: acquisition.description,
				release:     acquisition.release,
			})
			acquired[i] = acquisition
		}()
	}
	wg.Wait()

	if err := errors.Join(errs...); err != nil {
		return nil, err
	}
	chains := make(map[ChainID]*Chain, len(acquired))
	for _, acquisition := range acquired {
		chains[acquisition.chain.ID()] = acquisition.chain
	}
	return chains, nil
}

func productionDrivers() drivers {
	return drivers{
		validatePrerequisites: validateProductionPrerequisites,
		acquireChain:          acquireChain,
		acquireIBCInstance:    acquireIBCInstance,
		prepareConnections:    prepareConnections,
		acquireConnection:     acquireConnection,
		acquireAttestor:       acquireAttestor,
	}
}

func validateProductionPrerequisites(spec Spec, _ Runtime) error {
	if len(spec.Attestors) == 0 {
		return nil
	}
	path, err := filepath.Abs(ibclink.ResolvedBin())
	if err != nil {
		return fmt.Errorf("environment: resolve IBC Link binary prerequisite: %w", err)
	}
	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("environment: IBC Link binary prerequisite %q: %w", path, err)
	}
	if !info.Mode().IsRegular() || info.Mode()&0o111 == 0 {
		return fmt.Errorf("environment: IBC Link binary prerequisite %q is not an executable file", path)
	}
	return nil
}

func acquireChain(
	ctx context.Context,
	declaration ChainSpec,
	runtime Runtime,
	ws workspace,
) (chainAcquisition, error) {
	switch spec := declaration.(type) {
	case ManagedAnvil:
		return acquireAnvil(ctx, spec, ws)
	case ManagedBesu:
		return acquireBesu(ctx, spec, ws)
	case AttachedEVM:
		return attachEVM(ctx, spec, runtime)
	default:
		return chainAcquisition{}, fmt.Errorf("unsupported Chain declaration %T", declaration)
	}
}

func acquireAnvil(ctx context.Context, spec ManagedAnvil, ws workspace) (chainAcquisition, error) {
	adapter, err := anvil.Start(ctx, anvil.Spec{
		ID:        string(spec.ID),
		ChainID:   spec.EVMChainID,
		LogPath:   filepath.Join(ws.diagnosticsDir, "anvil-"+resourcePathToken(string(spec.ID))+".log"),
		RunID:     ws.runID,
		BlockTime: spec.BlockInterval,
	})
	if err != nil {
		return chainAcquisition{}, err
	}

	timing := instantTiming()
	if spec.BlockInterval > 0 {
		timing = blockTiming(spec.BlockInterval)
	}
	resolved := &Chain{
		id:         spec.ID,
		evmChainID: spec.EVMChainID,
		rpcURL:     adapter.RPCURL(),
		timing:     timing,
		impl:       adapter,
		node:       &NodeLifecycle{controller: adapter},
		funding:    &Funding{controller: adapter},
	}
	if spec.BlockInterval == 0 {
		resolved.mining = &Mining{controller: adapter}
	}
	return chainAcquisition{
		chain:       resolved,
		description: fmt.Sprintf("stop Chain %q", spec.ID),
		release:     func(context.Context) error { return adapter.Stop() },
	}, nil
}

func acquireBesu(ctx context.Context, spec ManagedBesu, ws workspace) (chainAcquisition, error) {
	adapter, err := besu.StartQBFT(ctx, besu.Spec{
		ID:          string(spec.ID),
		ChainID:     spec.EVMChainID,
		WorkDir:     filepath.Join(ws.privateDir, "besu-"+resourcePathToken(string(spec.ID))),
		RunID:       ws.runID,
		BlockPeriod: besuBlockPeriod,
	})
	if err != nil {
		return chainAcquisition{}, err
	}
	return chainAcquisition{
		chain: &Chain{
			id:         spec.ID,
			evmChainID: spec.EVMChainID,
			rpcURL:     adapter.RPCURL(),
			timing:     blockTiming(besuBlockPeriod),
			impl:       adapter,
			funding:    &Funding{controller: adapter},
		},
		description: fmt.Sprintf("stop Chain %q", spec.ID),
		release:     func(context.Context) error { return adapter.Stop() },
	}, nil
}

func attachEVM(ctx context.Context, spec AttachedEVM, runtime Runtime) (chainAcquisition, error) {
	endpoint, _ := runtime.endpoint(spec.Endpoint)
	adapter, err := evm.ConnectAttached(ctx, evm.AttachedSpec{
		ID:      string(spec.ID),
		ChainID: spec.EVMChainID,
		RPCURL:  endpoint.RPCURL,
	})
	if err != nil {
		return chainAcquisition{}, err
	}
	return chainAcquisition{
		chain: &Chain{
			id:         spec.ID,
			evmChainID: spec.EVMChainID,
			rpcURL:     adapter.RPCURL(),
			timing:     spec.Timing,
			impl:       adapter,
		},
		description: fmt.Sprintf("close local handle for Chain %q", spec.ID),
		release: func(context.Context) error {
			adapter.Close()
			return nil
		},
	}, nil
}

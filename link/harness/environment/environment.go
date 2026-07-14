package environment

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"sync"
)

// Environment is returned only after every supported declaration is ready. It
// is the sole owner of acquired local handles and environment-owned resources.
type Environment struct {
	chains      map[ChainID]*Chain
	instances   map[IBCInstanceID]*IBCInstance
	connections map[ConnectionID]*Connection
	clients     map[ClientID]*IBCClient
	attestors   map[AttestorID]*Attestor

	journal *journal
	effects *effectJournal
	ws      workspace

	lease *environmentLease

	closeMu sync.Mutex
	closed  bool
}

func (e *Environment) Chain(id ChainID) (*Chain, error) {
	chain, ok := e.chains[id]
	if !ok {
		return nil, fmt.Errorf("environment: no Chain %q", id)
	}
	return chain, nil
}

// Chains returns the resolved Chain identities in stable order. The returned
// slice is owned by the caller.
func (e *Environment) Chains() []ChainID {
	ids := make([]ChainID, 0, len(e.chains))
	for id := range e.chains {
		ids = append(ids, id)
	}
	slices.Sort(ids)
	return ids
}

func (e *Environment) IBCInstance(id IBCInstanceID) (*IBCInstance, error) {
	instance, ok := e.instances[id]
	if !ok {
		return nil, fmt.Errorf("environment: no IBC Instance %q", id)
	}
	return instance, nil
}

func (e *Environment) Connection(id ConnectionID) (*Connection, error) {
	connection, ok := e.connections[id]
	if !ok {
		return nil, fmt.Errorf("environment: no IBC Connection %q", id)
	}
	return connection, nil
}

func (e *Environment) IBCClient(id ClientID) (*IBCClient, error) {
	client, ok := e.clients[id]
	if !ok {
		return nil, fmt.Errorf("environment: no IBC Client %q", id)
	}
	return client, nil
}

func (e *Environment) Attestor(id AttestorID) (*Attestor, error) {
	attestor, ok := e.attestors[id]
	if !ok {
		return nil, fmt.Errorf("environment: no Attestor %q", id)
	}
	return attestor, nil
}

func (e *Environment) Manifest() Manifest {
	return e.journal.snapshot()
}

// Close releases acquired effects in reverse acquisition order, then removes
// the private workspace. Successful effects are never repeated. If cleanup
// fails or ctx expires, a later call retries only unfinished effects.
func (e *Environment) Close(ctx context.Context) error {
	e.closeMu.Lock()
	defer e.closeMu.Unlock()
	if e.closed {
		return nil
	}
	if e.lease != nil {
		if err := e.lease.close(ctx); err != nil {
			return fmt.Errorf("environment: wait for active operations before cleanup: %w", err)
		}
	}

	cleanupErrs := e.effects.cleanup(ctx, e.journal)
	if len(cleanupErrs) != 0 {
		return errors.Join(cleanupErrs...)
	}
	if err := e.ws.remove(); err != nil {
		return fmt.Errorf("environment cleanup workspace removal failed: %w", err)
	}
	e.closed = true
	return nil
}

type cleanupEffect struct {
	key       resourceKey
	ownership Ownership
	action    CleanupAction
	release   func(context.Context) error
	done      bool
}

// effectJournal tracks actual acquired effects, not desired declarations. An
// attached Chain therefore records a local client-close effect while its
// logical resource remains borrowed.
type effectJournal struct {
	mu      sync.Mutex
	effects []cleanupEffect
}

func (j *effectJournal) append(effect cleanupEffect) {
	j.mu.Lock()
	defer j.mu.Unlock()
	j.effects = append(j.effects, effect)
}

func (j *effectJournal) cleanup(ctx context.Context, resources *journal) []error {
	j.mu.Lock()
	defer j.mu.Unlock()

	failures := make([]error, 0)
	hasEffect := make(map[resourceKey]struct{}, len(j.effects))
	for i := len(j.effects) - 1; i >= 0; i-- {
		effect := &j.effects[i]
		hasEffect[effect.key] = struct{}{}
		if effect.done {
			continue
		}
		err := effect.release(ctx)
		outcome := CleanupOutcomeSucceeded
		if err != nil {
			outcome = CleanupOutcomeFailed
			failures = append(failures, &cleanupFailure{
				kind: effect.key.kind, id: effect.key.id, action: effect.action, cause: err,
			})
		}
		resources.recordCleanup(effect.key.kind, effect.key.id, effect.action, outcome)
		if err == nil {
			effect.done = true
		}

		state := ResourceStateRetained
		if effect.ownership == OwnershipOwnedEphemeral {
			state = ResourceStateReleased
			if err != nil {
				state = ResourceStateReleaseFailed
			}
		}
		_ = resources.setResourceState(effect.key.kind, effect.key.id, state)
	}

	// Resources without direct cleanup effects still need a terminal manifest
	// disposition. Host-scoped state follows its specific managed host or hosts;
	// durable and borrowed state remains outside Environment cleanup.
	resourceSnapshot := resources.snapshot().Resources()
	chainStates := make(map[ChainID]ResourceState)
	for _, record := range resourceSnapshot {
		if record.Kind == ResourceKindChain {
			chainStates[ChainID(record.ID)] = record.State
		}
	}
	for _, record := range resourceSnapshot {
		switch record.Ownership {
		case OwnershipBorrowed, OwnershipOwnedDurable:
			if record.State == ResourceStateReady || record.State == ResourceStateAcquired {
				_ = resources.setResourceState(record.Kind, record.ID, ResourceStateRetained)
			}
		case OwnershipOwnedHostScoped:
			hosts := resources.resourceHosts(record.Kind, record.ID)
			allReleased := len(hosts) != 0
			for _, host := range hosts {
				if chainStates[host] != ResourceStateReleased {
					allReleased = false
					break
				}
			}
			if record.State == ResourceStateReady ||
				record.State == ResourceStateAcquired ||
				record.State == ResourceStateFailed ||
				record.State == ResourceStateReleaseFailed {
				state := ResourceStateReleaseFailed
				if allReleased {
					state = ResourceStateReleased
				}
				_ = resources.setResourceState(record.Kind, record.ID, state)
			}
		case OwnershipOwnedEphemeral:
			if _, ok := hasEffect[resourceKey{kind: record.Kind, id: record.ID}]; ok {
				continue
			}
			if record.State == ResourceStateReleased {
				continue
			}
			_ = resources.setResourceState(record.Kind, record.ID, ResourceStateReleaseFailed)
			failures = append(failures, &cleanupFailure{
				kind: record.Kind, id: record.ID, action: CleanupActionStop,
				cause: errors.New("owned ephemeral resource has no release effect"),
			})
		}
	}
	return failures
}

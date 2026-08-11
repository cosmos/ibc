// SPDX-License-Identifier: Apache-2.0

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

// IBCInstanceForChain returns the single IBC Instance hosted on chain. It
// errors when the Chain hosts zero or multiple Instances.
func (e *Environment) IBCInstanceForChain(id ChainID) (*IBCInstance, error) {
	var found *IBCInstance
	for _, instance := range e.instances {
		if instance.chain.id != id {
			continue
		}
		if found != nil {
			return nil, fmt.Errorf("environment: multiple IBC Instances on Chain %q", id)
		}
		found = instance
	}
	if found == nil {
		return nil, fmt.Errorf("environment: no IBC Instance on Chain %q", id)
	}
	return found, nil
}

func (e *Environment) Connection(id ConnectionID) (*Connection, error) {
	connection, ok := e.connections[id]
	if !ok {
		return nil, fmt.Errorf("environment: no IBC Connection %q", id)
	}
	return connection, nil
}

// Connections returns the resolved IBC Connection identities in stable order.
// The returned slice is owned by the caller.
func (e *Environment) Connections() []ConnectionID {
	ids := make([]ConnectionID, 0, len(e.connections))
	for id := range e.connections {
		ids = append(ids, id)
	}
	slices.Sort(ids)
	return ids
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

// Attestors returns the resolved Attestor identities in stable order. The
// returned slice is owned by the caller.
func (e *Environment) Attestors() []AttestorID {
	ids := make([]AttestorID, 0, len(e.attestors))
	for id := range e.attestors {
		ids = append(ids, id)
	}
	slices.Sort(ids)
	return ids
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
	if err := e.lease.close(ctx); err != nil {
		return fmt.Errorf("environment: wait for active operations before cleanup: %w", err)
	}

	cleanupErrs := e.effects.cleanup(ctx)
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
	description string
	release     func(context.Context) error
	done        bool
}

// effectJournal tracks acquired cleanup work in reverse-release order.
type effectJournal struct {
	mu      sync.Mutex
	effects []cleanupEffect
}

func (j *effectJournal) append(effect cleanupEffect) {
	j.mu.Lock()
	defer j.mu.Unlock()
	j.effects = append(j.effects, effect)
}

func (j *effectJournal) cleanup(ctx context.Context) []error {
	j.mu.Lock()
	defer j.mu.Unlock()

	failures := make([]error, 0)
	for i := len(j.effects) - 1; i >= 0; i-- {
		effect := &j.effects[i]
		if effect.done {
			continue
		}
		if err := effect.release(ctx); err != nil {
			failures = append(failures, fmt.Errorf("environment cleanup %s failed: %w", effect.description, err))
			continue
		}
		effect.done = true
	}
	return failures
}

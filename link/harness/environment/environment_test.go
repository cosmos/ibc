package environment

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestEnvironmentChainsReturnsStableCallerOwnedIDs(t *testing.T) {
	env := &Environment{chains: map[ChainID]*Chain{
		"charlie": {},
		"alpha":   {},
		"bravo":   {},
	}}

	ids := env.Chains()
	require.Equal(t, []ChainID{"alpha", "bravo", "charlie"}, ids)

	ids[0] = "changed-by-caller"
	require.Equal(t, []ChainID{"alpha", "bravo", "charlie"}, env.Chains())
}

func TestHostScopedResourcesFollowTheirSpecificManagedHosts(t *testing.T) {
	resources := newJournal()
	for _, chainID := range []ChainID{"chain-a", "chain-b"} {
		require.NoError(t, resources.recordAcquired(
			ResourceKindChain,
			string(chainID),
			OwnershipOwnedEphemeral,
		))
		require.NoError(t, resources.setResourceState(
			ResourceKindChain,
			string(chainID),
			ResourceStateReady,
		))
	}
	for _, resource := range []struct {
		kind  ResourceKind
		id    string
		hosts []ChainID
	}{
		{kind: ResourceKindIBCInstance, id: "instance-a", hosts: []ChainID{"chain-a"}},
		{kind: ResourceKindIBCInstance, id: "instance-b", hosts: []ChainID{"chain-b"}},
		{kind: ResourceKindIBCConnection, id: "connection-ab", hosts: []ChainID{"chain-a", "chain-b"}},
	} {
		require.NoError(t, resources.recordAcquired(
			resource.kind,
			resource.id,
			OwnershipOwnedHostScoped,
			resource.hosts...,
		))
		require.NoError(t, resources.setResourceState(resource.kind, resource.id, ResourceStateReady))
	}

	releaseBErr := errors.New("release chain B")
	releaseBAttempts := 0
	effects := &effectJournal{}
	effects.append(cleanupEffect{
		key:       resourceKey{kind: ResourceKindChain, id: "chain-a"},
		ownership: OwnershipOwnedEphemeral,
		action:    CleanupActionStop,
		release:   func(context.Context) error { return nil },
	})
	effects.append(cleanupEffect{
		key:       resourceKey{kind: ResourceKindChain, id: "chain-b"},
		ownership: OwnershipOwnedEphemeral,
		action:    CleanupActionStop,
		release: func(context.Context) error {
			releaseBAttempts++
			if releaseBAttempts == 1 {
				return releaseBErr
			}
			return nil
		},
	})

	require.ErrorIs(t, errors.Join(effects.cleanup(t.Context(), resources)...), releaseBErr)
	first := resources.snapshot().Resources()
	require.Equal(t, ResourceStateReleased, resourceByID(t, first, "instance-a").State)
	require.Equal(t, ResourceStateReleaseFailed, resourceByID(t, first, "instance-b").State)
	require.Equal(t, ResourceStateReleaseFailed, resourceByID(t, first, "connection-ab").State)

	require.Empty(t, effects.cleanup(t.Context(), resources))
	second := resources.snapshot().Resources()
	require.Equal(t, ResourceStateReleased, resourceByID(t, second, "instance-b").State)
	require.Equal(t, ResourceStateReleased, resourceByID(t, second, "connection-ab").State)
}

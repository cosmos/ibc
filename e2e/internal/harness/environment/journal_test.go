package environment

import (
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestJournalSnapshotsIncrementallyInDeterministicOrder(t *testing.T) {
	j := newJournal()

	var wg sync.WaitGroup
	for _, resource := range []struct {
		kind      ResourceKind
		id        string
		ownership Ownership
	}{
		{ResourceKindChain, "chain-b", OwnershipBorrowed},
		{ResourceKindChain, "chain-a", OwnershipOwnedEphemeral},
		{ResourceKindIBCInstance, "ibc-a", OwnershipOwnedDurable},
	} {
		wg.Add(1)
		go func() {
			defer wg.Done()
			require.NoError(t, j.recordAcquired(resource.kind, resource.id, resource.ownership))
			require.NoError(t, j.setResourceState(resource.kind, resource.id, ResourceStateReady))
		}()
	}
	wg.Wait()

	got := j.snapshot().Resources()
	require.Equal(t, []string{"chain-a", "chain-b", "ibc-a"}, []string{
		got[0].ID, got[1].ID, got[2].ID,
	})
	require.Equal(t, ResourceStateReady, got[0].State)

	require.NoError(t, j.setResourceState(ResourceKindChain, "chain-a", ResourceStateReleased))
	require.Equal(t, ResourceStateReleased, j.snapshot().Resources()[0].State)
	require.Error(t, j.recordAcquired(ResourceKindChain, "chain-a", OwnershipOwnedEphemeral))
	require.Error(t, j.setResourceState(ResourceKindChain, "missing", ResourceStateFailed))
}

func TestJournalKeepsCleanupEffectsSeparateFromOwnership(t *testing.T) {
	j := newJournal()
	require.NoError(t, j.recordAcquired(ResourceKindChain, "attached", OwnershipBorrowed))
	require.NoError(t, j.setResourceState(ResourceKindChain, "attached", ResourceStateRetained))
	j.recordCleanup(ResourceKindChain, "attached", CleanupActionCloseLocalHandle, CleanupOutcomeSucceeded)

	m := j.snapshot()
	require.Equal(t, OwnershipBorrowed, m.Resources()[0].Ownership)
	require.Equal(t, ResourceStateRetained, m.Resources()[0].State)
	require.Equal(t, []CleanupAction{CleanupActionCloseLocalHandle}, []CleanupAction{m.CleanupEffects()[0].Action})
}

package environment

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestManifestSnapshotIsImmutableAndMachineReadable(t *testing.T) {
	m := Manifest{
		resources: []ResourceRecord{{
			Kind: ResourceKindChain, ID: "chain-a", Ownership: OwnershipBorrowed, State: ResourceStateReady,
		}},
		cleanup: []CleanupRecord{
			{
				Kind:    ResourceKindChain,
				ID:      "chain-a",
				Action:  CleanupActionCloseLocalHandle,
				Outcome: CleanupOutcomeSucceeded,
			},
		},
	}

	resources := m.Resources()
	resources[0].ID = "mutated"
	require.Equal(t, "chain-a", m.Resources()[0].ID)

	data, err := json.Marshal(m)
	require.NoError(t, err)
	require.JSONEq(t, `{
		"resources":[{"kind":"chain","id":"chain-a","ownership":"borrowed","state":"ready"}],
		"cleanup":[{"kind":"chain","id":"chain-a","action":"close_local_handle","outcome":"succeeded"}]
	}`, string(data))

	var roundTrip Manifest
	require.NoError(t, json.Unmarshal(data, &roundTrip))
	require.Equal(t, m.Resources(), roundTrip.Resources())
	require.Equal(t, m.CleanupEffects(), roundTrip.CleanupEffects())
}

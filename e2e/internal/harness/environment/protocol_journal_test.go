package environment

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestProtocolReceiptJournalKeepsSortedIndependentPrefixes(t *testing.T) {
	journal := newProtocolReceiptJournal()
	journal.recordInstance(IBCInstanceReceipt{
		ID: "instance-b", Chain: "chain-b",
		AccessManager: &EVMTransactionEvidence{Hash: "0xb"},
	})
	journal.recordInstance(IBCInstanceReceipt{
		ID: "instance-a", Chain: "chain-a",
		RouterProxy: &EVMTransactionEvidence{Hash: "0xa"},
	})
	journal.recordConnectionEnd("connection-ab", "A", IBCClientReceipt{
		ID: "client-a", IBCInstance: "instance-a", Locator: "link-a",
	})
	journal.recordConnectionEnd("connection-ab", "B", IBCClientReceipt{
		ID: "client-b", IBCInstance: "instance-b", Locator: "link-b",
	})

	snapshot := journal.snapshot()
	require.Equal(t, []IBCInstanceID{"instance-a", "instance-b"}, []IBCInstanceID{
		snapshot.instances[0].ID,
		snapshot.instances[1].ID,
	})
	require.Equal(t, ClientID("client-a"), snapshot.connections[0].A.ID)
	require.Equal(t, ClientID("client-b"), snapshot.connections[0].B.ID)

	snapshot.instances[1].AccessManager.Hash = "mutated"
	snapshot.connections[0].A.ID = "mutated"
	again := journal.snapshot()
	require.Equal(t, "0xb", again.instances[1].AccessManager.Hash)
	require.Equal(t, ClientID("client-a"), again.connections[0].A.ID)
}

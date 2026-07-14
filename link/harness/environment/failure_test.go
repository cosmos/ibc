package environment

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestStartErrorPreservesCauseCleanupAndManifest(t *testing.T) {
	primary := errors.New("primary failure detail")
	cleanupA := errors.New("cleanup-a detail")
	cleanupB := errors.New("cleanup-b")
	m := Manifest{resources: []ResourceRecord{{
		Kind: ResourceKindIBCInstance, ID: "ibc-a", Ownership: OwnershipOwnedDurable, State: ResourceStateRetained,
	}}}

	err := newStartErrorWithProtocol(
		primary, m, nil, "/tmp/diagnostics", protocolReceiptSnapshot{}, cleanupA, nil, cleanupB,
	)
	require.ErrorIs(t, err, primary)
	require.ErrorIs(t, err.CleanupError(), cleanupA)
	require.ErrorIs(t, err.CleanupError(), cleanupB)
	require.Contains(t, err.Error(), primary.Error())
	require.Contains(t, err.Error(), cleanupA.Error())
	require.Contains(t, err.Error(), cleanupB.Error())
	require.Equal(t, "/tmp/diagnostics", err.DiagnosticsDir())

	snapshot := err.Manifest()
	require.Equal(t, "ibc-a", snapshot.Resources()[0].ID)
	snapshot.resources[0].ID = "mutated"
	require.Equal(t, "ibc-a", err.Manifest().Resources()[0].ID)
}

func TestCleanupFailurePreservesAndIncludesCause(t *testing.T) {
	cause := errors.New("release detail")
	cleanup := &cleanupFailure{
		kind: ResourceKindChain, id: "chain-a", action: CleanupActionStop,
		cause: cause,
	}

	require.ErrorIs(t, cleanup, cause)
	require.Contains(t, cleanup.Error(), cause.Error())
}

func TestStartErrorWithoutCleanup(t *testing.T) {
	primary := errors.New("boom")
	err := newStartErrorWithProtocol(primary, Manifest{}, nil, "", protocolReceiptSnapshot{})
	require.EqualError(t, err, "environment start failed: boom")
	require.NoError(t, err.CleanupError())
	require.Panics(t, func() {
		_ = newStartErrorWithProtocol(nil, Manifest{}, nil, "", protocolReceiptSnapshot{})
	})
}

func TestStartErrorReportsFailureLocationAndCause(t *testing.T) {
	primary := errors.New("endpoint failure detail")
	err := newStartErrorWithProtocol(primary, Manifest{}, []FailureRecord{{
		Kind: ResourceKindChain, ID: "chain-b",
	}}, "", protocolReceiptSnapshot{})
	require.EqualError(t, err, `environment start failed: start chain "chain-b": endpoint failure detail`)
	require.Equal(t, []FailureRecord{{
		Kind: ResourceKindChain, ID: "chain-b",
	}}, err.Failures())
}

func TestStartErrorProtocolReceiptsAreTypedAndDefensive(t *testing.T) {
	instance := IBCInstanceReceipt{
		ID: "instance-a", Chain: "chain-a",
		AccessManager: &EVMTransactionEvidence{Hash: "0x1", ContractAddress: "0xabc"},
	}
	connection := IBCConnectionReceipt{
		ID: "connection-ab",
		A: &IBCClientReceipt{
			ID: "client-a", IBCInstance: "instance-a", Locator: "link-a",
			LightClientDeployment: &EVMTransactionEvidence{Hash: "0x2", ContractAddress: "0xdef"},
		},
	}
	err := newStartErrorWithProtocol(
		errors.New("failed"),
		Manifest{},
		nil,
		"",
		protocolReceiptSnapshot{
			instances:   []IBCInstanceReceipt{instance},
			connections: []IBCConnectionReceipt{connection},
		},
	)

	instances := err.IBCInstanceReceipts()
	connections := err.IBCConnectionReceipts()
	require.Equal(t, instance, instances[0])
	require.Equal(t, connection, connections[0])

	instances[0].AccessManager.Hash = "mutated"
	connections[0].A.LightClientDeployment.Hash = "mutated"
	require.Equal(t, "0x1", err.IBCInstanceReceipts()[0].AccessManager.Hash)
	require.Equal(t, "0x2", err.IBCConnectionReceipts()[0].A.LightClientDeployment.Hash)
}

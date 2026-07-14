package environment

import (
	"encoding/json"
	"slices"
)

// ResourceKind identifies a domain resource without turning the Environment
// into a generic resource registry.
type ResourceKind string

const (
	ResourceKindChain         ResourceKind = "chain"
	ResourceKindIBCInstance   ResourceKind = "ibc_instance"
	ResourceKindIBCClient     ResourceKind = "ibc_client"
	ResourceKindIBCConnection ResourceKind = "ibc_connection"
	ResourceKindAttestor      ResourceKind = "attestor"
)

type Ownership string

const (
	OwnershipBorrowed       Ownership = "borrowed"
	OwnershipOwnedEphemeral Ownership = "owned_ephemeral"
	// OwnershipOwnedHostScoped identifies created state whose lifetime is
	// bounded by an environment-owned host, such as a contract on managed Anvil.
	OwnershipOwnedHostScoped Ownership = "owned_host_scoped"
	OwnershipOwnedDurable    Ownership = "owned_durable"
)

type ResourceState string

const (
	ResourceStateAcquired      ResourceState = "acquired"
	ResourceStateReady         ResourceState = "ready"
	ResourceStateFailed        ResourceState = "failed"
	ResourceStateReleased      ResourceState = "released"
	ResourceStateReleaseFailed ResourceState = "release_failed"
	ResourceStateRetained      ResourceState = "retained"
)

// ResourceRecord deliberately contains no arbitrary messages or metadata.
// Durable outputs belong on typed resolved resources; the failure manifest is
// safe to serialize without accidentally copying credentials or raw errors.
type ResourceRecord struct {
	Kind      ResourceKind  `json:"kind"`
	ID        string        `json:"id"`
	Ownership Ownership     `json:"ownership"`
	State     ResourceState `json:"state"`
}

type CleanupAction string

const (
	CleanupActionCloseLocalHandle CleanupAction = "close_local_handle"
	CleanupActionStop             CleanupAction = "stop"
)

type CleanupOutcome string

const (
	CleanupOutcomeSucceeded CleanupOutcome = "succeeded"
	CleanupOutcomeFailed    CleanupOutcome = "failed"
)

// CleanupRecord is separate from ResourceRecord because logical ownership and
// local cleanup are independent. A borrowed resource can be retained while a
// harness-owned client or other local handle is closed.
type CleanupRecord struct {
	Kind    ResourceKind   `json:"kind"`
	ID      string         `json:"id"`
	Action  CleanupAction  `json:"action"`
	Outcome CleanupOutcome `json:"outcome"`
}

// Manifest is an immutable snapshot. Accessors return copies, and its JSON
// representation contains only the redacted records above.
type Manifest struct {
	resources []ResourceRecord
	cleanup   []CleanupRecord
}

func (m Manifest) Resources() []ResourceRecord {
	return slices.Clone(m.resources)
}

func (m Manifest) CleanupEffects() []CleanupRecord {
	return slices.Clone(m.cleanup)
}

func (m Manifest) MarshalJSON() ([]byte, error) {
	type manifestJSON struct {
		Resources []ResourceRecord `json:"resources"`
		Cleanup   []CleanupRecord  `json:"cleanup"`
	}
	return json.Marshal(manifestJSON{
		Resources: m.Resources(),
		Cleanup:   m.CleanupEffects(),
	})
}

func (m *Manifest) UnmarshalJSON(data []byte) error {
	type manifestJSON struct {
		Resources []ResourceRecord `json:"resources"`
		Cleanup   []CleanupRecord  `json:"cleanup"`
	}
	var decoded manifestJSON
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}
	m.resources = slices.Clone(decoded.Resources)
	m.cleanup = slices.Clone(decoded.Cleanup)
	return nil
}

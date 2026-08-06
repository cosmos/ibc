// Package deploy is the target-agnostic deployment engine: manifest-driven,
// idempotent steps over a per-target Target implementation.
package deploy

import (
	"context"

	"github.com/cosmos/ibc/link/internal/deploy/manifest"
)

// ClientTypeAttestation is the only client type currently implemented.
const ClientTypeAttestation = "attestation"

// CoreParams parameterizes core-stack provisioning.
type CoreParams struct {
	ChainID string
}

// AttestationParams are the constructor inputs for an attestation client.
type AttestationParams struct {
	Attestors        []string
	Threshold        uint8
	InitialHeight    uint64
	InitialTimestamp uint64
}

// ClientSpec describes one light client to provision and register.
// Params carries type-specific parameters (AttestationParams for "attestation").
type ClientSpec struct {
	ClientID             string
	Type                 string
	CounterpartyChainID  string
	CounterpartyClientID string
	Params               any
}

// CoreRef is the result of provisioning the core stack.
type CoreRef struct {
	Router     string
	TargetData map[string]string
}

// ClientRef is the result of provisioning a light client contract.
type ClientRef struct {
	Address string
}

// Check statuses.
const (
	CheckOK     = "ok"
	CheckFailed = "failed"
)

// Check is one verification result.
type Check struct {
	Name   string `json:"name"`
	Status string `json:"status"`
	Detail string `json:"detail,omitempty"`
}

// Report is the outcome of verifying a manifest against live chain state.
type Report struct {
	Checks []Check `json:"checks"`
}

func (r Report) Failed() []Check {
	var failed []Check
	for _, c := range r.Checks {
		if c.Status != CheckOK {
			failed = append(failed, c)
		}
	}
	return failed
}

// Target deploys and wires IBC on one deployment-target ecosystem.
// Provision* verbs use the target's idiomatic tooling; the rest are IBC
// wiring and reads.
type Target interface {
	ProvisionCore(ctx context.Context, p CoreParams) (CoreRef, error)
	ProvisionClient(ctx context.Context, router string, spec ClientSpec) (ClientRef, error)
	// RegisterClient registers a provisioned client on the router and returns
	// the registered client ID.
	RegisterClient(ctx context.Context, router string, spec ClientSpec, client ClientRef) (string, error)
	// ClientRegistered reports whether clientID is registered on router,
	// returning the registered client address when it is.
	ClientRegistered(ctx context.Context, router, clientID string) (string, bool, error)
	// HasCode reports whether an on-chain artifact exists at address.
	HasCode(ctx context.Context, address string) (bool, error)
	// Head returns the chain's current height and timestamp (seconds).
	Head(ctx context.Context) (height, timestamp uint64, err error)
	Verify(ctx context.Context, m *manifest.Manifest) (Report, error)
	SupportedClientTypes() []string
}

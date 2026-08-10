// SPDX-License-Identifier: Apache-2.0

// Package deploy is the target-agnostic deployment engine: manifest-driven,
// idempotent steps over a per-target Target implementation.
package deploy

import (
	"context"

	"github.com/cosmos/ibc/link/internal/deploy/manifest"
)

// ClientTypeAttestation is the only client type currently implemented.
const ClientTypeAttestation = "attestation"

// GMPPortID is the fixed IBC port the ICS27-GMP app registers under
// (ICS27Lib.DEFAULT_PORT_ID). ICS27GMP.onRecvPacket requires this exact port,
// so the app is only reachable when registered here.
const GMPPortID = "gmpport"

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

// GMPRef is the result of provisioning the ICS27-GMP app.
type GMPRef struct {
	Address      string // proxy
	AccountLogic string // beacon logic impl
}

// IFTSpec describes one IFT token to deploy.
type IFTSpec struct {
	Owner  string
	Name   string
	Symbol string
}

// IFTRef is the result of provisioning an IFT token.
type IFTRef struct {
	Address string // proxy
}

// BridgeSpec describes one IFT bridge to register on a token.
type BridgeSpec struct {
	ClientID            string
	CounterpartyIFT     string
	SendCallConstructor string
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

// Failed returns the checks that did not pass.
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
	// ProvisionCore deploys the core routing stack.
	ProvisionCore(ctx context.Context, p CoreParams) (CoreRef, error)
	// ProvisionClient deploys a light client governed by router, without
	// registering it.
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
	// Verify checks a manifest's recorded deployment against live chain
	// state.
	Verify(ctx context.Context, m *manifest.Manifest) (Report, error)
	// SupportedClientTypes lists the client type names ProvisionClient
	// accepts in ClientSpec.Type.
	SupportedClientTypes() []string
	// ProvisionGMP deploys the ICS27-GMP app (account logic + impl + proxy).
	ProvisionGMP(ctx context.Context, router, accessManager string) (GMPRef, error)
	// RegisterApp registers app on the router under port.
	RegisterApp(ctx context.Context, router, app, port string) error
	// AppRegistered reports whether an app is registered at port, returning its
	// address when it is.
	AppRegistered(ctx context.Context, router, port string) (string, bool, error)
	// ProvisionIFT deploys an IFT token governed by the GMP app.
	ProvisionIFT(ctx context.Context, gmp string, spec IFTSpec) (IFTRef, error)
	// ProvisionSendCallConstructor deploys the stateless EVM IFT send-call
	// constructor and returns its address.
	ProvisionSendCallConstructor(ctx context.Context) (string, error)
	// RegisterIFTBridge registers a bridge on an IFT token.
	RegisterIFTBridge(ctx context.Context, ift string, spec BridgeSpec) error
	// IFTBridge reports whether a bridge for clientID exists on the token,
	// returning the counterparty IFT address when it does.
	IFTBridge(ctx context.Context, ift, clientID string) (string, bool, error)
}

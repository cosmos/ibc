// SPDX-License-Identifier: Apache-2.0

// Package deploy is the target-agnostic deployment engine: manifest-driven,
// idempotent steps over a per-target Target implementation.
package deploy

import (
	"context"
	"math/big"

	"github.com/ethereum/go-ethereum/common"

	"github.com/cosmos/ibc-contracts/packages/go-abigen/besumsgs"
	"github.com/cosmos/ibc/cli/besu"
	"github.com/cosmos/ibc/cli/internal/deploy/manifest"
)

// GMPPortID is the fixed IBC port the ICS27-GMP app registers under
// (ICS27Lib.DEFAULT_PORT_ID). ICS27GMP.onRecvPacket requires this exact port,
// so the app is only reachable when registered here.
const GMPPortID = "gmpport"

// CoreParams parameterizes core-stack provisioning.
type CoreParams struct {
	ChainID string
}

// ClientParams are the constructor inputs of one client type.
type ClientParams interface {
	clientParams()
}

// AttestationParams are the constructor inputs for an attestation client,
// recorded in the manifest as they are.
type AttestationParams manifest.AttestationParams

func (AttestationParams) clientParams() {}

// BesuParams are the constructor inputs for a Besu client verifying a chain
// that runs Consensus, which picks the contract. Periods are in seconds.
type BesuParams struct {
	Consensus             besu.Consensus
	IBCRouter             common.Address
	InitialHeight         uint64
	InitialConsensusState besumsgs.IBesuLightClientMsgsConsensusState
	TrustingPeriod        uint64
	MaxClockDrift         uint64
	TrustLevel            besumsgs.IBesuLightClientMsgsTrustThreshold
}

func (BesuParams) clientParams() {}

// BesuSource is implemented by targets whose chain runs Besu and can serve
// their sealed head, which a new client tracking them starts trusting.
type BesuSource interface {
	BesuHead(ctx context.Context) (*besu.ParsedHeader, error)
}

// ClientSpec describes one light client to provision and register.
type ClientSpec struct {
	ClientID             string
	CounterpartyChainID  string
	CounterpartyClientID string
	Params               ClientParams
}

// CoreRef is the result of provisioning the core stack.
type CoreRef struct {
	Router     string
	TargetData map[string]string
}

// MulticallRef is the deployed multicall proxy and its implementation.
type MulticallRef struct {
	Address        string
	Implementation string
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
	// RateLimit is set on the token after deployment; IFT rejects every
	// transfer until it has one. Nil leaves it to the owner, which must then
	// set it itself.
	RateLimit *IFTRateLimit
}

// IFTRateLimit is an IFT token's rate limit: each direction lets up to
// Capacity base units through when full, and refills completely over Window
// seconds.
type IFTRateLimit struct {
	Capacity *big.Int
	Window   uint64
}

// Equal reports whether l and o are the same rate limit.
func (l IFTRateLimit) Equal(o IFTRateLimit) bool {
	return l.Window == o.Window && l.Capacity.Cmp(o.Capacity) == 0
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
	// ProvisionMulticall deploys a multicall governed by the core's access manager.
	ProvisionMulticall(ctx context.Context, accessManager string) (MulticallRef, error)
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
	// ProvisionGMP deploys the ICS27-GMP app (account logic + impl + proxy).
	ProvisionGMP(ctx context.Context, router, accessManager string) (GMPRef, error)
	// RegisterApp registers app on the router under port.
	RegisterApp(ctx context.Context, router, app, port string) error
	// AppRegistered reports whether an app is registered at port, returning its
	// address when it is.
	AppRegistered(ctx context.Context, router, port string) (string, bool, error)
	// ProvisionIFT deploys an IFT token governed by the GMP app.
	ProvisionIFT(ctx context.Context, gmp string, spec IFTSpec) (IFTRef, error)
	// SetIFTRateLimit sets an IFT token's rate limit. Only the token owner
	// may.
	SetIFTRateLimit(ctx context.Context, ift string, limit IFTRateLimit) error
	// IFTRateLimit returns an IFT token's configured rate limit.
	IFTRateLimit(ctx context.Context, ift string) (IFTRateLimit, error)
	// ProvisionSendCallConstructor deploys the stateless EVM IFT send-call
	// constructor and returns its address.
	ProvisionSendCallConstructor(ctx context.Context) (string, error)
	// RegisterIFTBridge registers a bridge on an IFT token.
	RegisterIFTBridge(ctx context.Context, ift string, spec BridgeSpec) error
	// IFTBridge reports whether a bridge for clientID exists on the token,
	// returning its counterparty IFT address and send-call constructor when it
	// does.
	IFTBridge(ctx context.Context, ift, clientID string) (counterparty, constructor string, registered bool, err error)
}

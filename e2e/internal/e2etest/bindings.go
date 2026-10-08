// SPDX-License-Identifier: Apache-2.0

package e2etest

import (
	"context"
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	relayerv2 "github.com/cosmos/ibc/cli/api/v2/relayer"
	"github.com/cosmos/ibc/harness/environment"
	"github.com/cosmos/ibc/harness/ibccli"
	"github.com/cosmos/ibc/harness/traffic"
)

// This file adapts the harness traffic layer to testing.TB: each wrapper fails
// the test on error and registers the cleanup of whatever it started, so
// t.Cleanup's LIFO order stops processes before their directories are removed
// and before the Environment closes.

const (
	ChainA = traffic.ChainA
	ChainB = traffic.ChainB

	ProtocolAuthorityID = traffic.ProtocolAuthorityID
)

const relayerStopTimeout = traffic.RelayerStopTimeout

type (
	// RouteID names a directed route.
	RouteID = traffic.RouteID
	// Route is a directed route between two Chains.
	Route = traffic.Route
	// PacketTx locates one packet originated by a test application.
	PacketTx = traffic.PacketTx
	// Signer is an independent local identity.
	Signer = traffic.Signer
	// Deployment is the traffic-layer view of protocol apps and test tokens.
	Deployment = traffic.Deployment
	// ChainDeployment holds per-chain application contracts.
	ChainDeployment = traffic.ChainDeployment
	// RouteClients holds the client IDs and kinds for one directed route.
	RouteClients = traffic.RouteClients

	// Transfer drives ICS20 transfers on a single directed route.
	Transfer = traffic.Transfer
	// TransferRequest describes one ICS20 transfer.
	TransferRequest = traffic.TransferRequest
	// PreparedTransfer is an ICS20 transfer prepared for submission.
	PreparedTransfer = traffic.PreparedTransfer
	// TransferSend is the result of an ICS20 send.
	TransferSend = traffic.TransferSend

	// IFT drives the Interchain Fungible Token pair on a single directed route.
	IFT = traffic.IFT
	// IFTRequest describes one IFT transfer.
	IFTRequest = traffic.IFTRequest
	// IFTSend is the result of an IFT send.
	IFTSend = traffic.IFTSend
	// IFTBatch is the result of an IFT batch send.
	IFTBatch = traffic.IFTBatch

	// GMP drives ICS27 GMP calls on a single directed route.
	GMP = traffic.GMP
	// GMPRequest describes one GMP call.
	GMPRequest = traffic.GMPRequest
	// GMPCall is the result of a GMP call.
	GMPCall = traffic.GMPCall
)

func AtoB(a, b environment.ChainID) Route       { return traffic.AtoB(a, b) }
func BtoA(b, a environment.ChainID) Route       { return traffic.BtoA(b, a) }
func ManualAtoB(a, b environment.ChainID) Route { return traffic.ManualAtoB(a, b) }

// RequiredSignerBalance is the minimum balance external Chains must provision
// for each test actor before deployment.
func RequiredSignerBalance() *big.Int { return traffic.RequiredSignerBalance() }

// ProtocolAuthorityAddress is the deployer funded for IBC Instance realization.
func ProtocolAuthorityAddress() common.Address { return traffic.ProtocolAuthorityAddress() }

// RuntimeWithProtocolDeployer returns runtime with the protocol deployer authority.
func RuntimeWithProtocolDeployer(runtime environment.Runtime) environment.Runtime {
	return traffic.RuntimeWithProtocolDeployer(runtime)
}

// NewAddress generates a fresh, unfunded EVM address.
func NewAddress() (common.Address, error) { return traffic.NewAddress() }

// PackERC20Transfer packs an ERC20 transfer call.
func PackERC20Transfer(to common.Address, amount *big.Int) ([]byte, error) {
	return traffic.PackERC20Transfer(to, amount)
}

// AwaitState waits for the packet to reach want.
func AwaitState(
	ctx context.Context,
	relayer *ibccli.Relayer,
	packet PacketTx,
	want relayerv2.PacketState,
) (*relayerv2.PacketStatus, error) {
	return traffic.AwaitState(ctx, relayer, packet, want)
}

// AwaitStable requires the packet to remain in one state across its route's
// stability window.
func AwaitStable(
	ctx context.Context,
	relayer *ibccli.Relayer,
	packet PacketTx,
	want relayerv2.PacketState,
) error {
	return traffic.AwaitStable(ctx, relayer, packet, want)
}

// RelayAll submits every packet in the packet's source transaction.
func RelayAll(ctx context.Context, relayer *ibccli.Relayer, packet PacketTx) error {
	return traffic.RelayAll(ctx, relayer, packet)
}

// RelaySelected submits explicit packets from one source transaction.
func RelaySelected(ctx context.Context, relayer *ibccli.Relayer, packets ...PacketTx) error {
	return traffic.RelaySelected(ctx, relayer, packets...)
}

// NewSigner generates a randomly-keyed local identity.
func NewSigner(t testing.TB) Signer {
	t.Helper()
	signer, err := traffic.NewSigner()
	require.NoError(t, err)
	return signer
}

// NewSignerFromHex creates a deterministically keyed local identity.
func NewSignerFromHex(t testing.TB, privateKeyHex string) Signer {
	t.Helper()
	signer, err := traffic.NewSignerFromHex(privateKeyHex)
	require.NoError(t, err)
	return signer
}

// Deploy writes a temporary black-box configuration, runs its migration, and
// deploys the test applications on each Chain. It does not start the relayer.
func Deploy(
	t testing.TB,
	env *environment.Environment,
	deployer Signer,
	relayer Signer,
	routes ...Route,
) (*ibccli.Driver, *Deployment) {
	t.Helper()
	return DeployWithRelayerConfig(t, env, deployer, relayer, nil, routes...)
}

// DeployWithRelayerConfig is Deploy with a hook that adjusts the resolved
// relayer configuration after environment Attestors are assigned.
func DeployWithRelayerConfig(
	t testing.TB,
	env *environment.Environment,
	deployer Signer,
	relayer Signer,
	configure func(*ibccli.RelayerConfig),
	routes ...Route,
) (*ibccli.Driver, *Deployment) {
	t.Helper()
	driver, deployment, err := traffic.DeployWithRelayerConfig(
		t.Context(), env, t.TempDir(), deployer, relayer, configure, routes...,
	)
	require.NoError(t, err)
	return driver, deployment
}

// StartRelayer starts the test relayer and registers idempotent teardown.
func StartRelayer(t testing.TB, driver *ibccli.Driver, env *environment.Environment) *ibccli.Relayer {
	t.Helper()

	// opt-on dump for debugging
	dumpRelayer := func() {
		if environment.DumpEnabled() && driver != nil && env != nil {
			environment.DumpTestDirectory(t, env.RunID(), driver.ConfigHome())
		}
	}

	relayer, err := traffic.StartRelayer(t.Context(), driver, env)
	if err != nil {
		dumpRelayer()
		t.Fatalf("e2etest: %v", err)
	}

	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), relayerStopTimeout)
		defer cancel()
		assert.NoError(t, relayer.Stop(ctx), "e2etest: stop relayer")
		dumpRelayer()
	})
	return relayer
}

// StartProver writes the prover's config and runs it.
func StartProver(t testing.TB, env *environment.Environment, signer Signer) *ibccli.Prover {
	t.Helper()
	prover, err := traffic.StartProver(env, signer, t.TempDir())
	require.NoError(t, err)
	t.Cleanup(func() { assert.NoError(t, prover.Stop(), "e2etest: stop prover service") })
	return prover
}

// NewTransfer constructs the Transfer app for a route. The sender must be the
// signer that deployed the apps: it holds the minted token supply.
func NewTransfer(
	t testing.TB,
	env *environment.Environment,
	deployment *Deployment,
	sender Signer,
	route Route,
) *Transfer {
	t.Helper()
	app, err := traffic.NewTransfer(env, deployment, sender, route)
	require.NoError(t, err)
	return app
}

// NewIFT constructs the IFT app for a route. The sender must be the signer
// that deployed the apps: it holds the minted token supply.
func NewIFT(
	t testing.TB,
	env *environment.Environment,
	deployment *Deployment,
	sender Signer,
	route Route,
) *IFT {
	t.Helper()
	app, err := traffic.NewIFT(env, deployment, sender, route)
	require.NoError(t, err)
	return app
}

// DeployIFTTokenPair deploys, funds, and registers another IFT pair on an existing route.
func DeployIFTTokenPair(
	t testing.TB,
	env *environment.Environment,
	deployment *Deployment,
	deployer Signer,
	route Route,
) *IFT {
	t.Helper()
	app, err := traffic.DeployIFTTokenPair(t.Context(), env, deployment, deployer, route)
	require.NoError(t, err)
	return app
}

// NewGMP constructs the GMP app for a route. The sender must be the signer
// that deployed the apps: it holds the minted token supply.
func NewGMP(
	t testing.TB,
	env *environment.Environment,
	deployment *Deployment,
	sender Signer,
	route Route,
) *GMP {
	t.Helper()
	app, err := traffic.NewGMP(env, deployment, sender, route)
	require.NoError(t, err)
	return app
}

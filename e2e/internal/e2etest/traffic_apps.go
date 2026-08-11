// SPDX-License-Identifier: Apache-2.0

package e2etest

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/cosmos/ibc/e2e/internal/harness/environment"
	"github.com/cosmos/ibc/e2e/internal/harness/environment/solidityibc/counter"
)

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
	source, destination, sourceApps, destinationApps, clients := resolveDeploymentRoute(t, env, deployment, route)
	sourceEndpoint, destinationEndpoint, err := resolveRouteEndpoints(route.ID, source, destination)
	require.NoError(t, err, "e2etest: resolve endpoints for Transfer on route %q", route.ID)
	return &Transfer{
		routeID:        route.ID,
		source:         sourceEndpoint,
		destination:    destinationEndpoint,
		sender:         sender.account,
		sourceToken:    sourceApps.Token,
		sourceICS20:    sourceApps.ICS20Transfer,
		sourceRouter:   sourceApps.ICS26Router,
		destICS20:      destinationApps.ICS20Transfer,
		sourceClientID: clients.SourceClientID,
		destClientID:   clients.DestClientID,
	}
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
	source, destination, sourceApps, destinationApps, clients := resolveDeploymentRoute(t, env, deployment, route)
	sourceEndpoint, destinationEndpoint, err := resolveRouteEndpoints(route.ID, source, destination)
	require.NoError(t, err, "e2etest: resolve endpoints for IFT on route %q", route.ID)
	return &IFT{
		routeID:        route.ID,
		source:         sourceEndpoint,
		destination:    destinationEndpoint,
		sender:         sender.account,
		sourceIFT:      sourceApps.IFT,
		destIFT:        destinationApps.IFT,
		sourceRouter:   sourceApps.ICS26Router,
		sourceClientID: clients.SourceClientID,
		batcher:        sourceApps.IFTBatchShim,
	}
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
	source, destination, sourceApps, destinationApps, clients := resolveDeploymentRoute(t, env, deployment, route)
	sourceEndpoint, destinationEndpoint, err := resolveRouteEndpoints(route.ID, source, destination)
	require.NoError(t, err, "e2etest: resolve endpoints for additional IFT on route %q", route.ID)

	sourceIFT, err := deployIFTToken(
		t.Context(), sourceEndpoint.evm, deployer.account, sourceApps.ICS27GMP,
	)
	require.NoError(t, err, "e2etest: deploy additional IFT on Chain %q", route.Source)
	destinationIFT, err := deployIFTToken(
		t.Context(), destinationEndpoint.evm, deployer.account, destinationApps.ICS27GMP,
	)
	require.NoError(t, err, "e2etest: deploy additional IFT on Chain %q", route.Destination)
	batcher, err := deployIFTBatchShim(
		t.Context(), sourceEndpoint.evm, deployer.account, sourceIFT, initialTokenSupply,
	)
	require.NoError(t, err, "e2etest: deploy additional IFT batch transfer shim on Chain %q", route.Source)

	registerIFTBridge(
		t, sourceEndpoint.evm, deployer, route.Source,
		sourceIFT, clients.SourceClientID, destinationIFT, sourceApps.IFTSendCallConstructor,
	)
	if !route.SkipDestinationIFTBridge {
		registerIFTBridge(
			t, destinationEndpoint.evm, deployer, route.Destination,
			destinationIFT, clients.DestClientID, sourceIFT, destinationApps.IFTSendCallConstructor,
		)
	}

	return &IFT{
		routeID:        route.ID,
		source:         sourceEndpoint,
		destination:    destinationEndpoint,
		sender:         deployer.account,
		sourceIFT:      sourceIFT,
		destIFT:        destinationIFT,
		sourceRouter:   sourceApps.ICS26Router,
		sourceClientID: clients.SourceClientID,
		batcher:        batcher,
	}
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
	source, destination, sourceApps, destinationApps, clients := resolveDeploymentRoute(t, env, deployment, route)
	sourceEndpoint, destinationEndpoint, err := resolveRouteEndpoints(route.ID, source, destination)
	require.NoError(t, err, "e2etest: resolve endpoints for GMP on route %q", route.ID)
	defaultCall, err := mustABI(counter.CounterMetaData).Pack("increment")
	require.NoError(t, err, "e2etest: pack Counter.increment()")
	return &GMP{
		routeID:        route.ID,
		source:         sourceEndpoint,
		destination:    destinationEndpoint,
		sender:         sender.account,
		sourceGMP:      sourceApps.ICS27GMP,
		sourceRouter:   sourceApps.ICS26Router,
		counter:        destinationApps.Counter,
		sourceClientID: clients.SourceClientID,
		destClientID:   clients.DestClientID,
		destGMP:        destinationApps.ICS27GMP,
		destToken:      destinationApps.Token,
		defaultCall:    defaultCall,
	}
}

func resolveDeploymentRoute(
	t testing.TB,
	env *environment.Environment,
	deployment *Deployment,
	route Route,
) (*environment.Chain, *environment.Chain, ChainDeployment, ChainDeployment, RouteClients) {
	t.Helper()
	require.NotNil(t, env, "e2etest: Environment is required")
	require.NotNil(t, deployment, "e2etest: deployment is required")

	source, err := env.Chain(route.Source)
	require.NoError(t, err, "e2etest: resolve source Chain %q", route.Source)
	destination, err := env.Chain(route.Destination)
	require.NoError(t, err, "e2etest: resolve destination Chain %q", route.Destination)
	sourceDeployment, ok := deployment.Chain(route.Source)
	require.True(t, ok, "e2etest: deployment has no source Chain %q", route.Source)
	destinationDeployment, ok := deployment.Chain(route.Destination)
	require.True(t, ok, "e2etest: deployment has no destination Chain %q", route.Destination)
	clients, ok := deployment.RouteClients(route.ID)
	require.True(t, ok, "e2etest: deployment has no route %q", route.ID)
	return source, destination, sourceDeployment, destinationDeployment, clients
}

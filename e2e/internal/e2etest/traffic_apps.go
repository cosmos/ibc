// SPDX-License-Identifier: Apache-2.0

package e2etest

import (
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/stretchr/testify/require"

	"github.com/cosmos/ibc/e2e/internal/harness/chain/evm"
	"github.com/cosmos/ibc/e2e/internal/harness/environment"
	"github.com/cosmos/ibc/gen/go/solidity-abi/counter"
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
	r := resolveAppRoute(t, env, deployment, route, sender.account)
	return &Transfer{
		sourceRoute:  r.sourceRoute,
		destination:  r.destination,
		sourceToken:  r.sourceApps.Token,
		sourceICS20:  r.sourceApps.ICS20Transfer,
		destRouter:   r.destinationApps.ICS26Router,
		destICS20:    r.destinationApps.ICS20Transfer,
		destClientID: r.destClientID,
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
	r := resolveAppRoute(t, env, deployment, route, sender.account)
	return &IFT{
		sourceRoute: r.sourceRoute,
		destination: r.destination,
		sourceIFT:   r.sourceApps.IFT,
		destIFT:     r.destinationApps.IFT,
		destRouter:  r.destinationApps.ICS26Router,
		batcher:     r.sourceApps.IFTBatchShim,
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
	r := resolveAppRoute(t, env, deployment, route, deployer.account)

	sourceIFT, err := deployIFTToken(
		t.Context(), r.source.evm, deployer.account, r.sourceApps.ICS27GMP,
	)
	require.NoError(t, err, "e2etest: deploy additional IFT on Chain %q", route.Source)
	destinationIFT, err := deployIFTToken(
		t.Context(), r.destination.evm, deployer.account, r.destinationApps.ICS27GMP,
	)
	require.NoError(t, err, "e2etest: deploy additional IFT on Chain %q", route.Destination)
	batcher, err := deployIFTBatchShim(
		t.Context(), r.source.evm, deployer.account, sourceIFT, initialTokenSupply,
	)
	require.NoError(t, err, "e2etest: deploy additional IFT batch transfer shim on Chain %q", route.Source)

	registerIFTBridge(
		t, r.source.evm, deployer, route.Source,
		sourceIFT, r.sourceClientID, destinationIFT, r.sourceApps.IFTSendCallConstructor,
	)
	if !route.SkipDestinationIFTBridge {
		registerIFTBridge(
			t, r.destination.evm, deployer, route.Destination,
			destinationIFT, r.destClientID, sourceIFT, r.destinationApps.IFTSendCallConstructor,
		)
	}

	return &IFT{
		sourceRoute: r.sourceRoute,
		destination: r.destination,
		sourceIFT:   sourceIFT,
		destIFT:     destinationIFT,
		destRouter:  r.destinationApps.ICS26Router,
		batcher:     batcher,
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
	r := resolveAppRoute(t, env, deployment, route, sender.account)
	bound := mustBinding(counter.NewCounterTransactor(common.Address{}, nil))
	defaultCall, err := calldata(bound.Increment)
	require.NoError(t, err, "e2etest: pack Counter.increment()")
	return &GMP{
		sourceRoute:  r.sourceRoute,
		destination:  r.destination,
		sourceGMP:    r.sourceApps.ICS27GMP,
		counter:      r.destinationApps.Counter,
		destClientID: r.destClientID,
		destGMP:      r.destinationApps.ICS27GMP,
		destToken:    r.destinationApps.Token,
		defaultCall:  defaultCall,
	}
}

// appRoute is a route resolved against a deployment for the signer an
// application sends from.
type appRoute struct {
	sourceRoute
	destination     endpoint
	sourceApps      ChainDeployment
	destinationApps ChainDeployment
	destClientID    string
}

func resolveAppRoute(
	t testing.TB,
	env *environment.Environment,
	deployment *Deployment,
	route Route,
	sender evm.Account,
) appRoute {
	t.Helper()
	require.NotNil(t, env, "e2etest: Environment is required")
	require.NotNil(t, deployment, "e2etest: deployment is required")

	source := resolveEndpoint(t, env, route.Source)
	destination := resolveEndpoint(t, env, route.Destination)
	sourceApps, ok := deployment.Chain(route.Source)
	require.True(t, ok, "e2etest: deployment has no source Chain %q", route.Source)
	destinationApps, ok := deployment.Chain(route.Destination)
	require.True(t, ok, "e2etest: deployment has no destination Chain %q", route.Destination)
	clients, ok := deployment.RouteClients(route.ID)
	require.True(t, ok, "e2etest: deployment has no route %q", route.ID)
	return appRoute{
		sourceRoute: sourceRoute{
			routeID:        route.ID,
			source:         source,
			sender:         sender,
			sourceRouter:   sourceApps.ICS26Router,
			sourceClientID: clients.SourceClientID,
		},
		destination:     destination,
		sourceApps:      sourceApps,
		destinationApps: destinationApps,
		destClientID:    clients.DestClientID,
	}
}

func resolveEndpoint(t testing.TB, env *environment.Environment, id environment.ChainID) endpoint {
	t.Helper()
	chain, err := env.Chain(id)
	require.NoError(t, err, "e2etest: resolve Chain %q", id)
	client, err := chain.EVM()
	require.NoError(t, err, "e2etest: EVM client for Chain %q", id)
	return endpoint{chain: chain, evm: client}
}

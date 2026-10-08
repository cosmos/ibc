// SPDX-License-Identifier: Apache-2.0

package traffic

import (
	"context"
	"errors"
	"fmt"

	"github.com/ethereum/go-ethereum/common"

	"github.com/cosmos/ibc/gen/go/solidity-abi/counter"
	"github.com/cosmos/ibc/harness/environment"
)

// NewTransfer constructs the Transfer app for a route. The sender must be the
// signer that deployed the apps: it holds the minted token supply.
func NewTransfer(
	env *environment.Environment,
	deployment *Deployment,
	sender Signer,
	route Route,
) (*Transfer, error) {
	source, destination, sourceApps, destinationApps, clients, err := resolveDeploymentRoute(env, deployment, route)
	if err != nil {
		return nil, err
	}
	sourceEndpoint, destinationEndpoint, err := resolveRouteEndpoints(route.ID, source, destination)
	if err != nil {
		return nil, fmt.Errorf("traffic: resolve endpoints for Transfer on route %q: %w", route.ID, err)
	}
	return &Transfer{
		routeID:        route.ID,
		source:         sourceEndpoint,
		destination:    destinationEndpoint,
		sender:         sender.account,
		sourceToken:    sourceApps.Token,
		sourceICS20:    sourceApps.ICS20Transfer,
		sourceRouter:   sourceApps.ICS26Router,
		destRouter:     destinationApps.ICS26Router,
		destICS20:      destinationApps.ICS20Transfer,
		sourceClientID: clients.SourceClientID,
		destClientID:   clients.DestClientID,
	}, nil
}

// NewIFT constructs the IFT app for a route. The sender must be the signer
// that deployed the apps: it holds the minted token supply.
func NewIFT(
	env *environment.Environment,
	deployment *Deployment,
	sender Signer,
	route Route,
) (*IFT, error) {
	source, destination, sourceApps, destinationApps, clients, err := resolveDeploymentRoute(env, deployment, route)
	if err != nil {
		return nil, err
	}
	sourceEndpoint, destinationEndpoint, err := resolveRouteEndpoints(route.ID, source, destination)
	if err != nil {
		return nil, fmt.Errorf("traffic: resolve endpoints for IFT on route %q: %w", route.ID, err)
	}
	return &IFT{
		routeID:        route.ID,
		source:         sourceEndpoint,
		destination:    destinationEndpoint,
		sender:         sender.account,
		sourceIFT:      sourceApps.IFT,
		destIFT:        destinationApps.IFT,
		sourceRouter:   sourceApps.ICS26Router,
		destRouter:     destinationApps.ICS26Router,
		sourceClientID: clients.SourceClientID,
		batcher:        sourceApps.IFTBatchShim,
	}, nil
}

// DeployIFTTokenPair deploys, funds, and registers another IFT pair on an existing route.
func DeployIFTTokenPair(
	ctx context.Context,
	env *environment.Environment,
	deployment *Deployment,
	deployer Signer,
	route Route,
) (*IFT, error) {
	source, destination, sourceApps, destinationApps, clients, err := resolveDeploymentRoute(env, deployment, route)
	if err != nil {
		return nil, err
	}
	sourceEndpoint, destinationEndpoint, err := resolveRouteEndpoints(route.ID, source, destination)
	if err != nil {
		return nil, fmt.Errorf("traffic: resolve endpoints for additional IFT on route %q: %w", route.ID, err)
	}

	sourceIFT, err := deployIFTToken(ctx, sourceEndpoint.evm, deployer.account, sourceApps.ICS27GMP)
	if err != nil {
		return nil, fmt.Errorf("traffic: deploy additional IFT on Chain %q: %w", route.Source, err)
	}
	destinationIFT, err := deployIFTToken(ctx, destinationEndpoint.evm, deployer.account, destinationApps.ICS27GMP)
	if err != nil {
		return nil, fmt.Errorf("traffic: deploy additional IFT on Chain %q: %w", route.Destination, err)
	}
	batcher, err := deployIFTBatchShim(ctx, sourceEndpoint.evm, deployer.account, sourceIFT, initialTokenSupply)
	if err != nil {
		return nil, fmt.Errorf(
			"traffic: deploy additional IFT batch transfer shim on Chain %q: %w", route.Source, err,
		)
	}

	if err := registerIFTBridge(
		ctx, sourceEndpoint.evm, deployer, route.Source,
		sourceIFT, clients.SourceClientID, destinationIFT, sourceApps.IFTSendCallConstructor,
	); err != nil {
		return nil, err
	}
	if !route.SkipDestinationIFTBridge {
		if err := registerIFTBridge(
			ctx, destinationEndpoint.evm, deployer, route.Destination,
			destinationIFT, clients.DestClientID, sourceIFT, destinationApps.IFTSendCallConstructor,
		); err != nil {
			return nil, err
		}
	}

	return &IFT{
		routeID:        route.ID,
		source:         sourceEndpoint,
		destination:    destinationEndpoint,
		sender:         deployer.account,
		sourceIFT:      sourceIFT,
		destIFT:        destinationIFT,
		sourceRouter:   sourceApps.ICS26Router,
		destRouter:     destinationApps.ICS26Router,
		sourceClientID: clients.SourceClientID,
		batcher:        batcher,
	}, nil
}

// NewGMP constructs the GMP app for a route. The sender must be the signer
// that deployed the apps: it holds the minted token supply.
func NewGMP(
	env *environment.Environment,
	deployment *Deployment,
	sender Signer,
	route Route,
) (*GMP, error) {
	source, destination, sourceApps, destinationApps, clients, err := resolveDeploymentRoute(env, deployment, route)
	if err != nil {
		return nil, err
	}
	sourceEndpoint, destinationEndpoint, err := resolveRouteEndpoints(route.ID, source, destination)
	if err != nil {
		return nil, fmt.Errorf("traffic: resolve endpoints for GMP on route %q: %w", route.ID, err)
	}
	bound := mustBinding(counter.NewCounterTransactor(common.Address{}, nil))
	defaultCall, err := calldata(bound.Increment)
	if err != nil {
		return nil, fmt.Errorf("traffic: pack Counter.increment(): %w", err)
	}
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
	}, nil
}

func resolveDeploymentRoute(
	env *environment.Environment,
	deployment *Deployment,
	route Route,
) (
	source, destination *environment.Chain,
	sourceApps, destinationApps ChainDeployment,
	clients RouteClients,
	err error,
) {
	fail := func(err error) (
		*environment.Chain, *environment.Chain, ChainDeployment, ChainDeployment, RouteClients, error,
	) {
		return nil, nil, ChainDeployment{}, ChainDeployment{}, RouteClients{}, err
	}
	if env == nil {
		return fail(errors.New("traffic: Environment is required"))
	}
	if deployment == nil {
		return fail(errors.New("traffic: deployment is required"))
	}

	if source, err = env.Chain(route.Source); err != nil {
		return fail(fmt.Errorf("traffic: resolve source Chain %q: %w", route.Source, err))
	}
	if destination, err = env.Chain(route.Destination); err != nil {
		return fail(fmt.Errorf("traffic: resolve destination Chain %q: %w", route.Destination, err))
	}
	var ok bool
	if sourceApps, ok = deployment.Chain(route.Source); !ok {
		return fail(fmt.Errorf("traffic: deployment has no source Chain %q", route.Source))
	}
	if destinationApps, ok = deployment.Chain(route.Destination); !ok {
		return fail(fmt.Errorf("traffic: deployment has no destination Chain %q", route.Destination))
	}
	if clients, ok = deployment.RouteClients(route.ID); !ok {
		return fail(fmt.Errorf("traffic: deployment has no route %q", route.ID))
	}
	return source, destination, sourceApps, destinationApps, clients, nil
}

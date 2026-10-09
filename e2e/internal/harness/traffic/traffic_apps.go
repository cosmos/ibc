// SPDX-License-Identifier: Apache-2.0

package traffic

import (
	"context"
	"errors"
	"fmt"

	"github.com/ethereum/go-ethereum/common"

	"github.com/cosmos/ibc/e2e/internal/harness/environment"
	"github.com/cosmos/ibc/gen/go/solidity-abi/counter"
)

// NewTransfer constructs the Transfer app for a route. The sender pays for and
// sends the transfers, so it must hold the token: the deployer holds the
// minted supply.
func NewTransfer(
	env *environment.Environment,
	deployment *Deployment,
	sender Signer,
	route Route,
) (*Transfer, error) {
	r, err := resolveRoute(env, deployment, route)
	if err != nil {
		return nil, err
	}
	return &Transfer{
		routeID:        route.ID,
		source:         r.source,
		destination:    r.destination,
		sender:         sender.account,
		sourceToken:    r.sourceApps.Token,
		sourceICS20:    r.sourceApps.ICS20Transfer,
		sourceRouter:   r.sourceApps.ICS26Router,
		destRouter:     r.destinationApps.ICS26Router,
		destICS20:      r.destinationApps.ICS20Transfer,
		sourceClientID: r.clients.SourceClientID,
		destClientID:   r.clients.DestClientID,
	}, nil
}

// NewIFT constructs the IFT app for a route. The sender pays for and sends the
// transfers, so it must hold the token: the deployer holds the minted supply,
// and other senders are funded with FundWallets.
func NewIFT(
	env *environment.Environment,
	deployment *Deployment,
	sender Signer,
	route Route,
) (*IFT, error) {
	r, err := resolveRoute(env, deployment, route)
	if err != nil {
		return nil, err
	}
	return &IFT{
		routeID:        route.ID,
		source:         r.source,
		destination:    r.destination,
		sender:         sender.account,
		sourceIFT:      r.sourceApps.IFT,
		destIFT:        r.destinationApps.IFT,
		sourceRouter:   r.sourceApps.ICS26Router,
		destRouter:     r.destinationApps.ICS26Router,
		sourceClientID: r.clients.SourceClientID,
		batcher:        r.sourceApps.IFTBatchShim,
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
	r, err := resolveRoute(env, deployment, route)
	if err != nil {
		return nil, err
	}

	sourceIFT, err := deployIFTToken(ctx, r.source.evm, deployer.account, r.sourceApps.ICS27GMP)
	if err != nil {
		return nil, fmt.Errorf("traffic: deploy additional IFT on Chain %q: %w", route.Source, err)
	}
	destinationIFT, err := deployIFTToken(ctx, r.destination.evm, deployer.account, r.destinationApps.ICS27GMP)
	if err != nil {
		return nil, fmt.Errorf("traffic: deploy additional IFT on Chain %q: %w", route.Destination, err)
	}
	batcher, err := deployIFTBatchShim(ctx, r.source.evm, deployer.account, sourceIFT, initialTokenSupply)
	if err != nil {
		return nil, fmt.Errorf(
			"traffic: deploy additional IFT batch transfer shim on Chain %q: %w", route.Source, err,
		)
	}

	if err := registerIFTBridge(
		ctx, r.source.evm, deployer, route.Source,
		sourceIFT, r.clients.SourceClientID, destinationIFT, r.sourceApps.IFTSendCallConstructor,
	); err != nil {
		return nil, err
	}
	if !route.SkipDestinationIFTBridge {
		if err := registerIFTBridge(
			ctx, r.destination.evm, deployer, route.Destination,
			destinationIFT, r.clients.DestClientID, sourceIFT, r.destinationApps.IFTSendCallConstructor,
		); err != nil {
			return nil, err
		}
	}

	return &IFT{
		routeID:        route.ID,
		source:         r.source,
		destination:    r.destination,
		sender:         deployer.account,
		sourceIFT:      sourceIFT,
		destIFT:        destinationIFT,
		sourceRouter:   r.sourceApps.ICS26Router,
		destRouter:     r.destinationApps.ICS26Router,
		sourceClientID: r.clients.SourceClientID,
		batcher:        batcher,
	}, nil
}

// NewGMP constructs the GMP app for a route. The sender pays for the calls;
// the deployer holds the minted token supply.
func NewGMP(
	env *environment.Environment,
	deployment *Deployment,
	sender Signer,
	route Route,
) (*GMP, error) {
	r, err := resolveRoute(env, deployment, route)
	if err != nil {
		return nil, err
	}
	bound := mustBinding(counter.NewCounterTransactor(common.Address{}, nil))
	defaultCall, err := calldata(bound.Increment)
	if err != nil {
		return nil, fmt.Errorf("traffic: pack Counter.increment(): %w", err)
	}
	return &GMP{
		routeID:        route.ID,
		source:         r.source,
		destination:    r.destination,
		sender:         sender.account,
		sourceGMP:      r.sourceApps.ICS27GMP,
		sourceRouter:   r.sourceApps.ICS26Router,
		counter:        r.destinationApps.Counter,
		sourceClientID: r.clients.SourceClientID,
		destClientID:   r.clients.DestClientID,
		destGMP:        r.destinationApps.ICS27GMP,
		destToken:      r.destinationApps.Token,
		defaultCall:    defaultCall,
	}, nil
}

// resolvedRoute is a route's Chains, deployed apps, and clients.
type resolvedRoute struct {
	source, destination         endpoint
	sourceApps, destinationApps ChainDeployment
	clients                     RouteClients
}

func resolveRoute(env *environment.Environment, deployment *Deployment, route Route) (resolvedRoute, error) {
	var r resolvedRoute
	switch {
	case env == nil:
		return r, errors.New("traffic: Environment is required")
	case deployment == nil:
		return r, errors.New("traffic: deployment is required")
	case route.ID == "":
		return r, errors.New("traffic: route id is required")
	case route.Source == route.Destination:
		return r, fmt.Errorf("traffic: route %q must connect different Chains", route.ID)
	}
	var err error
	if r.source, err = resolveEndpoint(env, route.Source); err != nil {
		return r, fmt.Errorf("traffic: source of route %q: %w", route.ID, err)
	}
	if r.destination, err = resolveEndpoint(env, route.Destination); err != nil {
		return r, fmt.Errorf("traffic: destination of route %q: %w", route.ID, err)
	}
	var ok bool
	if r.sourceApps, ok = deployment.Chain(route.Source); !ok {
		return r, fmt.Errorf("traffic: deployment has no source Chain %q", route.Source)
	}
	if r.destinationApps, ok = deployment.Chain(route.Destination); !ok {
		return r, fmt.Errorf("traffic: deployment has no destination Chain %q", route.Destination)
	}
	if r.clients, ok = deployment.RouteClients(route.ID); !ok {
		return r, fmt.Errorf("traffic: deployment has no route %q", route.ID)
	}
	return r, nil
}

func resolveEndpoint(env *environment.Environment, id environment.ChainID) (endpoint, error) {
	chain, err := env.Chain(id)
	if err != nil {
		return endpoint{}, fmt.Errorf("resolve Chain %q: %w", id, err)
	}
	evmAccess, err := chain.EVM()
	if err != nil {
		return endpoint{}, fmt.Errorf("Chain %q: %w", id, err)
	}
	return endpoint{chain: chain, evm: evmAccess}, nil
}

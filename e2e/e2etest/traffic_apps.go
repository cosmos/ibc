package e2etest

import (
	"testing"

	"github.com/cosmos/ibc/e2e/internal/harness/environment"
	"github.com/cosmos/ibc/e2e/internal/harness/environment/solidityibc/counter"
)

func BindTransfer(
	t testing.TB,
	env *environment.Environment,
	deployment *Deployment,
	signers Signers,
	route Route,
) *Transfer {
	t.Helper()
	source, destination, sourceApps, destinationApps, clients := bindDeploymentRoute(t, env, deployment, route)
	sourceEndpoint, destinationEndpoint, err := bindRoute(route.ID, source, destination)
	if err != nil {
		t.Fatalf("e2etest: bind Transfer on route %q: %v", route.ID, err)
	}
	return &Transfer{
		routeID:      route.ID,
		source:       sourceEndpoint,
		destination:  destinationEndpoint,
		sender:       signers.application.account,
		sourceToken:  sourceApps.Token,
		sourceICS20:  sourceApps.ICS20Transfer,
		sourceRouter: sourceApps.ICS26Router,
		destICS20:    destinationApps.ICS20Transfer,
		sourceClient: clients.SourceClient,
		destClient:   clients.DestClient,
	}
}

func BindIFT(
	t testing.TB,
	env *environment.Environment,
	deployment *Deployment,
	signers Signers,
	route Route,
) *IFT {
	t.Helper()
	source, destination, sourceApps, destinationApps, clients := bindDeploymentRoute(t, env, deployment, route)
	sourceEndpoint, destinationEndpoint, err := bindRoute(route.ID, source, destination)
	if err != nil {
		t.Fatalf("e2etest: bind IFT on route %q: %v", route.ID, err)
	}
	return &IFT{
		routeID:      route.ID,
		source:       sourceEndpoint,
		destination:  destinationEndpoint,
		sender:       signers.application.account,
		sourceIFT:    sourceApps.IFT,
		destIFT:      destinationApps.IFT,
		sourceRouter: sourceApps.ICS26Router,
		sourceClient: clients.SourceClient,
	}
}

func BindGMP(
	t testing.TB,
	env *environment.Environment,
	deployment *Deployment,
	signers Signers,
	route Route,
) *GMP {
	t.Helper()
	source, destination, sourceApps, destinationApps, clients := bindDeploymentRoute(t, env, deployment, route)
	sourceEndpoint, destinationEndpoint, err := bindRoute(route.ID, source, destination)
	if err != nil {
		t.Fatalf("e2etest: bind GMP on route %q: %v", route.ID, err)
	}
	defaultCall, err := mustABI(counter.CounterMetaData).Pack("increment")
	if err != nil {
		t.Fatalf("e2etest: pack Counter.increment(): %v", err)
	}
	return &GMP{
		routeID:      route.ID,
		source:       sourceEndpoint,
		destination:  destinationEndpoint,
		sender:       signers.application.account,
		sourceGMP:    sourceApps.ICS27GMP,
		sourceRouter: sourceApps.ICS26Router,
		counter:      destinationApps.Counter,
		sourceClient: clients.SourceClient,
		defaultCall:  defaultCall,
	}
}

func bindDeploymentRoute(
	t testing.TB,
	env *environment.Environment,
	deployment *Deployment,
	route Route,
) (*environment.Chain, *environment.Chain, ChainDeployment, ChainDeployment, RouteClients) {
	t.Helper()
	if env == nil {
		t.Fatal("e2etest: Environment is required")
	}
	if deployment == nil {
		t.Fatal("e2etest: deployment is required")
	}

	source, err := env.Chain(route.Source)
	if err != nil {
		t.Fatalf("e2etest: resolve source Chain %q: %v", route.Source, err)
	}
	destination, err := env.Chain(route.Destination)
	if err != nil {
		t.Fatalf("e2etest: resolve destination Chain %q: %v", route.Destination, err)
	}
	sourceDeployment, ok := deployment.Chain(route.Source)
	if !ok {
		t.Fatalf("e2etest: deployment has no source Chain %q", route.Source)
	}
	destinationDeployment, ok := deployment.Chain(route.Destination)
	if !ok {
		t.Fatalf("e2etest: deployment has no destination Chain %q", route.Destination)
	}
	clients, ok := deployment.RouteClients(route.ID)
	if !ok {
		t.Fatalf("e2etest: deployment has no route %q", route.ID)
	}
	return source, destination, sourceDeployment, destinationDeployment, clients
}

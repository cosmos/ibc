package e2etest

import (
	"testing"

	"github.com/cosmos/ibc/e2e/internal/harness/environment"
	"github.com/cosmos/ibc/link/cmd/testappcmd"
)

func BindIFT(
	t testing.TB,
	env *environment.Environment,
	deployment *testappcmd.Deployment,
	signers Signers,
	route Route,
) *IFT {
	t.Helper()
	source, destination, sourceApps, destinationApps := bindDeploymentRoute(t, env, deployment, route)
	sourceEndpoint, destinationEndpoint, err := bindRoute(route.ID, source, destination)
	if err != nil {
		t.Fatalf("e2etest: bind IFT on route %q: %v", route.ID, err)
	}
	sourceApp, err := address("source MockIFT", sourceApps.MockIFT)
	if err != nil {
		t.Fatalf("e2etest: bind IFT on route %q: %v", route.ID, err)
	}
	destinationApp, err := address("destination MockIFT", destinationApps.MockIFT)
	if err != nil {
		t.Fatalf("e2etest: bind IFT on route %q: %v", route.ID, err)
	}
	return &IFT{
		routeID:     route.ID,
		source:      sourceEndpoint,
		destination: destinationEndpoint,
		sender:      signers.application.account,
		sourceApp:   sourceApp,
		destApp:     destinationApp,
	}
}

func BindGMP(
	t testing.TB,
	env *environment.Environment,
	deployment *testappcmd.Deployment,
	signers Signers,
	route Route,
) *GMP {
	t.Helper()
	source, destination, sourceApps, destinationApps := bindDeploymentRoute(t, env, deployment, route)
	sourceEndpoint, destinationEndpoint, err := bindRoute(route.ID, source, destination)
	if err != nil {
		t.Fatalf("e2etest: bind GMP on route %q: %v", route.ID, err)
	}
	sourceApp, err := address("source MockGMP", sourceApps.MockGMP)
	if err != nil {
		t.Fatalf("e2etest: bind GMP on route %q: %v", route.ID, err)
	}
	destinationApp, err := address("destination MockGMP", destinationApps.MockGMP)
	if err != nil {
		t.Fatalf("e2etest: bind GMP on route %q: %v", route.ID, err)
	}
	counter, err := address("destination Counter", destinationApps.Counter)
	if err != nil {
		t.Fatalf("e2etest: bind GMP on route %q: %v", route.ID, err)
	}
	defaultCall, err := counterABI.Pack("increment")
	if err != nil {
		t.Fatalf("e2etest: pack Counter.increment(): %v", err)
	}
	return &GMP{
		routeID:     route.ID,
		source:      sourceEndpoint,
		destination: destinationEndpoint,
		sender:      signers.application.account,
		sourceApp:   sourceApp,
		destApp:     destinationApp,
		counter:     counter,
		defaultCall: defaultCall,
	}
}

func bindDeploymentRoute(
	t testing.TB,
	env *environment.Environment,
	deployment *testappcmd.Deployment,
	route Route,
) (*environment.Chain, *environment.Chain, testappcmd.ChainDeployment, testappcmd.ChainDeployment) {
	t.Helper()
	if env == nil {
		t.Fatal("e2etest: Environment is required")
	}
	if deployment == nil {
		t.Fatal("e2etest: test-application deployment is required")
	}

	source, err := env.Chain(route.Source)
	if err != nil {
		t.Fatalf("e2etest: resolve source Chain %q: %v", route.Source, err)
	}
	destination, err := env.Chain(route.Destination)
	if err != nil {
		t.Fatalf("e2etest: resolve destination Chain %q: %v", route.Destination, err)
	}
	sourceDeployment, ok := deployment.Chain(string(route.Source))
	if !ok {
		t.Fatalf("e2etest: deployment has no source Chain %q", route.Source)
	}
	destinationDeployment, ok := deployment.Chain(string(route.Destination))
	if !ok {
		t.Fatalf("e2etest: deployment has no destination Chain %q", route.Destination)
	}
	return source, destination, sourceDeployment, destinationDeployment
}

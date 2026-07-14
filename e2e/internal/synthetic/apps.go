package synthetic

import (
	"testing"

	"github.com/cosmos/ibc/e2e/internal/harness/environment"
	"github.com/cosmos/ibc/e2e/internal/harness/ibclink/wire"
	"github.com/cosmos/ibc/e2e/internal/testapp"
)

func BindIFT(
	t testing.TB,
	env *environment.Environment,
	deployment *wire.TestAppDeployment,
	signers Signers,
	route Route,
) *testapp.IFT {
	t.Helper()
	if err := signers.validate(); err != nil {
		t.Fatalf("synthetic: invalid signers: %v", err)
	}
	source, destination, sourceApps, destinationApps := bindRoute(t, env, deployment, route)
	app, err := testapp.NewIFT(route.ID, source, destination, signers.application.account, testapp.IFTContracts{
		Source:      sourceApps.MockIFT,
		Destination: destinationApps.MockIFT,
	})
	if err != nil {
		t.Fatalf("synthetic: bind IFT on route %q: %v", route.ID, err)
	}
	return app
}

func BindGMP(
	t testing.TB,
	env *environment.Environment,
	deployment *wire.TestAppDeployment,
	signers Signers,
	route Route,
) *testapp.GMP {
	t.Helper()
	if err := signers.validate(); err != nil {
		t.Fatalf("synthetic: invalid signers: %v", err)
	}
	source, destination, sourceApps, destinationApps := bindRoute(t, env, deployment, route)
	app, err := testapp.NewGMP(route.ID, source, destination, signers.application.account, testapp.GMPContracts{
		Source:      sourceApps.MockGMP,
		Destination: destinationApps.MockGMP,
		Counter:     destinationApps.Counter,
	})
	if err != nil {
		t.Fatalf("synthetic: bind GMP on route %q: %v", route.ID, err)
	}
	return app
}

func bindRoute(
	t testing.TB,
	env *environment.Environment,
	deployment *wire.TestAppDeployment,
	route Route,
) (*environment.Chain, *environment.Chain, wire.ChainTestAppDeployment, wire.ChainTestAppDeployment) {
	t.Helper()
	if env == nil {
		t.Fatal("synthetic: Environment is required")
	}
	if deployment == nil {
		t.Fatal("synthetic: test-application deployment is required")
	}

	source, err := env.Chain(route.Source)
	if err != nil {
		t.Fatalf("synthetic: resolve source Chain %q: %v", route.Source, err)
	}
	destination, err := env.Chain(route.Destination)
	if err != nil {
		t.Fatalf("synthetic: resolve destination Chain %q: %v", route.Destination, err)
	}
	sourceDeployment, ok := deployment.Chain(string(route.Source))
	if !ok {
		t.Fatalf("synthetic: deployment has no source Chain %q", route.Source)
	}
	destinationDeployment, ok := deployment.Chain(string(route.Destination))
	if !ok {
		t.Fatalf("synthetic: deployment has no destination Chain %q", route.Destination)
	}
	return source, destination, sourceDeployment, destinationDeployment
}

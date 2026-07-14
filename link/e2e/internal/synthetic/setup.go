// Package synthetic contains the temporary test-application bootstrap and
// relayer plumbing used by the end-to-end suite.
package synthetic

import (
	"context"
	"errors"
	"math/big"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"

	"github.com/cosmos/ibc/link/e2e/internal/testapp"
	"github.com/cosmos/ibc/link/harness/environment"
	"github.com/cosmos/ibc/link/harness/ibclink"
	"github.com/cosmos/ibc/link/harness/ibclink/wire"
)

const relayerStopTimeout = 15 * time.Second

const weiPerEther int64 = 1_000_000_000_000_000_000

// RequiredSignerBalance is the minimum balance external Chains must provision
// for each synthetic actor before deployment.
func RequiredSignerBalance() *big.Int {
	return new(big.Int).Mul(big.NewInt(1_000), big.NewInt(weiPerEther))
}

type Route struct {
	ID          testapp.RouteID
	Source      environment.ChainID
	Destination environment.ChainID
	Manual      bool
}

const (
	RouteAtoB testapp.RouteID = "route-a-to-b"
	RouteBtoA testapp.RouteID = "route-b-to-a"
)

func Bidirectional(a, b environment.ChainID) []Route {
	return []Route{
		{ID: RouteAtoB, Source: a, Destination: b},
		{ID: RouteBtoA, Source: b, Destination: a},
	}
}

func AtoB(a, b environment.ChainID) Route {
	return Route{ID: RouteAtoB, Source: a, Destination: b}
}

func ManualAtoB(a, b environment.ChainID) Route {
	return Route{ID: RouteAtoB, Source: a, Destination: b, Manual: true}
}

// Deploy writes a temporary black-box configuration, runs its migration, and
// deploys the synthetic test applications. It does not start the relayer.
func Deploy(
	t testing.TB,
	env *environment.Environment,
	signers Signers,
	routes ...Route,
) (*ibclink.Driver, *wire.TestAppDeployment) {
	t.Helper()
	if env == nil {
		t.Fatal("synthetic: Environment is required")
	}
	if err := signers.validate(); err != nil {
		t.Fatalf("synthetic: invalid signers: %v", err)
	}
	ensureSignerBalances(t, env, signers)

	dir := t.TempDir()
	configuredSigners, err := signers.store(dir)
	if err != nil {
		t.Fatalf("synthetic: store signers: %v", err)
	}
	configPath := filepath.Join(dir, "ibc-link.config.yaml")
	driver, err := ibclink.NewDriver(configPath)
	if err != nil {
		t.Fatalf("synthetic: create driver: %v", err)
	}
	if bindErr := env.BindIBCLink(driver); bindErr != nil {
		t.Fatalf("synthetic: bind IBC Link process: %v", bindErr)
	}
	config := buildConfig(t, env, driver, routes, configuredSigners, filepath.Join(dir, "relayer.db"))
	data, err := config.Marshal()
	if err != nil {
		t.Fatalf("synthetic: encode config: %v", err)
	}
	if writeErr := os.WriteFile(configPath, data, 0o600); writeErr != nil {
		t.Fatalf("synthetic: write config: %v", writeErr)
	}

	if migrationErr := driver.MigrateUp(t.Context()); migrationErr != nil {
		t.Fatalf("synthetic: migrate database: %v", migrationErr)
	}

	deployment, err := driver.DeployTestApps(t.Context())
	if err != nil {
		t.Fatalf("synthetic: deploy test applications: %v", err)
	}
	return driver, deployment
}

// StartRelayer starts the synthetic relayer and registers idempotent teardown.
func StartRelayer(
	t testing.TB,
	driver *ibclink.Driver,
	env *environment.Environment,
) *ibclink.Relayer {
	t.Helper()
	if driver == nil {
		t.Fatal("synthetic: driver is required")
	}
	if env == nil {
		t.Fatal("synthetic: Environment is required")
	}

	relayer, err := driver.StartRelayer(t.Context())
	if err != nil {
		t.Fatalf("synthetic: start relayer: %v", err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), relayerStopTimeout)
		defer cancel()
		if err := relayer.Stop(ctx); err != nil {
			t.Errorf("synthetic: stop relayer: %v", err)
		}
	})

	connected := make(map[string]struct{}, len(relayer.Ready().ChainsConnected))
	for _, id := range relayer.Ready().ChainsConnected {
		connected[id] = struct{}{}
	}
	for _, id := range env.Chains() {
		if _, ok := connected[string(id)]; !ok {
			t.Fatalf("synthetic: relayer did not connect to Chain %q", id)
		}
	}
	return relayer
}

func buildConfig(
	t testing.TB,
	env *environment.Environment,
	driver *ibclink.Driver,
	routes []Route,
	signers []wire.Signer,
	dbPath string,
) wire.ConfigYAML {
	t.Helper()
	config := wire.ConfigYAML{
		Chains:  make([]wire.Chain, 0, len(env.Chains())),
		Signers: signers,
		DB:      wire.DB{Type: wire.DBTypeSQLite, URL: dbPath},
		Relayer: wire.Relayer{Routes: make([]wire.Route, 0, len(routes))},
	}
	for _, id := range env.Chains() {
		chain, err := env.Chain(id)
		if err != nil {
			t.Fatalf("synthetic: resolve Chain %q: %v", id, err)
		}
		rpc, err := driver.ChainRPC(string(id))
		if err != nil {
			t.Fatalf("synthetic: resolve Chain %q process binding: %v", id, err)
		}
		config.Chains = append(config.Chains, wire.Chain{
			ID:            string(id),
			Type:          wire.ChainTypeEVM,
			ChainID:       chain.EVMChainID(),
			EVMSigner:     relayerSignerAlias,
			TestAppSigner: applicationSignerAlias,
			RPC:           rpc,
		})
	}

	for _, route := range routes {
		compiled := wire.Route{
			ID:          string(route.ID),
			Source:      string(route.Source),
			Destination: string(route.Destination),
			Type:        wire.RouteEVMToEVMAttested,
		}
		if route.Manual {
			compiled.AutoRelay = &wire.AutoRelay{Enabled: false}
		}
		config.Relayer.Routes = append(config.Relayer.Routes, compiled)
	}
	return config
}

func ensureSignerBalances(t testing.TB, env *environment.Environment, signers Signers) {
	t.Helper()
	addresses := signers.Addresses()
	actors := []struct {
		role    string
		address common.Address
	}{
		{role: "application", address: addresses.Application},
		{role: "relayer", address: addresses.Relayer},
	}
	minimum := RequiredSignerBalance()
	for _, id := range env.Chains() {
		chain, err := env.Chain(id)
		if err != nil {
			t.Fatalf("synthetic: resolve Chain %q: %v", id, err)
		}
		funding, err := chain.Funding()
		if err == nil {
			for _, actor := range actors {
				if fundErr := funding.EnsureEOABalance(t.Context(), actor.address, minimum); fundErr != nil {
					t.Fatalf("synthetic: fund %s signer on Chain %q: %v", actor.role, id, fundErr)
				}
			}
			continue
		}
		if !errors.Is(err, environment.ErrCapabilityUnavailable) {
			t.Fatalf("synthetic: resolve funding on Chain %q: %v", id, err)
		}
		evmAccess, evmErr := chain.EVM()
		if evmErr != nil {
			t.Fatalf("synthetic: resolve EVM access on attached Chain %q: %v", id, evmErr)
		}
		for _, actor := range actors {
			balance, balanceErr := evmAccess.BalanceAt(t.Context(), actor.address, nil)
			if balanceErr != nil {
				t.Fatalf("synthetic: query %s signer balance on attached Chain %q: %v", actor.role, id, balanceErr)
			}
			if balance.Cmp(minimum) < 0 {
				t.Fatalf(
					"synthetic: %s signer %s on attached Chain %q has balance %s, need at least %s; provision it out of band",
					actor.role,
					actor.address,
					id,
					balance,
					minimum,
				)
			}
		}
	}
}

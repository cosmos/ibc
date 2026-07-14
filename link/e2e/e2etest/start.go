// Package e2etest selects and starts reusable Environment configurations.
package e2etest

import (
	"context"
	"flag"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/cosmos/ibc/link/harness/environment"
)

const (
	ChainA environment.ChainID = "chain-a"
	ChainB environment.ChainID = "chain-b"
)

const (
	cleanupTimeout = 30 * time.Second
	laneEnv        = "E2E_LANE"

	laneAnvil         = "anvil"
	laneAnvilInterval = "anvil-interval"
	laneBesu          = "besu"

	anvilChainIDBase         = 31337
	anvilIntervalChainIDBase = 31437
	besuChainIDBase          = 32337
	anvilIntervalBlockTime   = 2 * time.Second
)

var laneFlag = flag.String(
	"e2e.lane",
	"",
	"e2e lane to run: anvil, anvil-interval, or besu; overrides E2E_LANE",
)

// Suite is one reusable Environment selection.
type Suite struct {
	environmentSpec    environment.Spec
	environmentRuntime environment.Runtime
}

func SelectedSuite(t testing.TB) Suite {
	t.Helper()
	switch selectedLaneName(t) {
	case laneBesu:
		return twoBesuSuite()
	case laneAnvilInterval:
		return twoAnvilSuite(anvilIntervalChainIDBase, anvilIntervalBlockTime)
	default:
		return twoAnvilSuite(anvilChainIDBase, 0)
	}
}

func twoAnvilSuite(base uint64, interval time.Duration) Suite {
	return suite(
		[]environment.ChainSpec{
			environment.ManagedAnvil{ID: ChainA, EVMChainID: base, BlockInterval: interval},
			environment.ManagedAnvil{ID: ChainB, EVMChainID: base + 1, BlockInterval: interval},
		},
		environment.Runtime{},
	)
}

func twoBesuSuite() Suite {
	return suite(
		[]environment.ChainSpec{
			environment.ManagedBesu{ID: ChainA, EVMChainID: besuChainIDBase},
			environment.ManagedBesu{ID: ChainB, EVMChainID: besuChainIDBase + 1},
		},
		environment.Runtime{},
	)
}

// SuiteFor builds a selection for an explicit Environment.
func SuiteFor(spec environment.Spec, runtime environment.Runtime) Suite {
	return Suite{
		environmentSpec:    spec,
		environmentRuntime: runtime,
	}
}

func suite(chains []environment.ChainSpec, runtime environment.Runtime) Suite {
	return SuiteFor(environment.Spec{Chains: chains}, runtime)
}

// RequireAnvilLane deduplicates suites pinned to Anvil regardless of the
// selected lane.
func RequireAnvilLane(t testing.TB) {
	t.Helper()
	got := selectedLaneName(t)
	if got != laneAnvil {
		t.Skipf("runs only in the default anvil lane; selected %s", got)
	}
}

func selectedLaneName(t testing.TB) string {
	t.Helper()
	name := normalizeLaneName(rawLaneName())
	switch name {
	case laneAnvil, laneAnvilInterval, laneBesu:
		return name
	default:
		t.Fatalf(
			"unknown e2e lane %q; set %s or -e2e.lane to anvil, anvil-interval, or besu",
			rawLaneName(),
			laneEnv,
		)
		return ""
	}
}

func rawLaneName() string {
	if strings.TrimSpace(*laneFlag) != "" {
		return *laneFlag
	}
	return os.Getenv(laneEnv)
}

func normalizeLaneName(name string) string {
	trimmed := strings.ToLower(strings.TrimSpace(name))
	if trimmed == "" {
		return laneAnvil
	}
	return trimmed
}

func Start(t testing.TB, selected Suite) *environment.Environment {
	t.Helper()
	env, err := environment.Start(t.Context(), selected.environmentSpec, selected.environmentRuntime)
	if err != nil {
		t.Fatalf("e2etest: start Environment: %v", err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), cleanupTimeout)
		defer cancel()
		if err := env.Close(ctx); err != nil {
			t.Errorf("e2etest: close Environment: %v", err)
		}
	})
	return env
}

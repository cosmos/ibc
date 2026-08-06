// Package e2etest starts Environments and provides the traffic bindings the
// acceptance tests drive against IBC Link.
package e2etest

import (
	"context"
	"flag"
	"maps"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/cosmos/ibc/e2e/internal/harness/environment"
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

const ProtocolAuthorityID environment.AuthorityID = "protocol-deployer"

// Deterministic deployer key used by test protocol realization; funded by managed Chains.
const protocolAuthorityKeyHex = "0000000000000000000000000000000000000000000000000000000000000005"

var laneFlag = flag.String(
	"e2e.lane",
	"",
	"e2e lane to run: anvil, anvil-interval, or besu; overrides E2E_LANE",
)

// ChainSpecsForConfiguredLane returns the two-chain topology selected by the
// e2e lane flag or environment variable.
func ChainSpecsForConfiguredLane(t testing.TB) []environment.ChainSpec {
	t.Helper()
	switch selectedLaneName(t) {
	case laneBesu:
		return []environment.ChainSpec{
			environment.ManagedBesu{ID: ChainA, EVMChainID: besuChainIDBase},
			environment.ManagedBesu{ID: ChainB, EVMChainID: besuChainIDBase + 1},
		}
	case laneAnvilInterval:
		return anvilChainSpecs(anvilIntervalChainIDBase, anvilIntervalBlockTime)
	default:
		return anvilChainSpecs(anvilChainIDBase, 0)
	}
}

func anvilChainSpecs(base uint64, interval time.Duration) []environment.ChainSpec {
	return []environment.ChainSpec{
		environment.ManagedAnvil{ID: ChainA, EVMChainID: base, BlockInterval: interval},
		environment.ManagedAnvil{ID: ChainB, EVMChainID: base + 1, BlockInterval: interval},
	}
}

// RuntimeWithProtocolDeployer returns runtime with the protocol deployer
// authority, without retaining caller-owned maps.
func RuntimeWithProtocolDeployer(runtime environment.Runtime) environment.Runtime {
	runtime.Endpoints = maps.Clone(runtime.Endpoints)
	runtime.Authorities = maps.Clone(runtime.Authorities)
	if runtime.Authorities == nil {
		runtime.Authorities = map[environment.AuthorityID]environment.EVMAuthority{}
	}
	runtime.Authorities[ProtocolAuthorityID] = environment.EVMAuthority{
		PrivateKeyHex: protocolAuthorityKeyHex,
	}
	return runtime
}

// RequireAnvilLane skips tests pinned to Anvil in other lanes.
func RequireAnvilLane(t testing.TB) {
	t.Helper()
	got := selectedLaneName(t)
	if got != laneAnvil {
		t.Skipf("runs only in the default anvil lane; selected %s", got)
	}
}

func RequireAnvilNodeLifecycle(t testing.TB) {
	t.Helper()
	if selectedLaneName(t) == laneBesu {
		t.Skip("requires Anvil node lifecycle control")
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

func Start(t testing.TB, spec environment.Spec, runtime environment.Runtime) *environment.Environment {
	t.Helper()
	env, err := environment.Start(t.Context(), spec, runtime)
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

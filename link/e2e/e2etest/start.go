// Package e2etest is the only e2e helper package that imports testing.
package e2etest

import (
	"context"
	"flag"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/cosmos/ibc/link/harness"
	"github.com/cosmos/ibc/link/harness/chain/sandboxd"
	"github.com/cosmos/ibc/link/harness/topology"
)

const (
	cleanupTimeout = 30 * time.Second
	laneEnv        = "E2E_LANE"

	laneAnvil         = "anvil"
	laneAnvilInterval = "anvil-interval"
	laneBesu          = "besu"
	laneSandbox       = "sandbox"
)

var laneFlag = flag.String(
	"e2e.lane",
	"",
	"e2e lane to run: anvil, anvil-interval, besu, or sandbox; overrides E2E_LANE",
)

// SelectedLane resolves the runner's lane choice (E2E_LANE / -e2e.lane, defaulting to instant Anvil)
// to the topology.Lane that binds a shape to that infrastructure. The lane is the runner's; the
// shape is the test's: portable tests use SelectedTopology, and a shape-forced suite that should
// still follow the runner's lane calls SelectedLane(t)(shape).
func SelectedLane(t testing.TB) topology.Lane {
	t.Helper()
	switch selectedLaneName(t) {
	case laneBesu:
		requireDocker(t)
		return topology.Besu
	case laneSandbox:
		RequireSandboxd(t)
		return topology.Sandbox
	case laneAnvilInterval:
		return topology.AnvilInterval
	default:
		return topology.Anvil
	}
}

// SelectedTopology is the portable-test entry point: the selected lane bound to the two-EVM shape.
func SelectedTopology(t testing.TB) topology.Topology {
	t.Helper()
	return SelectedLane(t)(topology.TwoEVM())
}

// RequireAnvilLane skips a test whose topology pins the anvil lane regardless of the selected one
// (it never reads SelectedLane), so it runs exactly once — during the anvil lane pass — instead of
// redundantly under every -e2e.lane value. Runtime operations still negotiate their chain capabilities.
func RequireAnvilLane(t testing.TB) {
	t.Helper()
	got := selectedLaneName(t)
	if got != laneAnvil {
		t.Skipf(
			"runs only in the default anvil lane (dedup; its topology is anvil-pinned regardless of -e2e.lane); selected %s",
			got,
		)
	}
}

func selectedLaneName(t testing.TB) string {
	t.Helper()
	name := normalizeLaneName(rawLaneName())
	switch name {
	case laneAnvil, laneAnvilInterval, laneBesu, laneSandbox:
		return name
	default:
		t.Fatalf(
			"unknown e2e lane %q; set %s or -e2e.lane to anvil, anvil-interval, besu, or sandbox",
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

func requireDocker(t testing.TB) {
	t.Helper()
	if _, err := exec.LookPath("docker"); err != nil {
		t.Fatalf("docker is required for the besu e2e lane: %v", err)
	}
	if err := exec.Command("docker", "info").Run(); err != nil {
		t.Fatalf("docker is not running or not reachable for the besu e2e lane: %v", err)
	}
}

// RequireSandboxd fatals when the sandboxd binary is missing — the sandbox analog of requireDocker. The
// binary is a large, separately-built dependency (not committed), so a truthful "run `make bin/sandboxd`"
// message beats a cryptic launch failure deep in harness.Start. It is exported so a test whose topology
// pins a managed sandboxd chain (e.g. the evm->cosmos IFT test, which runs in the anvil lane but needs
// the sandboxd node for its cosmos slot) can guard on the binary directly. The smoke suite's make
// target builds sandboxd as a prerequisite (like build-stub), so this is a defensive guard for a
// direct `go test`, not the normal path.
func RequireSandboxd(t testing.TB) {
	t.Helper()
	bin := sandboxd.ResolvedBin()
	if info, err := os.Stat(bin); err != nil || info.IsDir() {
		t.Fatalf("sandboxd binary not found at %s: run `make bin/sandboxd` (or set SANDBOXD_BIN)", bin)
	}
}

// StartHarness brings up the topology's world — chains, compiled config, ibc link driver — and registers
// its teardown. Nothing of ibc link runs. Use it when the test drives one-shot ibc link verbs
// (validate/deploy) itself; most tests want Start.
func StartHarness(t testing.TB, topo topology.Topology) *harness.Harness {
	t.Helper()
	keep := keepAfterTest()
	h, err := harness.Start(t.Context(), harness.StartConfig{
		Topology:    topo,
		KeepOnClose: keep,
		ArtifactDir: os.Getenv("E2E_ARTIFACT_DIR"),
	})
	if err != nil {
		t.Fatalf("e2etest.StartHarness: %v", err)
	}
	if keep {
		// Logged at startup, not teardown, so the path survives a wedged or panicking test.
		t.Logf("KEEP_AFTER_TEST: world kept after test; workdir=%s", h.WorkDir())
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), cleanupTimeout)
		defer cancel()
		if err := h.Shutdown(ctx, t.Failed()); err != nil {
			t.Logf("e2etest: harness shutdown: %v", err)
		}
	})
	return h
}

// Start brings up the full stack: the harness world (StartHarness's scope) plus relayer bring-up —
// migrate, validate, deploy, relayer daemon, on-chain readers. Harness.Shutdown is the single teardown
// path (it owns every daemon instance the run started), so one cleanup registered here covers every
// failure point — including a daemon that came up during StartRelayer and failed a later assertion.
func Start(t testing.TB, topo topology.Topology) *harness.Session {
	t.Helper()
	run, err := StartHarness(t, topo).StartRelayer(t.Context())
	if err != nil {
		t.Fatalf("e2etest.Start: %v", err)
	}
	return run
}

func keepAfterTest() bool {
	v := os.Getenv("KEEP_AFTER_TEST")
	return strings.EqualFold(v, "true") || v == "1"
}

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
	"github.com/cosmos/ibc/link/harness/topology"
)

const (
	cleanupTimeout = 30 * time.Second
	laneEnv        = "E2E_LANE"

	laneAnvil         = "anvil"
	laneAnvilInterval = "anvil-interval"
	laneBesu          = "besu"
)

var laneFlag = flag.String(
	"e2e.lane",
	"",
	"e2e lane to run: anvil, anvil-interval, or besu; overrides E2E_LANE",
)

func SelectedLane(t testing.TB) topology.Lane {
	t.Helper()
	switch selectedLaneName(t) {
	case laneBesu:
		requireDocker(t)
		return topology.Besu
	case laneAnvilInterval:
		return topology.AnvilInterval
	default:
		return topology.Anvil
	}
}

func SelectedTopology(t testing.TB) topology.Topology {
	t.Helper()
	return SelectedLane(t)(topology.TwoEVM())
}

// RequireAnvilLane deduplicates anvil-pinned topologies: they ignore SelectedLane and would re-run
// under every -e2e.lane.
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

func requireDocker(t testing.TB) {
	t.Helper()
	if _, err := exec.LookPath("docker"); err != nil {
		t.Fatalf("docker is required for the besu e2e lane: %v", err)
	}
	if err := exec.Command("docker", "info").Run(); err != nil {
		t.Fatalf("docker is not running or not reachable for the besu e2e lane: %v", err)
	}
}

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

// Start registers one cleanup via Harness.Shutdown, which must own every daemon started by StartRelayer.
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

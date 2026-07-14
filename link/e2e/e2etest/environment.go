package e2etest

import (
	"testing"

	"github.com/cosmos/ibc/link/harness/environment"
)

// RequireCapabilities applies testing policy to the environment's pure
// assessment. Invalid selections fail the test; unavailable capabilities
// skip it before Environment.Start can acquire anything.
func RequireCapabilities(
	t testing.TB,
	selected Suite,
	requirements environment.Requirements,
) {
	t.Helper()

	assessment, err := environment.Assess(
		selected.environmentSpec,
		selected.environmentRuntime,
		requirements,
	)
	if err != nil {
		t.Fatalf("e2etest: selected environment is invalid: %v", err)
	}
	if !assessment.Feasible() {
		t.Skipf("e2etest: selected environment cannot satisfy test: %s", assessment)
	}
}

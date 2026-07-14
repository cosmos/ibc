package e2etest

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/cosmos/ibc/link/harness/environment"
)

func TestSuiteSelectionsExposeOnlyGuaranteedCapabilities(t *testing.T) {
	requirements := environment.Requirements{
		MiningControl: []environment.ChainID{ChainB},
		NodeLifecycle: []environment.ChainID{ChainB},
	}

	instant := twoAnvilSuite(anvilChainIDBase, 0)
	assessment, err := environment.Assess(instant.environmentSpec, instant.environmentRuntime, requirements)
	require.NoError(t, err)
	require.True(t, assessment.Feasible())

	interval := twoAnvilSuite(anvilIntervalChainIDBase, 2*time.Second)
	assessment, err = environment.Assess(interval.environmentSpec, interval.environmentRuntime, requirements)
	require.NoError(t, err)
	require.False(t, assessment.Feasible())
	require.Equal(t, environment.Requirements{MiningControl: []environment.ChainID{ChainB}}, assessment.Missing(),
		"interval Anvil retains node lifecycle but not manual mining")

	besu := twoBesuSuite()
	assessment, err = environment.Assess(besu.environmentSpec, besu.environmentRuntime, requirements)
	require.NoError(t, err)
	require.False(t, assessment.Feasible())
	require.Equal(t, requirements, assessment.Missing())
}

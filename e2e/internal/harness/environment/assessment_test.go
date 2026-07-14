package environment

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestAssessDerivesChainCapabilities(t *testing.T) {
	spec := Spec{Chains: []ChainSpec{
		ManagedAnvil{ID: "instant", EVMChainID: 31337},
		ManagedAnvil{ID: "interval", EVMChainID: 31338, BlockInterval: 2 * time.Second},
		ManagedBesu{ID: "besu", EVMChainID: 32337},
		AttachedEVM{ID: "attached", EVMChainID: 33337, Endpoint: "attached-rpc", Timing: testTiming()},
	}}
	runtime := Runtime{
		Endpoints: map[EndpointBindingID]EndpointBinding{
			"attached-rpc": {RPCURL: "http://attached.invalid"},
		},
	}

	assessment, err := Assess(spec, runtime, Requirements{
		MiningControl: []ChainID{"instant", "interval", "besu", "attached", "besu"},
		NodeLifecycle: []ChainID{"instant", "interval", "besu", "attached", "attached"},
	})
	require.NoError(t, err)
	require.False(t, assessment.Feasible())
	require.Equal(t, Requirements{
		MiningControl: []ChainID{"interval", "besu", "attached"},
		NodeLifecycle: []ChainID{"besu", "attached"},
	}, assessment.Missing())
	require.Equal(
		t,
		`Chain "interval" has no mining control; Chain "besu" has no mining control; Chain "attached" has no mining control; Chain "besu" has no node lifecycle control; Chain "attached" has no node lifecycle control`,
		assessment.String(),
	)
}

func TestAssessFeasible(t *testing.T) {
	requirements := Requirements{
		MiningControl: []ChainID{"anvil"},
		NodeLifecycle: []ChainID{"anvil"},
	}
	assessment, err := Assess(Spec{Chains: []ChainSpec{
		ManagedAnvil{ID: "anvil", EVMChainID: 31337},
	}}, Runtime{}, requirements)
	require.NoError(t, err)
	require.True(t, assessment.Feasible())
	require.Equal(t, "feasible", assessment.String())
}

func TestAssessRejectsInvalidRequirementTargets(t *testing.T) {
	spec := Spec{Chains: []ChainSpec{ManagedAnvil{ID: "anvil", EVMChainID: 31337}}}

	_, err := Assess(spec, Runtime{}, Requirements{MiningControl: []ChainID{""}})
	require.ErrorIs(t, err, ErrInvalidRequirement)
	require.ErrorContains(t, err, "MiningControl[0] has an empty Chain id")

	_, err = Assess(spec, Runtime{}, Requirements{NodeLifecycle: []ChainID{"missing"}})
	require.ErrorIs(t, err, ErrInvalidRequirement)
	require.ErrorContains(t, err, `NodeLifecycle[0] references unknown Chain "missing"`)
}

func TestAssessReturnsInvalidSelectionErrorsInsteadOfCapabilityGaps(t *testing.T) {
	attached := Spec{Chains: []ChainSpec{AttachedEVM{
		ID: "attached", EVMChainID: 31337, Endpoint: "rpc", Timing: testTiming(),
	}}}
	_, err := Assess(attached, Runtime{}, Requirements{MiningControl: []ChainID{"attached"}})
	require.ErrorContains(t, err, `no runtime endpoint binding for "rpc"`)
}

func TestAssessmentMissingReturnsDefensiveCopies(t *testing.T) {
	assessment, err := Assess(Spec{Chains: []ChainSpec{
		ManagedBesu{ID: "besu", EVMChainID: 32337},
	}}, Runtime{}, Requirements{MiningControl: []ChainID{"besu"}})
	require.NoError(t, err)

	missing := assessment.Missing()
	missing.MiningControl[0] = "mutated"
	require.Equal(t, []ChainID{"besu"}, assessment.Missing().MiningControl)
}

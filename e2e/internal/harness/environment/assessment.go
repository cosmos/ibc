package environment

import (
	"errors"
	"fmt"
	"strings"
)

var ErrInvalidRequirement = errors.New("environment: invalid requirement")

// Requirements names workflow-visible capabilities on stable authored
// resources. It is separate from Spec because these are properties a caller
// needs from a selected Environment, not properties the caller may grant to it.
type Requirements struct {
	MiningControl []ChainID
	NodeLifecycle []ChainID
}

// Assessment reports declaration-level capability gaps. It does not promise
// that external executables, Docker, credentials, or endpoints will work when
// Start performs acquisition.
type Assessment struct {
	missing Requirements
}

func (a Assessment) Feasible() bool {
	return len(a.missing.MiningControl) == 0 && len(a.missing.NodeLifecycle) == 0
}

func (a Assessment) Missing() Requirements {
	return cloneRequirements(a.missing)
}

func (a Assessment) String() string {
	if a.Feasible() {
		return "feasible"
	}
	parts := make([]string, 0, len(a.missing.MiningControl)+len(a.missing.NodeLifecycle))
	for _, id := range a.missing.MiningControl {
		parts = append(parts, fmt.Sprintf("Chain %q has no mining control", id))
	}
	for _, id := range a.missing.NodeLifecycle {
		parts = append(parts, fmt.Sprintf("Chain %q has no node lifecycle control", id))
	}
	return strings.Join(parts, "; ")
}

// Assess validates the complete selected Environment and classifies requirements
// without filesystem, process, Docker, RPC, or network work. Invalid Specs,
// runtime bindings, and requirement targets are errors; unavailable
// capabilities are returned as a non-feasible Assessment.
func Assess(spec Spec, runtime Runtime, requirements Requirements) (Assessment, error) {
	spec = spec.snapshot()
	runtime = runtime.snapshot()
	if err := spec.validate(); err != nil {
		return Assessment{}, err
	}
	if err := validateRuntime(spec, runtime); err != nil {
		return Assessment{}, err
	}

	chains := make(map[ChainID]chainCapabilities, len(spec.Chains))
	for _, declaration := range spec.Chains {
		chains[declaration.chainID()] = deriveChainCapabilities(declaration)
	}

	var assessment Assessment
	missing, err := assessChainRequirement(
		"MiningControl",
		requirements.MiningControl,
		chains,
		func(capabilities chainCapabilities) bool { return capabilities.miningControl },
	)
	if err != nil {
		return Assessment{}, err
	}
	assessment.missing.MiningControl = missing

	missing, err = assessChainRequirement(
		"NodeLifecycle",
		requirements.NodeLifecycle,
		chains,
		func(capabilities chainCapabilities) bool { return capabilities.nodeLifecycle },
	)
	if err != nil {
		return Assessment{}, err
	}
	assessment.missing.NodeLifecycle = missing
	return assessment, nil
}

type chainCapabilities struct {
	miningControl bool
	nodeLifecycle bool
}

// deriveChainCapabilities is the single declaration-level source used by both
// assessment and realization. Runtime authority can be added here when a
// concrete adapter proves that it changes a capability guarantee.
func deriveChainCapabilities(declaration ChainSpec) chainCapabilities {
	switch chain := declaration.(type) {
	case ManagedAnvil:
		return chainCapabilities{
			miningControl: chain.BlockInterval == 0,
			nodeLifecycle: true,
		}
	case ManagedBesu, AttachedEVM:
		return chainCapabilities{}
	default:
		return chainCapabilities{}
	}
}

func assessChainRequirement(
	field string,
	ids []ChainID,
	chains map[ChainID]chainCapabilities,
	provides func(chainCapabilities) bool,
) ([]ChainID, error) {
	missing := make([]ChainID, 0)
	seen := make(map[ChainID]struct{}, len(ids))
	for i, id := range ids {
		if id == "" {
			return nil, fmt.Errorf("%w: %s[%d] has an empty Chain id", ErrInvalidRequirement, field, i)
		}
		capabilities, ok := chains[id]
		if !ok {
			return nil, fmt.Errorf(
				"%w: %s[%d] references unknown Chain %q",
				ErrInvalidRequirement,
				field,
				i,
				id,
			)
		}
		if _, duplicate := seen[id]; duplicate {
			continue
		}
		seen[id] = struct{}{}
		if !provides(capabilities) {
			missing = append(missing, id)
		}
	}
	return missing, nil
}

func cloneRequirements(in Requirements) Requirements {
	return Requirements{
		MiningControl: append([]ChainID(nil), in.MiningControl...),
		NodeLifecycle: append([]ChainID(nil), in.NodeLifecycle...),
	}
}

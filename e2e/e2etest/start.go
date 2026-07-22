// Package e2etest selects and starts reusable Environment configurations and
// provides the temporary Link traffic machinery used by the acceptance tests.
package e2etest

import (
	"context"
	"flag"
	"fmt"
	"os"
	"slices"
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

const protocolAuthorityID environment.AuthorityID = "protocol-deployer"

// Deterministic deployer key used by suite protocol realization; funded by managed Chains.
const protocolAuthorityKeyHex = "0000000000000000000000000000000000000000000000000000000000000005"

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
	return SuiteFor(
		environment.Spec{Chains: []environment.ChainSpec{
			environment.ManagedAnvil{ID: ChainA, EVMChainID: base, BlockInterval: interval},
			environment.ManagedAnvil{ID: ChainB, EVMChainID: base + 1, BlockInterval: interval},
		}},
		environment.Runtime{},
	)
}

func twoBesuSuite() Suite {
	return SuiteFor(
		environment.Spec{Chains: []environment.ChainSpec{
			environment.ManagedBesu{ID: ChainA, EVMChainID: besuChainIDBase},
			environment.ManagedBesu{ID: ChainB, EVMChainID: besuChainIDBase + 1},
		}},
		environment.Runtime{},
	)
}

// SuiteFor builds a selection for an explicit Environment. Specs that omit IBC
// Instances receive a DummyClient app stack on every Chain pair.
func SuiteFor(spec environment.Spec, runtime environment.Runtime) Suite {
	spec, runtime = ensureProtocolApps(spec, runtime)
	return Suite{
		environmentSpec:    spec,
		environmentRuntime: runtime,
	}
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

func ensureProtocolApps(
	spec environment.Spec,
	runtime environment.Runtime,
) (environment.Spec, environment.Runtime) {
	if len(spec.IBCInstances) > 0 || len(spec.Connections) > 0 {
		return spec, runtime
	}

	chainIDs := make([]environment.ChainID, 0, len(spec.Chains))
	for _, declaration := range spec.Chains {
		chainIDs = append(chainIDs, chainSpecID(declaration))
	}
	slices.Sort(chainIDs)

	instances := make([]environment.IBCInstanceSpec, 0, len(chainIDs))
	for _, id := range chainIDs {
		instances = append(instances, environment.NewIBCInstance{
			ID:        instanceIDForChain(id),
			Chain:     id,
			Authority: protocolAuthorityID,
		})
	}

	connections := make([]environment.ConnectionSpec, 0)
	for i := 0; i < len(chainIDs); i++ {
		for j := i + 1; j < len(chainIDs); j++ {
			a, b := chainIDs[i], chainIDs[j]
			connectionID := connectionIDForPair(a, b)
			connections = append(connections, environment.ConnectionSpec{
				ID: connectionID,
				A: environment.DummyClient{
					ID:          clientIDForEnd(connectionID, "a"),
					IBCInstance: instanceIDForChain(a),
					Authority:   protocolAuthorityID,
				},
				B: environment.DummyClient{
					ID:          clientIDForEnd(connectionID, "b"),
					IBCInstance: instanceIDForChain(b),
					Authority:   protocolAuthorityID,
				},
			})
		}
	}

	spec.IBCInstances = instances
	spec.Connections = connections

	if runtime.Authorities == nil {
		runtime.Authorities = map[environment.AuthorityID]environment.EVMAuthority{}
	}
	if _, ok := runtime.Authorities[protocolAuthorityID]; !ok {
		runtime.Authorities[protocolAuthorityID] = environment.EVMAuthority{
			PrivateKeyHex: protocolAuthorityKeyHex,
		}
	}
	return spec, runtime
}

func chainSpecID(declaration environment.ChainSpec) environment.ChainID {
	switch chain := declaration.(type) {
	case environment.ManagedAnvil:
		return chain.ID
	case environment.ManagedBesu:
		return chain.ID
	case environment.AttachedEVM:
		return chain.ID
	default:
		panic(fmt.Sprintf("e2etest: unsupported Chain declaration %T", declaration))
	}
}

func instanceIDForChain(id environment.ChainID) environment.IBCInstanceID {
	return environment.IBCInstanceID(fmt.Sprintf("ibc-%s", id))
}

func connectionIDForPair(a, b environment.ChainID) environment.ConnectionID {
	return environment.ConnectionID(fmt.Sprintf("conn-%s-%s", a, b))
}

func clientIDForEnd(connection environment.ConnectionID, end string) environment.ClientID {
	return environment.ClientID(fmt.Sprintf("%s-%s", connection, end))
}

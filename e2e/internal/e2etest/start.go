// SPDX-License-Identifier: Apache-2.0

// Package e2etest resolves test requirements into Environments and adapts the
// harness traffic layer to testing.TB for the acceptance tests.
package e2etest

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cosmos/ibc/harness/environment"
)

const (
	cleanupTimeout = 30 * time.Second
	modeEnv        = "E2E_MODE"

	anvilChainIDBase = 31337
	besuChainIDBase  = 32337
)

type Mode string

const (
	ModeFast       Mode = "fast"
	ModeComplete   Mode = "complete"
	ModeProduction Mode = "production"
)

type EVMProvider string

const (
	EVMProviderAnvil EVMProvider = "anvil"
	EVMProviderBesu  EVMProvider = "besu"
)

type EVMRequirements struct {
	Provider         EVMProvider
	ControlledMining bool
	NodeLifecycle    bool
}

var modeFlag = flag.String(
	"e2e.mode",
	"",
	"e2e mode to run: fast, complete, or production; overrides E2E_MODE",
)

type evmResolution struct {
	chains     []environment.ChainSpec
	provider   EVMProvider
	skipReason string
}

func EVMChains(
	t testing.TB,
	requirements EVMRequirements,
	ids ...environment.ChainID,
) []environment.ChainSpec {
	t.Helper()
	mode, err := resolveMode(*modeFlag, os.Getenv(modeEnv))
	if err != nil {
		t.Fatalf("e2etest: %v", err)
	}
	resolution, err := resolveEVMChains(mode, requirements, ids)
	if err != nil {
		t.Fatalf("e2etest: %v", err)
	}
	if resolution.skipReason != "" {
		recordEVMSelection(t, mode, requirements, "", "skip", resolution.skipReason)
		t.Skipf("e2etest: %s", resolution.skipReason)
	}
	recordEVMSelection(t, mode, requirements, resolution.provider, "", "")
	return resolution.chains
}

func resolveMode(flagValue, envValue string) (Mode, error) {
	raw := envValue
	if strings.TrimSpace(flagValue) != "" {
		raw = flagValue
	}
	mode := Mode(strings.ToLower(strings.TrimSpace(raw)))
	if mode == "" {
		return ModeFast, nil
	}
	switch mode {
	case ModeFast, ModeComplete, ModeProduction:
		return mode, nil
	default:
		return "", fmt.Errorf("unknown e2e mode %q; set %s or -e2e.mode to fast, complete, or production", raw, modeEnv)
	}
}

func resolveEVMChains(
	mode Mode,
	requirements EVMRequirements,
	ids []environment.ChainID,
) (evmResolution, error) {
	if err := validateEVMRequirements(requirements); err != nil {
		return evmResolution{}, err
	}
	if err := validateChainIDs(ids); err != nil {
		return evmResolution{}, err
	}

	var providers []EVMProvider
	switch mode {
	case ModeFast:
		providers = []EVMProvider{EVMProviderAnvil}
	case ModeComplete:
		providers = []EVMProvider{EVMProviderAnvil, EVMProviderBesu}
	case ModeProduction:
		providers = []EVMProvider{EVMProviderBesu, EVMProviderAnvil}
	default:
		return evmResolution{}, fmt.Errorf("unknown e2e mode %q", mode)
	}

	for _, provider := range providers {
		if requirements.Provider != "" && requirements.Provider != provider {
			continue
		}
		if (requirements.ControlledMining || requirements.NodeLifecycle) && provider != EVMProviderAnvil {
			continue
		}
		return evmResolution{chains: evmChainSpecs(provider, ids), provider: provider}, nil
	}

	reason := fmt.Sprintf("no EVM provider satisfies requirements %+v in %s mode", requirements, mode)
	if mode == ModeFast {
		return evmResolution{skipReason: reason}, nil
	}
	return evmResolution{}, errors.New(reason)
}

func validateEVMRequirements(requirements EVMRequirements) error {
	switch requirements.Provider {
	case "", EVMProviderAnvil, EVMProviderBesu:
		return nil
	default:
		return fmt.Errorf("unknown EVM provider %q", requirements.Provider)
	}
}

func validateChainIDs(ids []environment.ChainID) error {
	if len(ids) == 0 {
		return errors.New("at least one EVM chain ID is required")
	}
	seen := make(map[environment.ChainID]struct{}, len(ids))
	for _, id := range ids {
		if id == "" {
			return errors.New("EVM chain ID must not be empty")
		}
		if _, ok := seen[id]; ok {
			return fmt.Errorf("duplicate EVM chain ID %q", id)
		}
		seen[id] = struct{}{}
	}
	return nil
}

func evmChainSpecs(provider EVMProvider, ids []environment.ChainID) []environment.ChainSpec {
	chains := make([]environment.ChainSpec, len(ids))
	for i, id := range ids {
		switch provider {
		case EVMProviderAnvil:
			chains[i] = environment.ManagedAnvil{ID: id, EVMChainID: anvilChainIDBase + uint64(i)}
		case EVMProviderBesu:
			chains[i] = environment.ManagedBesu{ID: id, EVMChainID: besuChainIDBase + uint64(i)}
		}
	}
	return chains
}

func Start(t testing.TB, spec environment.Spec, runtime environment.Runtime) *environment.Environment {
	t.Helper()
	if matrixDiscoveryEnabled() {
		mode, err := resolveMode(*modeFlag, os.Getenv(modeEnv))
		if err != nil {
			t.Fatalf("e2etest: %v", err)
		}
		if err := environment.Validate(spec, runtime); err != nil {
			t.Fatalf("e2etest: validate Environment: %v", err)
		}
		recordResolvedSpec(t, mode, spec)
		t.SkipNow()
		return nil
	}
	env, err := environment.Start(t.Context(), spec, runtime)
	require.NoError(t, err, "e2etest: start Environment")
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), cleanupTimeout)
		defer cancel()
		assert.NoError(t, env.Close(ctx), "e2etest: close Environment")
	})
	return env
}

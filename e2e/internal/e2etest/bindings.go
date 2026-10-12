// SPDX-License-Identifier: Apache-2.0

package e2etest

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cosmos/ibc/e2e/internal/harness/environment"
	"github.com/cosmos/ibc/e2e/internal/harness/ibccli"
	"github.com/cosmos/ibc/e2e/internal/harness/traffic"
)

// This file holds the testing.TB wrappers e2e/README.md describes. Those that
// start resources register their cleanup, so t.Cleanup's LIFO order stops
// processes before their directories are removed and before the Environment
// closes.

// NewSigner generates a randomly-keyed local identity.
func NewSigner(t testing.TB) traffic.Signer {
	t.Helper()
	signer, err := traffic.NewSigner()
	require.NoError(t, err)
	return signer
}

// Deploy writes a temporary black-box configuration, runs its migration, and
// deploys the test applications on each Chain. It does not start the relayer.
func Deploy(
	t testing.TB,
	env *environment.Environment,
	deployer traffic.Signer,
	relayer traffic.Signer,
	routes ...traffic.Route,
) (*ibccli.Driver, *traffic.Deployment) {
	t.Helper()
	return DeployWithRelayerConfig(t, env, deployer, relayer, nil, routes...)
}

// DeployWithRelayerConfig is Deploy with a hook that adjusts the resolved
// relayer configuration after environment Attestors are assigned.
func DeployWithRelayerConfig(
	t testing.TB,
	env *environment.Environment,
	deployer traffic.Signer,
	relayer traffic.Signer,
	configure func(*ibccli.RelayerConfig),
	routes ...traffic.Route,
) (*ibccli.Driver, *traffic.Deployment) {
	t.Helper()
	driver, deployment, err := traffic.Deploy(
		t.Context(), env, t.TempDir(), deployer, relayer, configure, routes...,
	)
	require.NoError(t, err)
	return driver, deployment
}

// StartRelayer starts the test relayer and registers idempotent teardown.
// With E2E_DUMP enabled it dumps the relayer's directory after it stops.
func StartRelayer(t testing.TB, driver *ibccli.Driver, env *environment.Environment) *ibccli.Relayer {
	t.Helper()

	dumpRelayer := func() {
		if environment.DumpEnabled() && driver != nil && env != nil {
			environment.DumpDirectory(env.RunID(), driver.ConfigHome(), t.Name())
		}
	}

	relayer, err := traffic.StartRelayer(t.Context(), driver, env)
	if err != nil {
		dumpRelayer()
		t.Fatalf("e2etest: %v", err)
	}

	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), traffic.StopTimeout)
		defer cancel()
		assert.NoError(t, relayer.Stop(ctx), "e2etest: stop relayer")
		dumpRelayer()
	})
	return relayer
}

// StartProver writes the prover's config and runs it.
func StartProver(t testing.TB, env *environment.Environment, signer traffic.Signer) *ibccli.Prover {
	t.Helper()
	prover, err := traffic.StartProver(t.Context(), env, signer, t.TempDir())
	require.NoError(t, err)
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), traffic.StopTimeout)
		defer cancel()
		assert.NoError(t, prover.Stop(ctx), "e2etest: stop prover service")
	})
	return prover
}

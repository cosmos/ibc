// SPDX-License-Identifier: Apache-2.0

package lightclient

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

type testFactory string

func (f testFactory) Type() string { return string(f) }

func (testFactory) New(context.Context, FactoryOptions) (ProofGenerator, error) {
	return nil, nil
}

func TestRegistryRegisterUsesFactoryType(t *testing.T) {
	registry := NewRegistry()
	factory := testFactory("custom")
	require.NoError(t, registry.Register(factory))

	got, ok := registry.Get("custom")
	require.True(t, ok)
	require.Equal(t, ProverFactory(factory), got)
	require.ErrorContains(t, registry.Register(factory), "already registered")
}

func TestRegistryRejectsBuiltInAttestation(t *testing.T) {
	require.ErrorContains(t, NewRegistry().Register(testFactory("attestation")), "built in")
}

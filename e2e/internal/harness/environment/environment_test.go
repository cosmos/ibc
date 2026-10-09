// SPDX-License-Identifier: Apache-2.0

package environment

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestEnvironmentChainsReturnsStableCallerOwnedIDs(t *testing.T) {
	env := &Environment{chains: map[ChainID]*Chain{
		"charlie": {},
		"alpha":   {},
		"bravo":   {},
	}}

	ids := env.Chains()
	require.Equal(t, []ChainID{"alpha", "bravo", "charlie"}, ids)

	ids[0] = "changed-by-caller"
	require.Equal(t, []ChainID{"alpha", "bravo", "charlie"}, env.Chains())
}

func TestEnvironmentAttestorsReturnsStableCallerOwnedIDs(t *testing.T) {
	env := &Environment{attestors: map[AttestorID]*Attestor{
		"charlie": {},
		"alpha":   {},
		"bravo":   {},
	}}

	ids := env.Attestors()
	require.Equal(t, []AttestorID{"alpha", "bravo", "charlie"}, ids)

	ids[0] = "changed-by-caller"
	require.Equal(t, []AttestorID{"alpha", "bravo", "charlie"}, env.Attestors())
}

// SPDX-License-Identifier: Apache-2.0

package e2etest

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/cosmos/ibc/harness/environment"
)

func TestResolveMode(t *testing.T) {
	tests := []struct {
		name      string
		flagValue string
		envValue  string
		want      Mode
		wantErr   bool
	}{
		{name: "default", want: ModeFast},
		{name: "environment", envValue: " complete ", want: ModeComplete},
		{name: "normalized", envValue: " PrOdUcTiOn ", want: ModeProduction},
		{name: "flag overrides environment", flagValue: " COMPLETE ", envValue: "production", want: ModeComplete},
		{name: "blank flag uses environment", flagValue: "  ", envValue: "production", want: ModeProduction},
		{name: "invalid environment", envValue: "slow", wantErr: true},
		{name: "invalid override", flagValue: "slow", envValue: "fast", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := resolveMode(tt.flagValue, tt.envValue)
			if (err != nil) != tt.wantErr {
				t.Fatalf("resolveMode() error = %v, wantErr %t", err, tt.wantErr)
			}
			if got != tt.want {
				t.Fatalf("resolveMode() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestResolveEVMChains(t *testing.T) {
	tests := []struct {
		name         string
		mode         Mode
		requirements EVMRequirements
		wantProvider environment.EVMProvider
		wantSkip     bool
		wantErr      bool
	}{
		{name: "fast portable", mode: ModeFast, wantProvider: environment.EVMProviderAnvil},
		{name: "complete portable", mode: ModeComplete, wantProvider: environment.EVMProviderAnvil},
		{name: "production portable", mode: ModeProduction, wantProvider: environment.EVMProviderBesu},
		{
			name:         "fast controlled mining",
			mode:         ModeFast,
			requirements: EVMRequirements{ControlledMining: true},
			wantProvider: environment.EVMProviderAnvil,
		},
		{
			name:         "complete controlled mining",
			mode:         ModeComplete,
			requirements: EVMRequirements{ControlledMining: true},
			wantProvider: environment.EVMProviderAnvil,
		},
		{
			name:         "production controlled mining",
			mode:         ModeProduction,
			requirements: EVMRequirements{ControlledMining: true},
			wantProvider: environment.EVMProviderAnvil,
		},
		{
			name:         "fast node lifecycle",
			mode:         ModeFast,
			requirements: EVMRequirements{NodeLifecycle: true},
			wantProvider: environment.EVMProviderAnvil,
		},
		{
			name:         "complete node lifecycle",
			mode:         ModeComplete,
			requirements: EVMRequirements{NodeLifecycle: true},
			wantProvider: environment.EVMProviderAnvil,
		},
		{
			name:         "production node lifecycle",
			mode:         ModeProduction,
			requirements: EVMRequirements{NodeLifecycle: true},
			wantProvider: environment.EVMProviderAnvil,
		},
		{
			name:         "fast exact anvil",
			mode:         ModeFast,
			requirements: EVMRequirements{Provider: environment.EVMProviderAnvil},
			wantProvider: environment.EVMProviderAnvil,
		},
		{
			name:         "complete exact anvil",
			mode:         ModeComplete,
			requirements: EVMRequirements{Provider: environment.EVMProviderAnvil},
			wantProvider: environment.EVMProviderAnvil,
		},
		{
			name:         "production exact anvil",
			mode:         ModeProduction,
			requirements: EVMRequirements{Provider: environment.EVMProviderAnvil},
			wantProvider: environment.EVMProviderAnvil,
		},
		{
			name:         "fast exact besu skips",
			mode:         ModeFast,
			requirements: EVMRequirements{Provider: environment.EVMProviderBesu},
			wantSkip:     true,
		},
		{
			name:         "complete exact besu",
			mode:         ModeComplete,
			requirements: EVMRequirements{Provider: environment.EVMProviderBesu},
			wantProvider: environment.EVMProviderBesu,
		},
		{
			name:         "production exact besu",
			mode:         ModeProduction,
			requirements: EVMRequirements{Provider: environment.EVMProviderBesu},
			wantProvider: environment.EVMProviderBesu,
		},
		{
			name:         "fast incompatible skips",
			mode:         ModeFast,
			requirements: EVMRequirements{Provider: environment.EVMProviderBesu, ControlledMining: true},
			wantSkip:     true,
		},
		{
			name:         "complete incompatible fails",
			mode:         ModeComplete,
			requirements: EVMRequirements{Provider: environment.EVMProviderBesu, ControlledMining: true},
			wantErr:      true,
		},
		{
			name:         "production incompatible fails",
			mode:         ModeProduction,
			requirements: EVMRequirements{Provider: environment.EVMProviderBesu, NodeLifecycle: true},
			wantErr:      true,
		},
		{name: "unknown mode", mode: "other", wantErr: true},
		{name: "unknown provider", mode: ModeFast, requirements: EVMRequirements{Provider: "other"}, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := resolveEVMChains(tt.mode, tt.requirements, []environment.ChainID{ChainA})
			if (err != nil) != tt.wantErr {
				t.Fatalf("resolveEVMChains() error = %v, wantErr %t", err, tt.wantErr)
			}
			if (got.skipReason != "") != tt.wantSkip {
				t.Fatalf("resolveEVMChains() skip reason = %q, wantSkip %t", got.skipReason, tt.wantSkip)
			}
			if tt.wantProvider != "" && providerOf(t, got.chains[0]) != tt.wantProvider {
				t.Fatalf("resolveEVMChains() provider = %q, want %q", providerOf(t, got.chains[0]), tt.wantProvider)
			}
		})
	}
}

func TestResolveEVMChainsIDs(t *testing.T) {
	for _, ids := range [][]environment.ChainID{{"one"}, {"two", "one"}, {"three", "one", "two"}} {
		got, err := resolveEVMChains(ModeProduction, EVMRequirements{}, ids)
		if err != nil {
			t.Fatalf("resolveEVMChains(%v): %v", ids, err)
		}
		want := make([]environment.ChainSpec, len(ids))
		for i, id := range ids {
			want[i] = environment.ManagedBesu{ID: id, EVMChainID: 32337 + uint64(i)}
		}
		require.Equal(t, want, got.chains)
	}
}

func TestResolveEVMChainsInvalidIDs(t *testing.T) {
	for _, ids := range [][]environment.ChainID{nil, {""}, {"same", "same"}} {
		if _, err := resolveEVMChains(ModeFast, EVMRequirements{}, ids); err == nil {
			t.Fatalf("resolveEVMChains(%v) succeeded", ids)
		}
	}
}

func providerOf(t *testing.T, chain environment.ChainSpec) environment.EVMProvider {
	t.Helper()
	switch chain.(type) {
	case environment.ManagedAnvil:
		return environment.EVMProviderAnvil
	case environment.ManagedBesu:
		return environment.EVMProviderBesu
	default:
		t.Fatalf("unexpected chain type %T", chain)
		return ""
	}
}

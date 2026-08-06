package e2etest

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/cosmos/ibc/e2e/internal/harness/environment"
)

func TestRuntimeWithProtocolDeployer(t *testing.T) {
	input := environment.Runtime{
		Endpoints: map[environment.EndpointBindingID]environment.EndpointBinding{
			"rpc": {RPCURL: "http://example.test"},
		},
		Authorities: map[environment.AuthorityID]environment.EVMAuthority{
			ProtocolAuthorityID: {PrivateKeyHex: "old"},
			"other":             {PrivateKeyHex: "other-key"},
		},
	}
	got := RuntimeWithProtocolDeployer(input)
	got.Endpoints["new"] = environment.EndpointBinding{RPCURL: "http://new.test"}
	got.Authorities["new"] = environment.EVMAuthority{PrivateKeyHex: "new-key"}

	require.Equal(t, protocolAuthorityKeyHex, got.Authorities[ProtocolAuthorityID].PrivateKeyHex)
	require.Equal(t, "old", input.Authorities[ProtocolAuthorityID].PrivateKeyHex)
	require.Empty(t, input.Authorities["new"].PrivateKeyHex)
	require.Empty(t, input.Endpoints["new"].RPCURL)
	require.Equal(t, input.Endpoints["rpc"], got.Endpoints["rpc"])
}

func TestChainSpecsForConfiguredLane(t *testing.T) {
	originalFlag := *laneFlag
	t.Cleanup(func() { *laneFlag = originalFlag })

	tests := []struct {
		name string
		flag string
		env  string
		want []environment.ChainSpec
	}{
		{
			name: "default anvil", want: anvilChainSpecs(anvilChainIDBase, 0),
		},
		{
			name: "anvil interval", env: laneAnvilInterval,
			want: anvilChainSpecs(anvilIntervalChainIDBase, anvilIntervalBlockTime),
		},
		{
			name: "besu", env: laneBesu,
			want: []environment.ChainSpec{
				environment.ManagedBesu{ID: ChainA, EVMChainID: besuChainIDBase},
				environment.ManagedBesu{ID: ChainB, EVMChainID: besuChainIDBase + 1},
			},
		},
		{
			name: "flag overrides environment", flag: laneAnvilInterval, env: laneBesu,
			want: anvilChainSpecs(anvilIntervalChainIDBase, anvilIntervalBlockTime),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			*laneFlag = tt.flag
			t.Setenv(laneEnv, tt.env)
			require.Equal(t, tt.want, ChainSpecsForConfiguredLane(t))
		})
	}
}

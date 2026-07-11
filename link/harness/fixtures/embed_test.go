package fixtures

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEmbeddedArtifacts(t *testing.T) {
	for _, c := range []Contract{Counter, FixtureDeployer, MockGMP, MockIFT} {
		t.Run(c.Name, func(t *testing.T) {
			require.NotEmpty(t, c.Bytecode, "embedded bytecode must be non-empty")

			_, err := c.ParsedABI()
			require.NoError(t, err, "embedded ABI must parse")
		})
	}
}

func TestExpectedFixtureSurface(t *testing.T) {
	cases := []struct {
		c       Contract
		methods []string
	}{
		{Counter, []string{"count", "increment"}},
		{MockGMP, []string{"send", "deliver"}},
		{MockIFT, []string{"balanceOf", "mint", "sendTransfer", "receiveTransfer"}},
	}
	for _, tc := range cases {
		parsed, err := tc.c.ParsedABI()
		require.NoError(t, err, tc.c.Name)
		for _, m := range tc.methods {
			assert.Contains(t, parsed.Methods, m, "%s must expose method %s", tc.c.Name, m)
		}
	}

	gmp, err := MockGMP.ParsedABI()
	require.NoError(t, err)
	assert.Contains(t, gmp.Events, "GMPSent")
	assert.Contains(t, gmp.Events, "GMPReceived")

	ift, err := MockIFT.ParsedABI()
	require.NoError(t, err)
	assert.Contains(t, ift.Events, "IFTSent")
	assert.Contains(t, ift.Events, "IFTReceived")
	assert.Empty(t, ift.Constructor.Inputs)

	deployer, err := FixtureDeployer.ParsedABI()
	require.NoError(t, err)
	assert.Empty(t, deployer.Methods)
	require.Len(t, deployer.Constructor.Inputs, 1)
	event, ok := deployer.Events["FixturesDeployed"]
	require.True(t, ok)
	require.Len(t, event.Inputs, 3)
	for _, input := range event.Inputs {
		assert.Equal(t, "address", input.Type.String())
	}
}

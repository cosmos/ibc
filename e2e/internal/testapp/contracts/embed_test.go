package contracts

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEmbeddedArtifacts(t *testing.T) {
	for _, contract := range []Contract{Counter, TestAppDeployer, MockGMP, MockIFT} {
		t.Run(contract.Name, func(t *testing.T) {
			require.NotEmpty(t, contract.Bytecode)
			_, err := contract.ParsedABI()
			require.NoError(t, err)
		})
	}
}

func TestExpectedContractSurface(t *testing.T) {
	cases := []struct {
		contract Contract
		methods  []string
	}{
		{Counter, []string{"count", "increment"}},
		{MockGMP, []string{"send", "deliver"}},
		{MockIFT, []string{"balanceOf", "mint", "sendTransfer", "receiveTransfer"}},
	}
	for _, tc := range cases {
		parsed, err := tc.contract.ParsedABI()
		require.NoError(t, err, tc.contract.Name)
		for _, method := range tc.methods {
			assert.Contains(t, parsed.Methods, method, "%s must expose method %s", tc.contract.Name, method)
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

	deployer, err := TestAppDeployer.ParsedABI()
	require.NoError(t, err)
	assert.Empty(t, deployer.Methods)
	require.Len(t, deployer.Constructor.Inputs, 1)
	event, ok := deployer.Events["TestAppsDeployed"]
	require.True(t, ok)
	require.Len(t, event.Inputs, 3)
	for _, input := range event.Inputs {
		assert.Equal(t, "address", input.Type.String())
	}
}

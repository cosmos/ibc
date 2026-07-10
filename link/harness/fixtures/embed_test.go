package fixtures

import (
	"encoding/hex"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestEmbeddedArtifacts is the every-PR fast-lane guard (no forge, no chain): it proves
// the committed forge artifacts are present and well-formed, so a stale/missing rebuild is caught as a
// unit-test failure rather than an opaque deploy error in an e2e run.
func TestEmbeddedArtifacts(t *testing.T) {
	for _, c := range []Contract{Counter, FixtureDeployer, MockGMP, MockIFT} {
		t.Run(c.Name, func(t *testing.T) {
			// Bytecode must be real creation code, not an empty/placeholder object.
			require.NotEmpty(t, c.Bytecode, "embedded bytecode must be non-empty")

			// The ABI must parse as a go-ethereum ABI — this is what M3/M4 rely on to encode calls
			// and decode the IFT/GMP events.
			_, err := c.ParsedABI()
			require.NoError(t, err, "embedded ABI must parse")
		})
	}
}

// TestExpectedFixtureSurface pins the handful of ABI members the harness will drive, so a fixture
// edit that drops or renames one of them fails here instead of deep in an e2e correlation step.
func TestExpectedFixtureSurface(t *testing.T) {
	// Contract holds a []byte, so it is not comparable and cannot be a map key — use a slice.
	cases := []struct {
		c       Contract
		methods []string
	}{
		{Counter, []string{"count", "increment"}},
		{MockGMP, []string{"send", "deliver", "deliverIFT", "deliveryClientId"}},
		{MockIFT, []string{"balanceOf", "mint", "sendTransfer", "receiveTransfer", "iftMint"}},
	}
	for _, tc := range cases {
		parsed, err := tc.c.ParsedABI()
		require.NoError(t, err, tc.c.Name)
		for _, m := range tc.methods {
			assert.Contains(t, parsed.Methods, m, "%s must expose method %s", tc.c.Name, m)
		}
	}

	// The events the correlator decodes in M3/M4 must exist on the ABI.
	gmp, err := MockGMP.ParsedABI()
	require.NoError(t, err)
	assert.Contains(t, gmp.Events, "GMPSent")
	assert.Contains(t, gmp.Events, "GMPReceived")

	ift, err := MockIFT.ParsedABI()
	require.NoError(t, err)
	assert.Contains(t, ift.Events, "IFTSent")
	assert.Contains(t, ift.Events, "IFTReceived")
	assert.Contains(t, ift.Events, "IFTMintReceived")
	assert.Equal(t, "0a7244e7", hex.EncodeToString(ift.Methods["iftMint"].ID))
	assert.True(t, ift.Events["IFTMintReceived"].Inputs[1].Indexed)
	assert.Equal(t, "address", ift.Constructor.Inputs[0].Type.String())

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

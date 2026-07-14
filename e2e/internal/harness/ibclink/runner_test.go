package ibclink

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/cosmos/ibc/e2e/internal/harness/ibclink/wire"
)

func TestDecodeTestAppDeploymentPreservesPartialReceiptsOnFailure(t *testing.T) {
	result := &result{
		code: wire.ExitTestAppDeployFailure,
		stdout: []byte(`{
  "chains": {"chain-a": {"counter": "0x1", "txHash": "0xreceipt"}}
}`),
		stderr: "chain-b failed",
	}

	deployment, err := decodeTestAppDeploymentResult(result)
	require.ErrorIs(t, err, ErrTestAppDeployFailed)
	require.NotNil(t, deployment)
	require.Equal(t, "0xreceipt", deployment.Chains["chain-a"].TxHash)
	require.Equal(t, "0x1", deployment.Chains["chain-a"].Counter)
}

func TestDecodeTestAppDeploymentRequiresJSONOnSuccess(t *testing.T) {
	deployment, err := decodeTestAppDeploymentResult(&result{code: wire.ExitOK, stdout: []byte("not-json")})
	require.Nil(t, deployment)
	require.ErrorContains(t, err, "stdout is not a TestAppDeployment")
}

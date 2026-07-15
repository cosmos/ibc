package testappcmd

import (
	"encoding/json"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"
)

func TestCommandRunsInjectedDeployHandler(t *testing.T) {
	called := false
	cmd := NewCommand(func(_ *cobra.Command, _ []string) error {
		called = true
		return nil
	})
	cmd.SetArgs([]string{"deploy"})

	require.NoError(t, cmd.Execute())
	require.True(t, called)
}

func TestDeploymentJSONSerialization(t *testing.T) {
	deployment := Deployment{Chains: map[string]ChainDeployment{
		"chain-a": {MockIFT: "0xift", MockGMP: "0xgmp", Counter: "0xcounter", TxHash: "0xtx"},
	}}

	encoded, err := json.Marshal(deployment)
	require.NoError(t, err)
	require.JSONEq(
		t,
		`{"chains":{"chain-a":{"mockIFT":"0xift","mockGMP":"0xgmp","counter":"0xcounter","txHash":"0xtx"}}}`,
		string(encoded),
	)
}

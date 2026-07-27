package relayercmd

import (
	"encoding/json"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"
)

func TestCommandRunsInjectedRunHandler(t *testing.T) {
	called := false
	cmd := NewCommand(func(_ *cobra.Command, _ []string) error {
		called = true
		return nil
	})
	cmd.SetArgs([]string{"run"})

	require.NoError(t, cmd.Execute())
	require.True(t, called)
}

func TestTransportJSONSerialization(t *testing.T) {
	readiness := Readiness{
		Event: ReadinessEvent, ChainsConnected: []string{"chain-a"}, HTTP: "127.0.0.1:1234",
	}
	encoded, err := json.Marshal(readiness)
	require.NoError(t, err)
	require.JSONEq(
		t,
		`{"event":"ready","chainsConnected":["chain-a"],"http":"127.0.0.1:1234"}`,
		string(encoded),
	)
}

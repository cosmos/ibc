package relayercmd

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

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

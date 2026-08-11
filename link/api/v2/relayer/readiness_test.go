package relayer_test

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/cosmos/ibc/link/api/v2/relayer"
)

func TestProcessReadiness(t *testing.T) {
	t.Parallel()

	encoded, err := json.Marshal(relayer.ProcessReadiness{
		Event:           relayer.ProcessReadinessEvent,
		ChainsConnected: []string{"chain-a"},
		HTTP:            "127.0.0.1:3000",
	})
	require.NoError(t, err)
	// the e2e harness reads readiness as one exact stdout line; the encoding must stay compact
	//nolint:testifylint // JSONEq would not catch a switch to indented output
	require.Equal(t, `{"event":"ready","chainsConnected":["chain-a"],"http":"127.0.0.1:3000"}`, string(encoded))
}

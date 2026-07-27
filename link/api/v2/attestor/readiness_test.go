package attestor_test

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/cosmos/ibc/link/api/v2/attestor"
)

func TestProcessReadiness(t *testing.T) {
	t.Parallel()

	require.Equal(t, "ready", attestor.ProcessReadinessEvent)

	encoded, err := json.Marshal(attestor.ProcessReadiness{
		Event: attestor.ProcessReadinessEvent,
		HTTP:  "127.0.0.1:3000",
	})
	require.NoError(t, err)
	require.Equal(t, `{"event":"ready","http":"127.0.0.1:3000"}`, string(encoded))
}

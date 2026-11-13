// SPDX-License-Identifier: Apache-2.0

package attestor_test

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/lfdt-ibc/ibc/cli/api/v2/attestor"
)

func TestProcessReadiness(t *testing.T) {
	t.Parallel()

	encoded, err := json.Marshal(attestor.ProcessReadiness{
		Event: attestor.ProcessReadinessEvent,
		HTTP:  "127.0.0.1:3000",
	})
	require.NoError(t, err)
	// the e2e harness parses these field names from the JSON log line on stderr
	//nolint:testifylint // pin the exact wire format, not just JSON equivalence
	require.Equal(t, `{"event":"ready","http":"127.0.0.1:3000"}`, string(encoded))
}

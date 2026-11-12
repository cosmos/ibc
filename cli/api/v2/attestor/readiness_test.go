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
	// the e2e harness parses readiness from one JSON log line on stderr; the encoding must stay compact
	//nolint:testifylint // JSONEq would not catch a switch to indented output
	require.Equal(t, `{"event":"ready","http":"127.0.0.1:3000"}`, string(encoded))
}

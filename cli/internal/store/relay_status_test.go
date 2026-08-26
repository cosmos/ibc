// SPDX-License-Identifier: Apache-2.0

package store

import (
	"os"
	"regexp"
	"testing"

	"github.com/stretchr/testify/require"
)

// AllRelayStatuses is hand-maintained beside the constants, so nothing stops a
// new status being declared and left out of it. A status missing there is
// excluded from every derived grouping — including the "no filter" listing —
// so a packet in that status becomes invisible rather than erroring.
func TestAllRelayStatusesIsComplete(t *testing.T) {
	t.Parallel()

	source, err := os.ReadFile("store.go")
	require.NoError(t, err)

	declared := regexp.MustCompile(`RelayStatus\w+\s+RelayStatus = "([A-Z_]+)"`).
		FindAllStringSubmatch(string(source), -1)
	require.NotEmpty(t, declared, "found no status constants; has the declaration form changed?")

	all := AllRelayStatuses()
	for _, match := range declared {
		require.Containsf(t, all, RelayStatus(match[1]),
			"%s is declared but missing from AllRelayStatuses", match[1])
	}

	require.Len(t, all, len(declared), "AllRelayStatuses lists a status that is not declared")
}

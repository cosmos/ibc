// SPDX-License-Identifier: Apache-2.0

package relayer

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// FuzzDecodeCursor feeds client-supplied pagination cursors to decodeCursor.
func FuzzDecodeCursor(f *testing.F) {
	f.Add("", int64(1))
	f.Add(encodeCursor(4096), int64(4096))
	f.Add("not-base64!", int64(-1))

	f.Fuzz(func(t *testing.T, cursor string, id int64) {
		if decoded, err := decodeCursor(cursor); err == nil && cursor != "" {
			require.Positive(t, decoded, "cursor %q", cursor)
		}

		if id <= 0 {
			return
		}
		got, err := decodeCursor(encodeCursor(id))
		require.NoError(t, err)
		require.Equal(t, id, got)
	})
}

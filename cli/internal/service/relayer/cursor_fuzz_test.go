// SPDX-License-Identifier: Apache-2.0

package relayer

import "testing"

// FuzzDecodeCursor feeds client-supplied pagination cursors to decodeCursor.
func FuzzDecodeCursor(f *testing.F) {
	f.Add("", int64(1))
	f.Add(encodeCursor(4096), int64(4096))
	f.Add("not-base64!", int64(-1))

	f.Fuzz(func(t *testing.T, cursor string, id int64) {
		decoded, err := decodeCursor(cursor)
		if err == nil && cursor != "" && decoded <= 0 {
			t.Fatalf("cursor %q decoded to non-positive id %d", cursor, decoded)
		}

		if id <= 0 {
			return
		}
		if got, err := decodeCursor(encodeCursor(id)); err != nil || got != id {
			t.Fatalf("round trip of %d gave (%d, %v)", id, got, err)
		}
	})
}

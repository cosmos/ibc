// SPDX-License-Identifier: Apache-2.0

package relayer

import (
	"encoding/base64"
	"strconv"

	"github.com/pkg/errors"
)

// Cursors are opaque so the paging position stays an implementation detail:
// callers that cannot read one cannot come to depend on the sort key.

func encodeCursor(id int64) string {
	return base64.RawURLEncoding.EncodeToString([]byte(strconv.FormatInt(id, 10)))
}

// decodeCursor returns the exclusive id bound a cursor names. An empty cursor
// yields zero, which Page treats as unbounded.
func decodeCursor(cursor string) (int64, error) {
	if cursor == "" {
		return 0, nil
	}

	raw, err := base64.RawURLEncoding.DecodeString(cursor)
	if err != nil {
		return 0, errors.Wrap(ErrInvalidInput, "cursor is malformed")
	}

	id, err := strconv.ParseInt(string(raw), 10, 64)
	if err != nil || id <= 0 {
		return 0, errors.Wrap(ErrInvalidInput, "cursor is malformed")
	}

	return id, nil
}

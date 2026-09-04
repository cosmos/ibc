// SPDX-License-Identifier: Apache-2.0

package logging

import (
	"context"
	"errors"
	"log/slog"
	"testing"
	"time"

	pkgerrors "github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
)

func TestDefaultLevel(t *testing.T) {
	for _, tt := range []struct {
		name            string
		inputLevel      string
		configuredLevel slog.Level
	}{
		{"debug", "debug", slog.LevelDebug},
		{"info", "info", slog.LevelInfo},
		{"Capital warn", "WARN", slog.LevelWarn},
		{"empty input defaults to info", "", slog.LevelInfo},     // empty falls back to info
		{"invalid defaults to info", "nonsense", slog.LevelInfo}, // unrecognized falls back to info
	} {
		t.Run(tt.name, func(t *testing.T) {
			logger := Default(false, tt.inputLevel)
			assert.True(t, logger.Enabled(context.Background(), tt.configuredLevel))
		})
	}
}

func TestReplaceAttrs(t *testing.T) {
	t.Run("arbitraryAttr", func(t *testing.T) {
		// ARRANGE
		attr := slog.String("status", "failed")

		// ACT
		actual := ReplaceAttrs(nil, attr)

		// ASSERT
		assert.Equal(t, attr, actual)
	})

	t.Run("errors", func(t *testing.T) {
		for _, tt := range []struct {
			name     string
			attr     slog.Attr
			expected slog.Attr
		}{
			{
				name:     "errKey",
				attr:     slog.Any("err", pkgerrors.WithStack(errors.New("boom"))),
				expected: slog.String("err", "boom"),
			},
			{
				name:     "errorKey",
				attr:     slog.Any("error", pkgerrors.WithStack(errors.New("boom"))),
				expected: slog.String("err", "boom"),
			},
			{
				name:     "errorKeyWithNonErrorValue",
				attr:     slog.String("error", "boom"),
				expected: slog.String("error", "boom"),
			},
		} {
			t.Run(tt.name, func(t *testing.T) {
				// ARRANGE
				attr := tt.attr

				// ACT
				actual := ReplaceAttrs(nil, attr)

				// ASSERT
				assert.Equal(t, tt.expected, actual)
			})
		}
	})

	t.Run("time", func(t *testing.T) {
		t.Run("convertsToUTC", func(t *testing.T) {
			// ARRANGE
			timestamp := time.Date(2026, time.August, 21, 13, 35, 36, 0, time.FixedZone("UTC+2", 2*60*60))

			// ACT
			actual := ReplaceAttrs(nil, slog.Time(slog.TimeKey, timestamp))

			// ASSERT
			assert.Equal(t, time.UTC, actual.Value.Time().Location())
			assert.Equal(t, timestamp.UTC(), actual.Value.Time())
		})

		t.Run("leavesNonTimeValueUnchanged", func(t *testing.T) {
			// ARRANGE
			attr := slog.String(slog.TimeKey, "2026-08-21T11:35:36Z")

			// ACT
			actual := ReplaceAttrs(nil, attr)

			// ASSERT
			assert.Equal(t, attr, actual)
		})
	})
}

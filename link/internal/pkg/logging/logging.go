// SPDX-License-Identifier: Apache-2.0

package logging

import (
	"log/slog"
	"os"
	"time"
)

const timeFormat = time.RFC3339

func Default(json bool) *slog.Logger {
	opts := &slog.HandlerOptions{
		ReplaceAttr: ReplaceAttrs,
	}

	var handler slog.Handler
	if json {
		handler = slog.NewJSONHandler(os.Stderr, opts)
	} else {
		handler = slog.NewTextHandler(os.Stderr, opts)
	}

	return slog.New(handler)
}

// ReplaceAttrs normalizes errors before they reach a slog handler.
func ReplaceAttrs(_ []string, attr slog.Attr) slog.Attr {
	switch attr.Key {
	case "err", "error":
		if err, ok := attr.Value.Any().(error); ok && err != nil {
			return slog.String("err", err.Error())
		}
	case slog.TimeKey:
		t := attr.Value.Time().UTC()
		return slog.String("time", t.Format(timeFormat))
	}

	return attr
}

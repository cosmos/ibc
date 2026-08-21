// SPDX-License-Identifier: Apache-2.0

package logging

import (
	"log/slog"
	"os"
	"time"
)

func Default(json bool) *slog.Logger {
	opts := &slog.HandlerOptions{ReplaceAttr: ReplaceAttrs}

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
	case slog.TimeKey:
		// enforce UTC timestamp
		if t, ok := attr.Value.Any().(time.Time); ok {
			return slog.Time(attr.Key, t.UTC())
		}
	case "err", "error":
		// normalize error w/o stack trace
		if err, ok := attr.Value.Any().(error); ok && err != nil {
			return slog.String("err", err.Error())
		}
	}

	return attr
}

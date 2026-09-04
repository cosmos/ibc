// SPDX-License-Identifier: Apache-2.0

package logging

import (
	"log/slog"
	"os"
	"time"
)

// ParseLevel parses a slog level name (debug, info, warn, error;
// case-insensitive). Empty defaults to info.
func ParseLevel(level string) (slog.Level, error) {
	if level == "" {
		return slog.LevelInfo, nil
	}

	var lvl slog.Level
	err := lvl.UnmarshalText([]byte(level))

	return lvl, err
}

func Default(json bool, level string) *slog.Logger {
	lvl, err := ParseLevel(level)
	if err != nil {
		lvl = slog.LevelInfo // unrecognized level falls back to info
	}

	opts := &slog.HandlerOptions{Level: lvl, ReplaceAttr: ReplaceAttrs}

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

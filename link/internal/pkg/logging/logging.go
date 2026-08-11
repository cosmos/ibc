// SPDX-License-Identifier: Apache-2.0

package logging

import (
	"log/slog"
	"os"
)

func Default(json bool) *slog.Logger {
	opts := &slog.HandlerOptions{
		ReplaceAttr: ReplaceErrorAttr,
	}

	var handler slog.Handler
	if json {
		handler = slog.NewJSONHandler(os.Stderr, opts)
	} else {
		handler = slog.NewTextHandler(os.Stderr, opts)
	}

	return slog.New(handler)
}

// ReplaceErrorAttr normalizes errors before they reach a slog handler.
func ReplaceErrorAttr(_ []string, attr slog.Attr) slog.Attr {
	if attr.Key != "err" {
		return attr
	}

	if err, ok := attr.Value.Any().(error); ok && err != nil {
		return slog.String("err", err.Error())
	}

	return attr
}

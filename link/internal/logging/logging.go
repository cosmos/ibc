// Package logging provides logging functionality.
package logging

import (
	"os"

	"github.com/rs/zerolog"
)

// DefaultLogger returns a default logger with INFO level.
func DefaultLogger() zerolog.Logger {
	return zerolog.New(os.Stdout).
		Level(zerolog.InfoLevel).
		With().
		Timestamp().
		Logger()
}

// WithComponent returns a logger with the given component name.
func WithComponent(logger zerolog.Logger, component string) zerolog.Logger {
	return logger.With().Str("component", component).Logger()
}

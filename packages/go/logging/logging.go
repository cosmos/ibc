package logging

import (
	"os"

	"github.com/rs/zerolog"
)

func DefaultLogger() zerolog.Logger {
	return zerolog.New(os.Stdout).
		Level(zerolog.InfoLevel).
		With().
		Timestamp().
		Logger()
}

func WithComponent(logger zerolog.Logger, component string) zerolog.Logger {
	return logger.With().Str("component", component).Logger()
}

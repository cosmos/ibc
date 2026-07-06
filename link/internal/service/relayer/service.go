package relayer

import (
	"context"
	"log/slog"

	"github.com/pkg/errors"
)

// Service represents relayer business logic.
type Service struct {
	logger *slog.Logger
}

// New Service constructor.
func New() *Service {
	return &Service{
		logger: slog.With("service", "relayer"),
	}
}

// Relay errors
var (
	ErrInvalidInput = errors.New("invalid input")
)

func (s *Service) Relay(_ context.Context, chainID, txHash string) error {
	switch {
	case chainID == "":
		return errors.Wrap(ErrInvalidInput, "chainID is required")
	case txHash == "":
		return errors.Wrap(ErrInvalidInput, "txHash is required")
	}

	s.logger.Info("Relaying transaction", "chainID", chainID, "txHash", txHash)

	// todo

	return nil
}

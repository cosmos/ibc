package relayer

import (
	"context"
	"log/slog"

	"github.com/pkg/errors"
)

type Service struct {
	logger *slog.Logger
}

func New() *Service {
	return &Service{
		logger: slog.With("service", "relayer"),
	}
}

var (
	ErrInvalidInput = errors.New("invalid input")
)

func (s *Service) Relay(ctx context.Context, chainID, txHash string) error {
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

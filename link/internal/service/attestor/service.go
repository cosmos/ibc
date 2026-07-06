package attestor

import (
	"context"
	"errors"
	"log/slog"
)

// Service manages configured attestors.
type Service struct {
	attestors map[string]Attestor
	logger    *slog.Logger
}

// Attestor reports attestation state.
type Attestor interface {
	LatestAttestableHeight(ctx context.Context) (uint64, error)
}

// Attestor errors
var (
	ErrNotFound = errors.New("attestor not found")
)

// New Service constructor.
func New() *Service {
	return &Service{
		logger: slog.With("service", "attestors"),
	}
}

// Add adds an attestor to the service. Not thread-safe.
func (s *Service) Add(id string, attestor Attestor) {
	s.attestors[id] = attestor
}

func (s *Service) LatestAttestableHeight(ctx context.Context, id string) (uint64, error) {
	attestor, ok := s.attestors[id]
	if !ok {
		return 0, ErrNotFound
	}

	return attestor.LatestAttestableHeight(ctx)
}

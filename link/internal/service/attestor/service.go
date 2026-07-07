package attestor

import (
	"context"
	"errors"
	"log/slog"

	"github.com/cosmos/ibc/link/internal/config"
)

// Service manages configured attestors.
type Service struct {
	attestors map[string]Attestor
	logger    *slog.Logger
}

// Attestor reports attestation state.
type Attestor interface {
	LatestAttestableHeight(ctx context.Context) (uint64, error)
	Name() string
}

// Attestor errors
var (
	ErrNotFound       = errors.New("attestor not found")
	ErrNoAttestations = errors.New("no attestations provided")
)

func NewFromConfig(cfg config.Config) (*Service, error) {
	if len(cfg.Attestor.Attestations) == 0 {
		return nil, ErrNoAttestations
	}

	attestorsSpecs := make([]Attestor, 0, len(cfg.Attestor.Attestations))
	for _, spec := range cfg.Attestor.Attestations {
		localAttestor := NewLocal(spec.ChainID, spec.Name)
		attestorsSpecs = append(attestorsSpecs, localAttestor)
	}

	return New(attestorsSpecs), nil
}

// New Service constructor.
func New(attestors []Attestor) *Service {
	attestorsMap := make(map[string]Attestor)
	for _, attestor := range attestors {
		attestorsMap[attestor.Name()] = attestor
	}

	return &Service{
		logger:    slog.With("service", "attestors"),
		attestors: attestorsMap,
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

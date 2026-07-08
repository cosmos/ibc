package attestor

import (
	"context"
	"errors"
	"fmt"
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
	// ChainID returns the chain ID.
	ChainID() string

	// Name is the name of the attestor. It is NOT unique across the service
	// Imagine there are 5 remote attestors and inside each they announce same `eth-1-attestor` name
	Name() string

	// Alias is the internal unique name of the attestors within THIS process.
	// Should be unique.
	Alias() string

	// IsLocal returns true if the attestor is local.
	IsLocal() bool

	LatestAttestableHeight(ctx context.Context) (uint64, error)
}

// Attestor errors
var (
	ErrNotFound       = errors.New("attestor not found")
	ErrNoAttestations = errors.New("no attestations provided")
)

// NewFromConfig creates a new attestor service from the configuration.
// Because config represents our local binary, ALL attestors are local.
func NewFromConfig(cfg config.Config) (*Service, error) {
	if len(cfg.Attestor.Attestations) == 0 {
		return nil, ErrNoAttestations
	}

	attestorsSpecs := make([]Attestor, 0, len(cfg.Attestor.Attestations))
	for _, spec := range cfg.Attestor.Attestations {
		localAttestor := NewLocal(spec.ChainID, spec.Name)
		attestorsSpecs = append(attestorsSpecs, localAttestor)
	}

	return New(attestorsSpecs)
}

// New Service constructor. Attestors should have unique aliases
func New(attestors []Attestor) (*Service, error) {
	set := make(map[string]Attestor)
	for _, attestor := range attestors {
		alias := attestor.Alias()

		if _, alreadyExists := set[alias]; alreadyExists {
			return nil, fmt.Errorf("attestor with alias %s already exists", alias)
		}

		set[alias] = attestor
	}

	return &Service{
		logger:    slog.With("service", "attestors"),
		attestors: set,
	}, nil
}

// Add adds an attestor to the service. Not thread-safe.
func (s *Service) Add(id string, attestor Attestor) {
	s.attestors[id] = attestor
}

func (s *Service) LatestAttestableHeight(ctx context.Context, attestorAlias string) (uint64, error) {
	attestor, ok := s.attestors[attestorAlias]
	if !ok {
		return 0, ErrNotFound
	}

	return attestor.LatestAttestableHeight(ctx)
}

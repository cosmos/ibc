// SPDX-License-Identifier: Apache-2.0

package attestor

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	attestordomain "github.com/cosmos/ibc/link/attestor"
	"github.com/cosmos/ibc/link/internal/chains"
	"github.com/cosmos/ibc/link/internal/config"
	"github.com/cosmos/ibc/link/internal/service/signer"
)

// Attestor reports attestation state.
type Attestor interface {
	ChainID() string
	Name() string
	Alias() string
	IsLocal() bool
	LatestHeight(ctx context.Context) (uint64, error)
	StateAttestation(ctx context.Context, height uint64) (attestordomain.Attestation, error)
	PacketAttestation(
		ctx context.Context,
		req attestordomain.PacketAttestationRequest,
	) (attestordomain.Attestation, error)
}

// Service manages configured attestors.
type Service struct {
	attestors map[string]Attestor
	logger    *slog.Logger
}

// Service errors.
var (
	ErrNotFound       = errors.New("attestor not found")
	ErrNoAttestations = errors.New("no attestations provided")
)

// NewFromConfig creates a new attestor service from the configuration.
// Because config represents our local binary, ALL attestors are local.
func NewFromConfig(cfg config.Config, clients *chains.ClientSet, signers *signer.Set) (*Service, error) {
	if len(cfg.Attestor.Attestations) == 0 {
		return nil, ErrNoAttestations
	}

	attestorsSpecs := make([]Attestor, 0, len(cfg.Attestor.Attestations))

	add := func(spec config.AttestationConfig) error {
		client, ok := clients.Get(spec.ChainID)
		if !ok {
			return fmt.Errorf("client not found for chain %s", spec.ChainID)
		}

		signer, ok := signers.Get(spec.Signer)
		if !ok {
			return fmt.Errorf("unknown signer %s", spec.Signer)
		}

		a, err := NewLocal(spec, client, signer)
		if err != nil {
			return err
		}

		attestorsSpecs = append(attestorsSpecs, a)

		return nil
	}

	for _, spec := range cfg.Attestor.Attestations {
		if err := add(spec); err != nil {
			return nil, fmt.Errorf("attestor %s: %w", spec.Name, err)
		}
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

// Get returns the attestor registered under alias
func (s *Service) Get(alias string) (Attestor, bool) {
	a, ok := s.attestors[alias]
	return a, ok
}

func (s *Service) LatestHeight(ctx context.Context, attestor string) (uint64, error) {
	a, ok := s.attestors[attestor]
	if !ok {
		return 0, ErrNotFound
	}

	return a.LatestHeight(ctx)
}

func (s *Service) StateAttestation(
	ctx context.Context,
	attestor string,
	height uint64,
) (attestordomain.Attestation, error) {
	a, ok := s.attestors[attestor]
	if !ok {
		return attestordomain.Attestation{}, ErrNotFound
	}

	return a.StateAttestation(ctx, height)
}

func (s *Service) PacketAttestation(
	ctx context.Context,
	attestor string,
	req attestordomain.PacketAttestationRequest,
) (attestordomain.Attestation, error) {
	a, ok := s.attestors[attestor]
	if !ok {
		return attestordomain.Attestation{}, ErrNotFound
	}

	return a.PacketAttestation(ctx, req)
}

package attestor

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/pkg/errors"
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

	// Name is the name of the attestor, used to key it uniquely within this
	// process's Service.
	Name() string

	// IsLocal returns true if the attestor is local.
	IsLocal() bool

	// Address is the attestor's signing address.
	Address() string

	LatestHeight(ctx context.Context) (uint64, error)
	StateAttestation(ctx context.Context, height uint64) (Attestation, error)
	PacketAttestation(ctx context.Context, req PacketAttestationRequest) (Attestation, error)
}

// Info identifies one attestor.
type Info struct {
	ChainID string
	Address string
}

// PacketAttestationRequest is a request for packet commitment attestations.
type PacketAttestationRequest struct {
	Height         uint64
	Packets        [][]byte
	CommitmentType CommitmentType
}

// Attestation is a signed attestation over chain state or packet commitments.
type Attestation struct {
	Height       uint64
	Timestamp    *time.Time
	AttestedData []byte
	Signature    []byte
}

// CommitmentType identifies the kind of packet commitment being attested.
type CommitmentType int32

// Commitment type values.
const (
	CommitmentTypeInvalid CommitmentType = iota
	CommitmentTypePacket
	CommitmentTypeAck
	CommitmentTypeReceipt
)

// MaxPacketsPerAttestation bounds packet decoding and chain calls per request.
const MaxPacketsPerAttestation = 100

// MaxPacketSizeBytes bounds the size of a single packet in bytes.
const MaxPacketSizeBytes = 128 * 1024 // 128 KB

// Attestor errors
var (
	ErrNotFound           = errors.New("attestor not found")
	ErrNoAttestations     = errors.New("no attestations provided")
	ErrNotFinalized       = errors.New("block is not finalized")
	ErrInvalidInput       = errors.New("invalid input")
	ErrCommitmentNotFound = errors.New("commitment not found")
	ErrReceiptExists      = errors.New("receipt exists")
)

// New Service constructor. Attestors should have unique names.
func New(attestors []Attestor) (*Service, error) {
	set := make(map[string]Attestor)
	for _, attestor := range attestors {
		name := attestor.Name()

		if _, alreadyExists := set[name]; alreadyExists {
			return nil, fmt.Errorf("attestor with alias %s already exists", name)
		}

		set[name] = attestor
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

// Info returns identity information for the attestor registered under alias.
func (s *Service) Info(alias string) (Info, bool) {
	a, ok := s.attestors[alias]
	if !ok {
		return Info{}, false
	}

	return Info{
		ChainID: a.ChainID(),
		Address: a.Address(),
	}, true
}

func (s *Service) LatestHeight(ctx context.Context, attestor string) (uint64, error) {
	a, ok := s.attestors[attestor]
	if !ok {
		return 0, ErrNotFound
	}

	return a.LatestHeight(ctx)
}

func (s *Service) StateAttestation(ctx context.Context, attestor string, height uint64) (Attestation, error) {
	a, ok := s.attestors[attestor]
	if !ok {
		return Attestation{}, ErrNotFound
	}

	return a.StateAttestation(ctx, height)
}

func (s *Service) PacketAttestation(
	ctx context.Context,
	attestor string,
	req PacketAttestationRequest,
) (Attestation, error) {
	a, ok := s.attestors[attestor]
	if !ok {
		return Attestation{}, ErrNotFound
	}

	return a.PacketAttestation(ctx, req)
}

func (req PacketAttestationRequest) Validate() error {
	switch req.CommitmentType {
	case CommitmentTypePacket, CommitmentTypeAck, CommitmentTypeReceipt:
	default:
		return errors.Errorf("unsupported commitment type %d", req.CommitmentType)
	}

	if count := len(req.Packets); count == 0 || count > MaxPacketsPerAttestation {
		return errors.Errorf(
			"packet count %d is outside allowed range 1..%d",
			count,
			MaxPacketsPerAttestation,
		)
	}

	for _, packet := range req.Packets {
		if len(packet) > MaxPacketSizeBytes {
			return errors.Errorf("packet size %d is greater than %d", len(packet), MaxPacketSizeBytes)
		}
	}

	return validateHeight(req.Height)
}

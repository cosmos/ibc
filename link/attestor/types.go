// Package attestor defines Link's attestation domain model.
package attestor

import (
	"errors"
	"fmt"
	"math"
	"time"
)

// Attestation is a signed attestation over chain state or packet commitments.
type Attestation struct {
	Height       uint64
	Timestamp    *time.Time
	AttestedData []byte
	Signature    []byte
}

// PacketAttestationRequest requests packet commitment attestations.
type PacketAttestationRequest struct {
	Height         uint64
	Packets        [][]byte
	CommitmentType CommitmentType
}

// CommitmentType identifies the packet commitment being attested.
type CommitmentType int32

// Commitment types.
const (
	CommitmentTypeInvalid CommitmentType = iota
	CommitmentTypePacket
	CommitmentTypeAck
	CommitmentTypeReceipt
)

// Request limits.
const (
	MaxPacketsPerAttestation = 100
	MaxPacketSizeBytes       = 128 * 1024
)

// Domain errors.
var (
	ErrNotFinalized       = errors.New("block is not finalized")
	ErrInvalidInput       = errors.New("invalid input")
	ErrCommitmentNotFound = errors.New("commitment not found")
	ErrReceiptExists      = errors.New("receipt exists")
)

// Validate checks the request's public input bounds.
func (req PacketAttestationRequest) Validate() error {
	switch req.CommitmentType {
	case CommitmentTypePacket, CommitmentTypeAck, CommitmentTypeReceipt:
	default:
		return fmt.Errorf("unsupported commitment type %d", req.CommitmentType)
	}

	if count := len(req.Packets); count == 0 || count > MaxPacketsPerAttestation {
		return fmt.Errorf(
			"packet count %d is outside allowed range 1..%d",
			count,
			MaxPacketsPerAttestation,
		)
	}

	for _, packet := range req.Packets {
		if len(packet) > MaxPacketSizeBytes {
			return fmt.Errorf("packet size %d is greater than %d", len(packet), MaxPacketSizeBytes)
		}
	}

	return ValidateHeight(req.Height)
}

// ValidateHeight rejects zero and reserved chain-query heights.
func ValidateHeight(height uint64) error {
	switch height {
	case 0:
		return errors.New("height must be greater than 0")
	case math.MaxUint64, math.MaxUint64 - 1:
		return fmt.Errorf("invalid height: %d", height)
	default:
		return nil
	}
}

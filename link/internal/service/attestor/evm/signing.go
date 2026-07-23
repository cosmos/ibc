package evm

import (
	"context"
	"crypto/sha256"
	"fmt"
)

// Signer signs a 32-byte ECDSA digest.
type Signer interface {
	Sign(ctx context.Context, data []byte) ([]byte, error)
}

// Domain separation tags.
const (
	TagStateAttestation  byte = 0x01
	TagPacketAttestation byte = 0x02
)

// SignABI signs sha256(tag || sha256(data)), matching the domain separation.
func SignABI(ctx context.Context, signer Signer, tag byte, data []byte) ([]byte, error) {
	innerHash := sha256.Sum256(data)

	var signingInput [1 + sha256.Size]byte
	signingInput[0] = tag
	copy(signingInput[1:], innerHash[:])
	digest := sha256.Sum256(signingInput[:])

	signature, err := signer.Sign(ctx, digest[:])
	if err != nil {
		return nil, err
	}

	return normalizeSignature(signature)
}

// normalizeSignature converts the recovery ID from the signer's 0/1 form to
// Solidity's 27/28 form. It accepts either form and does not mutate the input.
func normalizeSignature(signature []byte) ([]byte, error) {
	if len(signature) != 65 {
		return nil, fmt.Errorf("invalid signature length %d, expected 65", len(signature))
	}

	normalized := append([]byte(nil), signature...)
	switch normalized[64] {
	case 0, 1:
		normalized[64] += 27
	case 27, 28:
	default:
		return nil, fmt.Errorf("invalid recovery ID %d", normalized[64])
	}

	return normalized, nil
}

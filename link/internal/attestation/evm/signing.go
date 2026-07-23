package evm

import (
	"context"
	"crypto/sha256"
	"fmt"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
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

// SignABI signs Digest(tag, data).
func SignABI(ctx context.Context, signer Signer, tag byte, data []byte) ([]byte, error) {
	digest := Digest(tag, data)

	signature, err := signer.Sign(ctx, digest[:])
	if err != nil {
		return nil, err
	}

	return normalizeSignature(signature)
}

// Digest computes sha256(tag || sha256(data)), the domain-separated digest
// AttestationLightClient.sol verifies signatures over: attestors sign it
// with Digest+SignABI here; verifiers recover the signer over the same
// digest, computed independently since they only ever see the signature,
// never call SignABI themselves.
func Digest(tag byte, data []byte) [32]byte {
	inner := sha256.Sum256(data)

	var signingInput [1 + sha256.Size]byte
	signingInput[0] = tag
	copy(signingInput[1:], inner[:])

	return sha256.Sum256(signingInput[:])
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

// RecoverSigner recovers the address that produced sig (65-byte r||s||v)
// over digest, the inverse of SignABI: it accepts either recovery-ID form
// (crypto.SigToPub requires the signer's raw 0/1 form, but attestors sign
// with Ethereum's legacy v of 27/28 via normalizeSignature above, and a
// verifier only ever sees the signature on the wire, never picks the form
// itself).
func RecoverSigner(digest [32]byte, sig []byte) (common.Address, error) {
	if len(sig) != 65 {
		return common.Address{}, fmt.Errorf("signature must be 65 bytes, got %d", len(sig))
	}

	normalized := make([]byte, 65)
	copy(normalized, sig)

	if normalized[64] >= 27 {
		normalized[64] -= 27
	}

	pub, err := crypto.SigToPub(digest[:], normalized)
	if err != nil {
		return common.Address{}, fmt.Errorf("recovering signer public key: %w", err)
	}

	return crypto.PubkeyToAddress(*pub), nil
}

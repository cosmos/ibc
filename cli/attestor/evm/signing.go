// SPDX-License-Identifier: Apache-2.0

package evm

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"math/big"

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

// Digest computes sha256(tag || sha256(data)), the domain-separated digest
// AttestationLightClient.sol verifies signatures over.
func Digest(tag byte, data []byte) [32]byte {
	inner := sha256.Sum256(data)

	var signingInput [1 + sha256.Size]byte
	signingInput[0] = tag
	copy(signingInput[1:], inner[:])

	return sha256.Sum256(signingInput[:])
}

// SignABI signs Digest(tag, data).
func SignABI(ctx context.Context, signer Signer, tag byte, data []byte) ([]byte, error) {
	digest := Digest(tag, data)

	signature, err := signer.Sign(ctx, digest[:])
	if err != nil {
		return nil, err
	}

	return NormalizeSignature(signature)
}

var (
	secp256k1N     = crypto.S256().Params().N
	secp256k1HalfN = new(big.Int).Rsh(secp256k1N, 1)
)

// NormalizeSignature puts a 65-byte r||s||v signature in the only form
// AttestationLightClient.sol's ECDSA.recover accepts: v of 27/28 and s in the
// lower half of the curve order. It accepts v of 0/1 or 27/28 and either s,
// which recover to the same signer, and does not mutate the input.
func NormalizeSignature(signature []byte) ([]byte, error) {
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

	// (r, n-s) with the other recovery ID is the same signature.
	s := new(big.Int).SetBytes(normalized[32:64])
	if s.Cmp(secp256k1HalfN) > 0 && s.Cmp(secp256k1N) < 0 {
		s.Sub(secp256k1N, s).FillBytes(normalized[32:64])
		normalized[64] = 27 + 28 - normalized[64]
	}

	return normalized, nil
}

// RecoverSigner recovers the address that produced sig over digest. Like
// AttestationLightClient.sol, it accepts only v of 27/28 and low s;
// NormalizeSignature converts other forms of the same signature.
func RecoverSigner(digest [32]byte, sig []byte) (common.Address, error) {
	if len(sig) != 65 {
		return common.Address{}, fmt.Errorf("invalid signature length %d, expected 65", len(sig))
	}
	if sig[64] != 27 && sig[64] != 28 {
		return common.Address{}, fmt.Errorf("invalid recovery ID %d, expected 27 or 28", sig[64])
	}

	// crypto.SigToPub takes the signer's raw 0/1 recovery ID.
	raw := append([]byte(nil), sig...)
	raw[64] -= 27
	r := new(big.Int).SetBytes(raw[:32])
	s := new(big.Int).SetBytes(raw[32:64])
	if !crypto.ValidateSignatureValues(raw[64], r, s, true) {
		return common.Address{}, errors.New("signature not canonical: r or s out of range, or s above n/2")
	}

	pub, err := crypto.SigToPub(digest[:], raw)
	if err != nil {
		return common.Address{}, fmt.Errorf("recovering signer public key: %w", err)
	}

	return crypto.PubkeyToAddress(*pub), nil
}

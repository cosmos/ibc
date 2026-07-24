package attestation

import (
	"crypto/sha256"
	"fmt"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
)

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

// RecoverSigner recovers the address that produced sig (65-byte r||s||v)
// over digest
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

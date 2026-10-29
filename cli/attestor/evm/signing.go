// SPDX-License-Identifier: Apache-2.0

package evm

import (
	"context"
	"crypto/sha256"

	"github.com/cosmos/ibc/cli/internal/pkg/ecrecover"
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

// SignABI signs Digest(tag, data) in the form AttestationLightClient.sol
// accepts.
func SignABI(ctx context.Context, signer Signer, tag byte, data []byte) ([]byte, error) {
	digest := Digest(tag, data)

	signature, err := signer.Sign(ctx, digest[:])
	if err != nil {
		return nil, err
	}

	_, signature, err = ecrecover.Recover(digest[:], signature)

	return signature, err
}

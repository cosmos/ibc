// SPDX-License-Identifier: Apache-2.0

package evm

import (
	"context"
	"crypto/sha256"

	"github.com/ethereum/go-ethereum/common"

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

	_, signature, err = RecoverSigner(digest, signature)

	return signature, err
}

// RecoverSigner recovers the address that produced sig over digest, and
// returns sig in the only form AttestationLightClient.sol accepts. sig may use
// v of 0/1 or 27/28 and either s.
func RecoverSigner(digest [32]byte, sig []byte) (common.Address, []byte, error) {
	signer, canonical, err := ecrecover.Recover(digest[:], sig)
	if err != nil {
		return common.Address{}, nil, err
	}

	// AttestationLightClient.sol passes the signature to ECDSA.recover unchanged,
	// which takes only v of 27/28.
	canonical[64] += 27

	return signer, canonical, nil
}

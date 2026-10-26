// SPDX-License-Identifier: Apache-2.0

// Package ecrecover recovers secp256k1 signers in the form the light clients
// accept.
package ecrecover

import (
	"errors"
	"fmt"

	"github.com/decred/dcrd/dcrec/secp256k1/v4"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
)

// Recover returns the address that produced sig over digest, and sig with low
// s and a 0/1 recovery ID. sig may use v of 0/1 or 27/28 and either s.
//
// 0/1 is go-ethereum's form and the one Besu commit seals use, which the Besu
// light client takes as is. Callers whose contract passes the signature
// straight to OpenZeppelin's ECDSA.recover must add 27 to v.
func Recover(digest, sig []byte) (common.Address, []byte, error) {
	if len(sig) != crypto.SignatureLength {
		return common.Address{}, nil, fmt.Errorf("invalid signature length %d", len(sig))
	}

	canonical := append([]byte(nil), sig...)
	switch canonical[64] {
	case 0, 1:
	case 27, 28:
		canonical[64] -= 27
	default:
		return common.Address{}, nil, fmt.Errorf("invalid recovery ID %d", canonical[64])
	}

	// (r, n-s) with the other recovery ID is the same signature.
	var s secp256k1.ModNScalar
	if s.SetByteSlice(canonical[32:64]) {
		return common.Address{}, nil, errors.New("s not below the curve order")
	}
	if s.IsOverHalfOrder() {
		s.Negate().PutBytesUnchecked(canonical[32:64])
		canonical[64] ^= 1
	}

	pub, err := crypto.SigToPub(digest, canonical)
	if err != nil {
		return common.Address{}, nil, fmt.Errorf("recovering signer public key: %w", err)
	}

	return crypto.PubkeyToAddress(*pub), canonical, nil
}

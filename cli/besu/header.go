// SPDX-License-Identifier: Apache-2.0

package besu

import (
	"errors"
	"fmt"
	"math/big"
	"slices"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/rlp"
)

// Header is a parsed Besu QBFT block header. RLP is the exact encoding the
// light client receives; the other fields are decoded from it.
type Header struct {
	RLP         []byte
	Height      uint64
	Timestamp   uint64
	StateRoot   common.Hash
	Validators  []common.Address
	CommitSeals [][]byte

	items      []rlp.RawValue
	extraItems []rlp.RawValue
}

// ValidateValidators rejects empty sets, the zero address, duplicates and
// unsorted sets, as the light client's _validateValidators does. Besu writes
// validator sets in ascending address order.
func ValidateValidators(validators []common.Address) error {
	if len(validators) == 0 {
		return errors.New("empty validator set")
	}

	for i, validator := range validators {
		if validator == (common.Address{}) {
			return errors.New("zero validator address")
		}

		if i > 0 {
			switch validator.Cmp(validators[i-1]) {
			case 0:
				return fmt.Errorf("duplicate validator %s", validator)
			case -1:
				return fmt.Errorf("validator %s is out of order", validator)
			}
		}
	}

	return nil
}

// Signers recovers the unique commit seal signers of the header.
func (h *Header) Signers() ([]common.Address, error) {
	digest, err := h.CommitSealDigest()
	if err != nil {
		return nil, err
	}

	signers := make([]common.Address, 0, len(h.CommitSeals))
	seen := make(map[common.Address]struct{}, len(h.CommitSeals))

	for i, seal := range h.CommitSeals {
		signer, err := RecoverSealSigner(digest, seal)
		if err != nil {
			return nil, fmt.Errorf("commit seal %d: %w", i, err)
		}

		if _, dup := seen[signer]; dup {
			return nil, fmt.Errorf("%w: %s", ErrDuplicateSigner, signer)
		}

		seen[signer] = struct{}{}
		signers = append(signers, signer)
	}

	return signers, nil
}

// RecoverSealSigner recovers the address behind a 65-byte commit seal over
// digest. It accepts v as 0/1 or 27/28 and rejects high-s signatures, matching
// the light client's normalisation and OpenZeppelin ECDSA.tryRecover.
func RecoverSealSigner(digest common.Hash, seal []byte) (common.Address, error) {
	if len(seal) != crypto.SignatureLength {
		return common.Address{}, fmt.Errorf("%w: %d bytes, want %d", ErrInvalidSeal, len(seal), crypto.SignatureLength)
	}

	sig := slices.Clone(seal)
	if sig[crypto.SignatureLength-1] >= 27 {
		sig[crypto.SignatureLength-1] -= 27
	}

	r := new(big.Int).SetBytes(sig[:32])
	s := new(big.Int).SetBytes(sig[32:64])

	if !crypto.ValidateSignatureValues(sig[crypto.SignatureLength-1], r, s, true) {
		return common.Address{}, fmt.Errorf("%w: signature values out of range", ErrInvalidSeal)
	}

	pub, err := crypto.SigToPub(digest.Bytes(), sig)
	if err != nil {
		return common.Address{}, fmt.Errorf("%w: %w", ErrInvalidSeal, err)
	}

	return crypto.PubkeyToAddress(*pub), nil
}

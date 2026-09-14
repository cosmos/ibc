// SPDX-License-Identifier: Apache-2.0

package besu

import (
	"bytes"
	"errors"
	"fmt"
	"math/big"
	"slices"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/rlp"
)

// RLP layout of a Besu BFT header, as checked by BesuLightClientBase._parseHeader.
const (
	minHeaderItems     = 15
	extraDataItemCount = 5
	sealLength         = 65
	validatorLength    = 20
	nonceLength        = 8

	idxOmmersHash = 1
	idxStateRoot  = 3
	idxDifficulty = 7
	idxNumber     = 8
	idxTimestamp  = 11
	idxExtraData  = 12
	idxMixHash    = 13
	idxNonce      = 14

	extraIdxValidators  = 1
	extraIdxCommitSeals = 4
)

// BFTMixHash is the sentinel mix hash Besu BFT engines write into every header.
var BFTMixHash = common.HexToHash("0x63746963616c2062797a616e74696e65206661756c7420746f6c6572616e6365")

var (
	emptyRLPList = rlp.RawValue{0xc0}
	zeroNonce    = make([]byte, nonceLength)
	oneBig       = big.NewInt(1)
)

// Header is a parsed Besu QBFT block header. RLP is the exact encoding the
// light client receives; the other fields are decoded from it.
type Header struct {
	RLP         []byte
	Height      uint64
	Timestamp   uint64
	Validators  []common.Address
	CommitSeals [][]byte

	items      []rlp.RawValue
	extraItems []rlp.RawValue
}

// EncodeHeader RLP-encodes a go-ethereum header. go-ethereum keeps every
// optional post-merge field the node returned, so the result reproduces the
// bytes Besu sealed; a mismatch surfaces as commit seal signers that are not
// validators.
func EncodeHeader(h *types.Header) ([]byte, error) {
	if h == nil {
		return nil, fmt.Errorf("%w: nil header", ErrInvalidHeader)
	}

	encoded, err := rlp.EncodeToBytes(h)
	if err != nil {
		return nil, fmt.Errorf("encode header %d: %w", h.Number, err)
	}

	return encoded, nil
}

// ParseHeader decodes a raw Besu header and validates the fields the light
// client checks so a bad header fails before it is submitted.
func ParseHeader(headerRLP []byte) (*Header, error) {
	var items []rlp.RawValue
	if err := rlp.DecodeBytes(headerRLP, &items); err != nil {
		return nil, fmt.Errorf("%w: decode header list: %w", ErrInvalidHeader, err)
	}

	if len(items) < minHeaderItems {
		return nil, fmt.Errorf("%w: %d header items, need at least %d", ErrInvalidHeader, len(items), minHeaderItems)
	}

	header := &Header{RLP: slices.Clone(headerRLP), items: items}

	if err := header.decodeFixedFields(); err != nil {
		return nil, err
	}

	if err := header.decodeExtraData(); err != nil {
		return nil, err
	}

	return header, nil
}

func (h *Header) decodeFixedFields() error {
	var ommers common.Hash
	if err := rlp.DecodeBytes(h.items[idxOmmersHash], &ommers); err != nil {
		return fmt.Errorf("%w: ommers hash: %w", ErrInvalidHeader, err)
	}

	if ommers != types.EmptyUncleHash {
		return fmt.Errorf("%w: ommers hash %s is not the empty list hash", ErrInvalidHeader, ommers)
	}

	var stateRoot common.Hash
	if err := rlp.DecodeBytes(h.items[idxStateRoot], &stateRoot); err != nil {
		return fmt.Errorf("%w: state root: %w", ErrInvalidHeader, err)
	}

	var difficulty big.Int
	if err := rlp.DecodeBytes(h.items[idxDifficulty], &difficulty); err != nil {
		return fmt.Errorf("%w: difficulty: %w", ErrInvalidHeader, err)
	}

	if difficulty.Cmp(oneBig) != 0 {
		return fmt.Errorf("%w: difficulty %s, want 1", ErrInvalidHeader, &difficulty)
	}

	if err := rlp.DecodeBytes(h.items[idxNumber], &h.Height); err != nil {
		return fmt.Errorf("%w: number: %w", ErrInvalidHeader, err)
	}

	if err := rlp.DecodeBytes(h.items[idxTimestamp], &h.Timestamp); err != nil {
		return fmt.Errorf("%w: timestamp: %w", ErrInvalidHeader, err)
	}

	var mixHash common.Hash
	if err := rlp.DecodeBytes(h.items[idxMixHash], &mixHash); err != nil {
		return fmt.Errorf("%w: mix hash: %w", ErrInvalidHeader, err)
	}

	if mixHash != BFTMixHash {
		return fmt.Errorf("%w: mix hash %s is not the Besu BFT sentinel", ErrInvalidHeader, mixHash)
	}

	var nonce []byte
	if err := rlp.DecodeBytes(h.items[idxNonce], &nonce); err != nil {
		return fmt.Errorf("%w: nonce: %w", ErrInvalidHeader, err)
	}

	if !bytes.Equal(nonce, zeroNonce) {
		return fmt.Errorf("%w: nonce %x, want eight zero bytes", ErrInvalidHeader, nonce)
	}

	return nil
}

func (h *Header) decodeExtraData() error {
	var extraData []byte
	if err := rlp.DecodeBytes(h.items[idxExtraData], &extraData); err != nil {
		return fmt.Errorf("%w: extra data: %w", ErrInvalidHeader, err)
	}

	if err := rlp.DecodeBytes(extraData, &h.extraItems); err != nil {
		return fmt.Errorf("%w: extra data list: %w", ErrInvalidHeader, err)
	}

	if len(h.extraItems) != extraDataItemCount {
		return fmt.Errorf(
			"%w: %d extra data items, want %d", ErrInvalidHeader, len(h.extraItems), extraDataItemCount,
		)
	}

	if err := rlp.DecodeBytes(h.extraItems[extraIdxValidators], &h.Validators); err != nil {
		return fmt.Errorf("%w: validators: %w", ErrInvalidHeader, err)
	}

	if err := ValidateValidators(h.Validators); err != nil {
		return fmt.Errorf("%w: %w", ErrInvalidHeader, err)
	}

	if err := rlp.DecodeBytes(h.extraItems[extraIdxCommitSeals], &h.CommitSeals); err != nil {
		return fmt.Errorf("%w: commit seals: %w", ErrInvalidHeader, err)
	}

	return nil
}

// ValidateValidators rejects empty sets, the zero address and duplicates, as
// the light client constructor and header parser do.
func ValidateValidators(validators []common.Address) error {
	if len(validators) == 0 {
		return errors.New("empty validator set")
	}

	seen := make(map[common.Address]struct{}, len(validators))
	for _, validator := range validators {
		if validator == (common.Address{}) {
			return errors.New("zero validator address")
		}

		if _, dup := seen[validator]; dup {
			return fmt.Errorf("duplicate validator %s", validator)
		}

		seen[validator] = struct{}{}
	}

	return nil
}

// CommitSealDigest is the QBFT sealing digest: the header re-encoded with the
// commit seal list replaced by an empty RLP list, then keccak256.
func (h *Header) CommitSealDigest() (common.Hash, error) {
	signingExtra := slices.Clone(h.extraItems)
	signingExtra[extraIdxCommitSeals] = emptyRLPList

	extraData, err := rlp.EncodeToBytes(signingExtra)
	if err != nil {
		return common.Hash{}, fmt.Errorf("encode signing extra data: %w", err)
	}

	items := slices.Clone(h.items)

	encodedExtra, err := rlp.EncodeToBytes(extraData)
	if err != nil {
		return common.Hash{}, fmt.Errorf("encode signing extra data field: %w", err)
	}

	items[idxExtraData] = encodedExtra

	payload, err := rlp.EncodeToBytes(items)
	if err != nil {
		return common.Hash{}, fmt.Errorf("encode signing header: %w", err)
	}

	return crypto.Keccak256Hash(payload), nil
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
	if len(seal) != sealLength {
		return common.Address{}, fmt.Errorf("%w: %d bytes, want %d", ErrInvalidSeal, len(seal), sealLength)
	}

	sig := slices.Clone(seal)
	if sig[sealLength-1] >= 27 {
		sig[sealLength-1] -= 27
	}

	r := new(big.Int).SetBytes(sig[:32])
	s := new(big.Int).SetBytes(sig[32:64])

	if !crypto.ValidateSignatureValues(sig[sealLength-1], r, s, true) {
		return common.Address{}, fmt.Errorf("%w: signature values out of range", ErrInvalidSeal)
	}

	pub, err := crypto.SigToPub(digest.Bytes(), sig)
	if err != nil {
		return common.Address{}, fmt.Errorf("%w: %w", ErrInvalidSeal, err)
	}

	signer := crypto.PubkeyToAddress(*pub)
	if signer == (common.Address{}) {
		return common.Address{}, fmt.Errorf("%w: recovered the zero address", ErrInvalidSeal)
	}

	return signer, nil
}

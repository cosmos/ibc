// SPDX-License-Identifier: Apache-2.0

package besu

import (
	"bytes"
	"fmt"
	"math/big"
	"slices"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/rlp"

	"github.com/cosmos/ibc/cli/besu/internal/headerlayout"
)

// BFTMixHash is the Besu BFT sentinel checked by BesuLightClientBase._parseHeader.
// Keep this check: an ordinary Ethereum header is not a valid QBFT header.
var BFTMixHash = common.HexToHash("0x63746963616c2062797a616e74696e65206661756c7420746f6c6572616e6365")

var (
	zeroNonce = make([]byte, len(types.BlockNonce{}))
	oneBig    = big.NewInt(1)
)

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

// ParseSealedHeader encodes a node-supplied header the way Besu sealed it
// and parses the BFT fields the light client checks. Deploy and the prover
// both start from this: validators come from extraData, not a QBFT RPC.
func ParseSealedHeader(h *types.Header) (*Header, error) {
	encoded, err := EncodeHeader(h)
	if err != nil {
		return nil, err
	}

	return ParseHeader(encoded)
}

// ParseHeader decodes a raw Besu header and validates the fields the light
// client checks so a bad header fails before it is submitted.
func ParseHeader(headerRLP []byte) (*Header, error) {
	var items []rlp.RawValue
	if err := rlp.DecodeBytes(headerRLP, &items); err != nil {
		return nil, fmt.Errorf("%w: decode header list: %w", ErrInvalidHeader, err)
	}

	if len(items) < headerlayout.MinHeaderItems {
		return nil, fmt.Errorf(
			"%w: %d header items, need at least %d",
			ErrInvalidHeader,
			len(items),
			headerlayout.MinHeaderItems,
		)
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
	if err := rlp.DecodeBytes(h.items[headerlayout.IdxOmmersHash], &ommers); err != nil {
		return fmt.Errorf("%w: ommers hash: %w", ErrInvalidHeader, err)
	}

	if ommers != types.EmptyUncleHash {
		return fmt.Errorf("%w: ommers hash %s is not the empty list hash", ErrInvalidHeader, ommers)
	}

	if err := rlp.DecodeBytes(h.items[headerlayout.IdxStateRoot], &h.StateRoot); err != nil {
		return fmt.Errorf("%w: state root: %w", ErrInvalidHeader, err)
	}

	var difficulty big.Int
	if err := rlp.DecodeBytes(h.items[headerlayout.IdxDifficulty], &difficulty); err != nil {
		return fmt.Errorf("%w: difficulty: %w", ErrInvalidHeader, err)
	}

	if difficulty.Cmp(oneBig) != 0 {
		return fmt.Errorf("%w: difficulty %s, want 1", ErrInvalidHeader, &difficulty)
	}

	if err := rlp.DecodeBytes(h.items[headerlayout.IdxNumber], &h.Height); err != nil {
		return fmt.Errorf("%w: number: %w", ErrInvalidHeader, err)
	}

	if err := rlp.DecodeBytes(h.items[headerlayout.IdxTimestamp], &h.Timestamp); err != nil {
		return fmt.Errorf("%w: timestamp: %w", ErrInvalidHeader, err)
	}

	var mixHash common.Hash
	if err := rlp.DecodeBytes(h.items[headerlayout.IdxMixHash], &mixHash); err != nil {
		return fmt.Errorf("%w: mix hash: %w", ErrInvalidHeader, err)
	}

	if mixHash != BFTMixHash {
		return fmt.Errorf("%w: mix hash %s is not the Besu BFT sentinel", ErrInvalidHeader, mixHash)
	}

	var nonce []byte
	if err := rlp.DecodeBytes(h.items[headerlayout.IdxNonce], &nonce); err != nil {
		return fmt.Errorf("%w: nonce: %w", ErrInvalidHeader, err)
	}

	if !bytes.Equal(nonce, zeroNonce) {
		return fmt.Errorf("%w: nonce %x, want eight zero bytes", ErrInvalidHeader, nonce)
	}

	return nil
}

func (h *Header) decodeExtraData() error {
	var extraData []byte
	if err := rlp.DecodeBytes(h.items[headerlayout.IdxExtraData], &extraData); err != nil {
		return fmt.Errorf("%w: extra data: %w", ErrInvalidHeader, err)
	}

	if err := rlp.DecodeBytes(extraData, &h.extraItems); err != nil {
		return fmt.Errorf("%w: extra data list: %w", ErrInvalidHeader, err)
	}

	if len(h.extraItems) != headerlayout.ExtraDataItemCount {
		return fmt.Errorf(
			"%w: %d extra data items, want %d", ErrInvalidHeader, len(h.extraItems), headerlayout.ExtraDataItemCount,
		)
	}

	if err := rlp.DecodeBytes(h.extraItems[headerlayout.ExtraIdxValidators], &h.Validators); err != nil {
		return fmt.Errorf("%w: validators: %w", ErrInvalidHeader, err)
	}

	if err := ValidateValidators(h.Validators); err != nil {
		return fmt.Errorf("%w: %w", ErrInvalidHeader, err)
	}

	if err := rlp.DecodeBytes(h.extraItems[headerlayout.ExtraIdxCommitSeals], &h.CommitSeals); err != nil {
		return fmt.Errorf("%w: commit seals: %w", ErrInvalidHeader, err)
	}

	return nil
}

var emptyRLPList = rlp.RawValue{0xc0}

// CommitSealDigest is the QBFT sealing digest: the header re-encoded with the
// commit seal list replaced by an empty RLP list, then keccak256.
func (h *Header) CommitSealDigest() (common.Hash, error) {
	signingExtra := slices.Clone(h.extraItems)
	signingExtra[headerlayout.ExtraIdxCommitSeals] = emptyRLPList

	extraData, err := rlp.EncodeToBytes(signingExtra)
	if err != nil {
		return common.Hash{}, fmt.Errorf("encode signing extra data: %w", err)
	}

	items := slices.Clone(h.items)

	encodedExtra, err := rlp.EncodeToBytes(extraData)
	if err != nil {
		return common.Hash{}, fmt.Errorf("encode signing extra data field: %w", err)
	}

	items[headerlayout.IdxExtraData] = encodedExtra

	payload, err := rlp.EncodeToBytes(items)
	if err != nil {
		return common.Hash{}, fmt.Errorf("encode signing header: %w", err)
	}

	return crypto.Keccak256Hash(payload), nil
}

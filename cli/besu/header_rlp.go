// SPDX-License-Identifier: Apache-2.0

package besu

import (
	"fmt"
	"slices"

	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/rlp"

	"github.com/cosmos/ibc/cli/besu/internal/headerlayout"
)

// EncodeHeader RLP-encodes a go-ethereum header. go-ethereum keeps every
// optional post-merge field the node returned, so the result reproduces the
// bytes Besu sealed.
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
// and parses the fields needed for payloads. Deploy and the prover
// both start from this: validators come from extraData, not a QBFT RPC.
func ParseSealedHeader(h *types.Header) (*Header, error) {
	encoded, err := EncodeHeader(h)
	if err != nil {
		return nil, err
	}

	return ParseHeader(encoded)
}

// ParseHeader decodes the fields needed for payloads without verifying consensus.
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

	header := &Header{RLP: slices.Clone(headerRLP)}

	if err := header.decodeFixedFields(items); err != nil {
		return nil, err
	}

	if err := header.decodeExtraData(items); err != nil {
		return nil, err
	}

	return header, nil
}

func (h *Header) decodeFixedFields(items []rlp.RawValue) error {
	if err := rlp.DecodeBytes(items[headerlayout.IdxStateRoot], &h.StateRoot); err != nil {
		return fmt.Errorf("%w: state root: %w", ErrInvalidHeader, err)
	}
	if err := rlp.DecodeBytes(items[headerlayout.IdxNumber], &h.Height); err != nil {
		return fmt.Errorf("%w: number: %w", ErrInvalidHeader, err)
	}
	if err := rlp.DecodeBytes(items[headerlayout.IdxTimestamp], &h.Timestamp); err != nil {
		return fmt.Errorf("%w: timestamp: %w", ErrInvalidHeader, err)
	}
	return nil
}

func (h *Header) decodeExtraData(items []rlp.RawValue) error {
	var extraData []byte
	if err := rlp.DecodeBytes(items[headerlayout.IdxExtraData], &extraData); err != nil {
		return fmt.Errorf("%w: extra data: %w", ErrInvalidHeader, err)
	}

	var extraItems []rlp.RawValue
	if err := rlp.DecodeBytes(extraData, &extraItems); err != nil {
		return fmt.Errorf("%w: extra data list: %w", ErrInvalidHeader, err)
	}

	if len(extraItems) != headerlayout.ExtraDataItemCount {
		return fmt.Errorf(
			"%w: %d extra data items, want %d", ErrInvalidHeader, len(extraItems), headerlayout.ExtraDataItemCount,
		)
	}

	if err := rlp.DecodeBytes(extraItems[headerlayout.ExtraIdxValidators], &h.Validators); err != nil {
		return fmt.Errorf("%w: validators: %w", ErrInvalidHeader, err)
	}

	return nil
}

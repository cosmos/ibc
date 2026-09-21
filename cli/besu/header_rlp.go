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
// and reads the fields needed for payloads. Deploy and the prover
// both start from this: validators come from extraData, not a QBFT RPC.
func ParseSealedHeader(h *types.Header) (*Header, error) {
	encoded, err := EncodeHeader(h)
	if err != nil {
		return nil, err
	}

	return newHeader(h, encoded)
}

// ParseHeader decodes the fields needed for payloads without verifying consensus.
func ParseHeader(headerRLP []byte) (*Header, error) {
	var h types.Header
	if err := rlp.DecodeBytes(headerRLP, &h); err != nil {
		return nil, fmt.Errorf("%w: %w", ErrInvalidHeader, err)
	}

	return newHeader(&h, slices.Clone(headerRLP))
}

func newHeader(h *types.Header, encoded []byte) (*Header, error) {
	if h.Number == nil || !h.Number.IsUint64() {
		return nil, fmt.Errorf("%w: number %v", ErrInvalidHeader, h.Number)
	}

	var extraItems []rlp.RawValue
	if err := rlp.DecodeBytes(h.Extra, &extraItems); err != nil {
		return nil, fmt.Errorf("%w: extra data list: %w", ErrInvalidHeader, err)
	}

	if len(extraItems) != headerlayout.ExtraDataItemCount {
		return nil, fmt.Errorf(
			"%w: %d extra data items, want %d", ErrInvalidHeader, len(extraItems), headerlayout.ExtraDataItemCount,
		)
	}

	header := &Header{RLP: encoded, Height: h.Number.Uint64(), Timestamp: h.Time, StateRoot: h.Root}
	if err := rlp.DecodeBytes(extraItems[headerlayout.ExtraIdxValidators], &header.Validators); err != nil {
		return nil, fmt.Errorf("%w: validators: %w", ErrInvalidHeader, err)
	}

	return header, nil
}

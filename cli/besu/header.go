// SPDX-License-Identifier: Apache-2.0

package besu

import (
	"context"
	"errors"
	"fmt"
	"math/big"

	"github.com/cosmos/solidity-ibc-eureka/packages/go-abigen/besumsgs"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/rlp"
)

// Besu BFT extraData layout, matching _parseHeader in
// ibc-contracts/ibc-solidity/contracts/light-clients/besu/BesuLightClientBase.sol.
const (
	extraDataItemCount = 5
	extraIdxValidators = 1
)

// ParsedHeader is a Besu header's exact RLP plus the few fields decoded from
// it that light-client payloads need; consensus validity is checked by the
// contract.
type ParsedHeader struct {
	RLP        []byte
	Height     uint64
	Timestamp  uint64
	StateRoot  common.Hash
	Validators []common.Address
}

// ConsensusStateOf is the consensus state an update to header installs, and
// the preimage payloads carry for header's height.
func ConsensusStateOf(header *ParsedHeader) besumsgs.IBesuLightClientMsgsConsensusState {
	return besumsgs.IBesuLightClientMsgsConsensusState{
		Timestamp:  header.Timestamp,
		StateRoot:  header.StateRoot,
		Validators: header.Validators,
	}
}

// ParseSealedHeader encodes a node-supplied header the way Besu sealed it
// and reads the fields needed for payloads. go-ethereum keeps every optional
// post-merge field the node returned, so the encoding reproduces the sealed
// bytes. Validators come from extraData, not a QBFT RPC.
func ParseSealedHeader(h *types.Header) (*ParsedHeader, error) {
	if h == nil {
		return nil, errors.New("nil header")
	}

	encoded, err := rlp.EncodeToBytes(h)
	if err != nil {
		return nil, fmt.Errorf("encode header %d: %w", h.Number, err)
	}

	if h.Number == nil || !h.Number.IsUint64() {
		return nil, fmt.Errorf("invalid number %v", h.Number)
	}

	var extraItems []rlp.RawValue
	if err := rlp.DecodeBytes(h.Extra, &extraItems); err != nil {
		return nil, fmt.Errorf("extra data list: %w", err)
	}

	if len(extraItems) != extraDataItemCount {
		return nil, fmt.Errorf("%d extra data items, want %d", len(extraItems), extraDataItemCount)
	}

	header := &ParsedHeader{RLP: encoded, Height: h.Number.Uint64(), Timestamp: h.Time, StateRoot: h.Root}
	if err := rlp.DecodeBytes(extraItems[extraIdxValidators], &header.Validators); err != nil {
		return nil, fmt.Errorf("validators: %w", err)
	}

	return header, nil
}

// HeaderReader reads block headers from a node.
type HeaderReader interface {
	HeaderByNumber(ctx context.Context, number *big.Int) (*types.Header, error)
}

// ReadSealedHeader reads and parses the Besu QBFT header at number, or the
// head when number is nil.
func ReadSealedHeader(ctx context.Context, reader HeaderReader, number *big.Int) (*ParsedHeader, error) {
	at := "latest"
	if number != nil {
		at = number.String()
	}

	header, err := reader.HeaderByNumber(ctx, number)
	if err != nil {
		return nil, fmt.Errorf("getting header %s: %w", at, err)
	}

	parsed, err := ParseSealedHeader(header)
	if err != nil {
		return nil, fmt.Errorf("header %s is not a Besu QBFT header: %w", at, err)
	}

	return parsed, nil
}

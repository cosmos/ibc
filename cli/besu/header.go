// SPDX-License-Identifier: Apache-2.0

package besu

import (
	"context"
	"errors"
	"fmt"
	"math/big"
	"slices"

	"github.com/cosmos/solidity-ibc-eureka/packages/go-abigen/besumsgs"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/rlp"

	"github.com/cosmos/ibc/cli/internal/pkg/ecrecover"
)

// Besu BFT extraData layout, matching _parseHeader in
// ibc-contracts/ibc-solidity/contracts/light-clients/besu/BesuLightClientBase.sol.
const (
	extraDataItemCount  = 5
	extraIdxValidators  = 1
	extraIdxCommitSeals = 4
)

// emptyRLPList replaces the commit seals in the QBFT signing form of extraData.
var emptyRLPList = rlp.RawValue{0xc0}

// ParsedHeader is a Besu header's RLP in the form the light client accepts,
// with its commit seals normalized, plus the few fields decoded from it
// that light-client payloads need; consensus validity is checked by the
// contract.
type ParsedHeader struct {
	RLP        []byte
	Height     uint64
	Timestamp  uint64
	StateRoot  common.Hash
	Validators []common.Address
}

// ConsensusState is the consensus state an update to h installs, and the
// preimage payloads carry for h's height.
func (h *ParsedHeader) ConsensusState() besumsgs.IBesuLightClientMsgsConsensusState {
	return besumsgs.IBesuLightClientMsgsConsensusState{
		Timestamp:  h.Timestamp,
		StateRoot:  h.StateRoot,
		Validators: h.Validators,
	}
}

// ParseSealedHeader encodes a node-supplied header the way Besu sealed it,
// with its commit seals in the form the light client requires, and reads the
// fields needed for payloads. go-ethereum keeps every optional post-merge
// field the node returned, so apart from the commit seals the encoding
// reproduces the sealed bytes. Validators come from extraData, not a
// QBFT RPC.
func ParseSealedHeader(h *types.Header) (*ParsedHeader, error) {
	if h == nil {
		return nil, errors.New("nil header")
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

	header := &ParsedHeader{Height: h.Number.Uint64(), Timestamp: h.Time, StateRoot: h.Root}
	if err := rlp.DecodeBytes(extraItems[extraIdxValidators], &header.Validators); err != nil {
		return nil, fmt.Errorf("validators: %w", err)
	}

	sorted, err := normalizeCommitSeals(h, extraItems, header.Validators)
	if err != nil {
		return nil, fmt.Errorf("header %d: %w", header.Height, err)
	}

	header.RLP, err = rlp.EncodeToBytes(sorted)
	if err != nil {
		return nil, fmt.Errorf("encode header %d: %w", header.Height, err)
	}

	return header, nil
}

// normalizeCommitSeals returns a copy of h with only the commit seals the
// contract accepts: canonical, from one of validators, one per signer, in
// strictly ascending order of signer. The contract rejects the whole header
// for any other seal. Changing the seals is safe because the seal digest
// excludes them.
func normalizeCommitSeals(
	h *types.Header,
	extraItems []rlp.RawValue,
	validators []common.Address,
) (*types.Header, error) {
	var seals [][]byte
	if err := rlp.DecodeBytes(extraItems[extraIdxCommitSeals], &seals); err != nil {
		return nil, fmt.Errorf("commit seals: %w", err)
	}

	digest, err := commitSealDigest(h, extraItems)
	if err != nil {
		return nil, err
	}

	type signedSeal struct {
		signer common.Address
		seal   []byte
	}
	signed := make([]signedSeal, 0, len(seals))
	for _, seal := range seals {
		signer, canonical, sigErr := ecrecover.Recover(digest, seal)
		if sigErr == nil && slices.Contains(validators, signer) {
			signed = append(signed, signedSeal{signer: signer, seal: canonical})
		}
	}
	// Seals that all fail point at a digest mismatch, not at missing quorum.
	if len(seals) > 0 && len(signed) == 0 {
		return nil, errors.New("no commit seal recovers to a header validator")
	}
	slices.SortStableFunc(signed, func(a, b signedSeal) int { return a.signer.Cmp(b.signer) })
	signed = slices.CompactFunc(signed, func(a, b signedSeal) bool { return a.signer == b.signer })

	seals = seals[:0]
	for _, s := range signed {
		seals = append(seals, s.seal)
	}
	sortedSeals, err := rlp.EncodeToBytes(seals)
	if err != nil {
		return nil, fmt.Errorf("encode commit seals: %w", err)
	}

	return withCommitSeals(h, extraItems, sortedSeals)
}

// commitSealDigest is the QBFT digest commit seals sign: the header hash with
// the commit seals emptied from extraData, matching _commitSealDigest in
// BesuQBFTLightClient.sol.
func commitSealDigest(h *types.Header, extraItems []rlp.RawValue) ([]byte, error) {
	signing, err := withCommitSeals(h, extraItems, emptyRLPList)
	if err != nil {
		return nil, err
	}

	encoded, err := rlp.EncodeToBytes(signing)
	if err != nil {
		return nil, fmt.Errorf("encode signing header: %w", err)
	}

	return crypto.Keccak256(encoded), nil
}

// withCommitSeals returns a copy of h whose extraData carries seals, an RLP
// list, in place of its commit seals.
func withCommitSeals(h *types.Header, extraItems []rlp.RawValue, seals rlp.RawValue) (*types.Header, error) {
	items := slices.Clone(extraItems)
	items[extraIdxCommitSeals] = seals

	out := types.CopyHeader(h)
	var err error
	if out.Extra, err = rlp.EncodeToBytes(items); err != nil {
		return nil, fmt.Errorf("encode extra data: %w", err)
	}

	return out, nil
}

// ReadSealedHeader reads and parses the Besu QBFT header at number, or the
// head when number is nil.
func ReadSealedHeader(ctx context.Context, backend bind.ContractBackend, number *big.Int) (*ParsedHeader, error) {
	at := "latest"
	if number != nil {
		at = number.String()
	}

	header, err := backend.HeaderByNumber(ctx, number)
	if err != nil {
		return nil, fmt.Errorf("getting header %s: %w", at, err)
	}

	parsed, err := ParseSealedHeader(header)
	if err != nil {
		return nil, fmt.Errorf("parse header %s: %w", at, err)
	}

	return parsed, nil
}

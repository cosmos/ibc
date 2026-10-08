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
// with its commit seals sorted by signer, plus the few fields decoded from it
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
// with its commit seals ordered by signer as the light client requires, and
// reads the fields needed for payloads. go-ethereum keeps every optional
// post-merge field the node returned, so apart from the seal order the
// encoding reproduces the sealed bytes. Validators come from extraData, not a
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

	sorted, err := sortCommitSeals(h, extraItems)
	if err != nil {
		return nil, fmt.Errorf("header %d: %w", header.Height, err)
	}

	header.RLP, err = rlp.EncodeToBytes(sorted)
	if err != nil {
		return nil, fmt.Errorf("encode header %d: %w", header.Height, err)
	}

	return header, nil
}

// sortCommitSeals returns a copy of h whose commit seals are in strictly
// ascending order of their recovered signers. Besu does not order them that
// way, but the contract requires it; reordering is safe because the seal
// digest excludes the seals.
func sortCommitSeals(h *types.Header, extraItems []rlp.RawValue) (*types.Header, error) {
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
	signed := make([]signedSeal, len(seals))
	for i, seal := range seals {
		pubkey, sigErr := crypto.SigToPub(digest, seal)
		if sigErr != nil {
			return nil, fmt.Errorf("commit seal %d: %w", i, sigErr)
		}
		signed[i] = signedSeal{signer: crypto.PubkeyToAddress(*pubkey), seal: seal}
	}
	slices.SortFunc(signed, func(a, b signedSeal) int { return a.signer.Cmp(b.signer) })

	for i, s := range signed {
		seals[i] = s.seal
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

// SPDX-License-Identifier: Apache-2.0

package besu

import (
	"context"
	"errors"
	"fmt"
	"math/big"
	"slices"

	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/rlp"

	"github.com/cosmos/ibc-contracts/packages/go-abigen/besumsgs"
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
	// Consensus is the protocol whose digest the commit seals sign, empty for
	// a header without seals.
	Consensus Consensus
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
// consensus RPC. The consensus is the one whose digest the commit seals sign.
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

	sorted, consensus, err := normalizeCommitSeals(h, extraItems, header.Validators)
	if err != nil {
		return nil, fmt.Errorf("header %d: %w", header.Height, err)
	}
	header.Consensus = consensus

	header.RLP, err = rlp.EncodeToBytes(sorted)
	if err != nil {
		return nil, fmt.Errorf("encode header %d: %w", header.Height, err)
	}

	return header, nil
}

// normalizeCommitSeals returns h with only the commit seals the contract
// accepts: canonical, from one of validators, one per signer, in strictly
// ascending order of signer. The contract rejects the whole header for any
// other seal. Changing the seals is safe because the seal digest excludes
// them. It also returns the consensus whose digest the seals sign, which
// must be the only one under which any of them recovers to a validator.
func normalizeCommitSeals(
	h *types.Header,
	extraItems []rlp.RawValue,
	validators []common.Address,
) (*types.Header, Consensus, error) {
	var seals [][]byte
	if err := rlp.DecodeBytes(extraItems[extraIdxCommitSeals], &seals); err != nil {
		return nil, "", fmt.Errorf("commit seals: %w", err)
	}

	if len(seals) == 0 {
		return h, "", nil
	}

	var (
		detected Consensus
		kept     [][]byte
	)
	for _, consensus := range Consensuses {
		digest, err := commitSealDigest(h, extraItems, consensus)
		if err != nil {
			return nil, "", err
		}
		signed := validatorSeals(digest, seals, validators)
		if len(signed) == 0 {
			continue
		}
		if detected != "" {
			return nil, "", fmt.Errorf(
				"commit seals recover to header validators under both %s and %s",
				detected,
				consensus,
			)
		}
		detected, kept = consensus, signed
	}
	if detected == "" {
		return nil, "", errors.New("no commit seal recovers to a header validator")
	}

	sortedSeals, err := rlp.EncodeToBytes(kept)
	if err != nil {
		return nil, "", fmt.Errorf("encode commit seals: %w", err)
	}
	out, err := withCommitSeals(h, extraItems, sortedSeals)

	return out, detected, err
}

// validatorSeals returns the canonical form of the seals over digest that
// recover to one of validators, one per signer, in ascending signer order.
func validatorSeals(digest []byte, seals [][]byte, validators []common.Address) [][]byte {
	type signedSeal struct {
		signer common.Address
		seal   []byte
	}
	signed := make([]signedSeal, 0, len(seals))
	for _, seal := range seals {
		signer, canonical, err := ecrecover.Recover(digest, seal)
		if err == nil && slices.Contains(validators, signer) {
			signed = append(signed, signedSeal{signer: signer, seal: canonical})
		}
	}
	slices.SortStableFunc(signed, func(a, b signedSeal) int { return a.signer.Cmp(b.signer) })
	signed = slices.CompactFunc(signed, func(a, b signedSeal) bool { return a.signer == b.signer })

	kept := make([][]byte, len(signed))
	for i, s := range signed {
		kept[i] = s.seal
	}

	return kept
}

// commitSealDigest is the digest commit seals sign under consensus, matching
// _commitSealDigest in BesuQBFTLightClient.sol: the header hash with the
// commit seals emptied from extraData for QBFT.
func commitSealDigest(h *types.Header, extraItems []rlp.RawValue, consensus Consensus) ([]byte, error) {
	var signingItems []rlp.RawValue
	switch consensus {
	case QBFT:
		signingItems = slices.Clone(extraItems)
		signingItems[extraIdxCommitSeals] = emptyRLPList
	default:
		return nil, fmt.Errorf("no commit seal digest for besu consensus %q", consensus)
	}

	signing := types.CopyHeader(h)
	var err error
	if signing.Extra, err = rlp.EncodeToBytes(signingItems); err != nil {
		return nil, fmt.Errorf("encode signing extra data: %w", err)
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

// ReadSealedHeader reads and parses the Besu header at number, or the head
// when number is nil.
func ReadSealedHeader(
	ctx context.Context,
	backend bind.ContractBackend,
	number *big.Int,
) (*ParsedHeader, error) {
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

// SPDX-License-Identifier: Apache-2.0

package besu

import (
	"context"
	"crypto/ecdsa"
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
	// Signers are the validators whose commit seals the header keeps, in
	// ascending order: the signers the contract recovers.
	Signers []common.Address
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

	extraItems, err := decodeExtra(h)
	if err != nil {
		return nil, err
	}

	header := &ParsedHeader{Height: h.Number.Uint64(), Timestamp: h.Time, StateRoot: h.Root}
	if err = rlp.DecodeBytes(extraItems[extraIdxValidators], &header.Validators); err != nil {
		return nil, fmt.Errorf("validators: %w", err)
	}

	sorted, consensus, signers, err := normalizeCommitSeals(h, extraItems, header.Validators)
	if err != nil {
		return nil, fmt.Errorf("header %d: %w", header.Height, err)
	}
	header.Consensus, header.Signers = consensus, signers

	header.RLP, err = rlp.EncodeToBytes(sorted)
	if err != nil {
		return nil, fmt.Errorf("encode header %d: %w", header.Height, err)
	}

	return header, nil
}

// SealHeader sets h's validators and replaces its commit seals with seals
// by signers under consensus, then parses the result. It seals headers a
// chain never produced, such as misbehavior evidence. Every signer must be a
// distinct one of validators.
func SealHeader(
	h *types.Header,
	consensus Consensus,
	validators []common.Address,
	signers []*ecdsa.PrivateKey,
) (*ParsedHeader, error) {
	if h == nil {
		return nil, errors.New("nil header")
	}

	extraItems, err := decodeExtra(h)
	if err != nil {
		return nil, err
	}

	// The contract requires validators in ascending order.
	sorted := slices.SortedFunc(slices.Values(validators), common.Address.Cmp)
	if extraItems[extraIdxValidators], err = rlp.EncodeToBytes(sorted); err != nil {
		return nil, fmt.Errorf("encode validators: %w", err)
	}

	// The digest covers the validators, so it follows them.
	digest, err := commitSealDigest(h, extraItems, consensus)
	if err != nil {
		return nil, err
	}

	seals := make([][]byte, len(signers))
	signed := make([]common.Address, 0, len(signers))
	for i, key := range signers {
		signer := crypto.PubkeyToAddress(key.PublicKey)
		if !slices.Contains(sorted, signer) {
			return nil, fmt.Errorf("signer %s is not a validator", signer)
		}
		if slices.Contains(signed, signer) {
			return nil, fmt.Errorf("signer %s repeats", signer)
		}
		signed = append(signed, signer)
		if seals[i], err = crypto.Sign(digest, key); err != nil {
			return nil, fmt.Errorf("sign commit seal %d: %w", i, err)
		}
	}

	if extraItems[extraIdxCommitSeals], err = rlp.EncodeToBytes(seals); err != nil {
		return nil, fmt.Errorf("encode commit seals: %w", err)
	}

	sealed, err := withExtraData(h, extraItems)
	if err != nil {
		return nil, err
	}

	return ParseSealedHeader(sealed)
}

func decodeExtra(h *types.Header) ([]rlp.RawValue, error) {
	var extraItems []rlp.RawValue
	if err := rlp.DecodeBytes(h.Extra, &extraItems); err != nil {
		return nil, fmt.Errorf("extra data list: %w", err)
	}

	if len(extraItems) != extraDataItemCount {
		return nil, fmt.Errorf("%d extra data items, want %d", len(extraItems), extraDataItemCount)
	}

	return extraItems, nil
}

// normalizeCommitSeals returns h with only the commit seals the contract
// accepts: canonical, from one of validators, one per signer, in strictly
// ascending order of signer. The contract rejects the whole header for any
// other seal. Changing the seals is safe because the seal digest excludes
// them. It also returns the consensus whose digest the seals sign, which
// must be the only one under which any of them recovers to a validator, and
// the signers of the kept seals.
func normalizeCommitSeals(
	h *types.Header,
	extraItems []rlp.RawValue,
	validators []common.Address,
) (*types.Header, Consensus, []common.Address, error) {
	var seals [][]byte
	if err := rlp.DecodeBytes(extraItems[extraIdxCommitSeals], &seals); err != nil {
		return nil, "", nil, fmt.Errorf("commit seals: %w", err)
	}

	if len(seals) == 0 {
		return h, "", nil, nil
	}

	var (
		detected Consensus
		kept     []signedSeal
	)
	for _, consensus := range Consensuses {
		digest, err := commitSealDigest(h, extraItems, consensus)
		if err != nil {
			return nil, "", nil, err
		}
		signed := validatorSeals(digest, seals, validators)
		if len(signed) == 0 {
			continue
		}
		if detected != "" {
			return nil, "", nil, fmt.Errorf(
				"commit seals recover to header validators under both %s and %s",
				detected,
				consensus,
			)
		}
		detected, kept = consensus, signed
	}
	if detected == "" {
		return nil, "", nil, errors.New("no commit seal recovers to a header validator")
	}

	keptSeals := make([][]byte, len(kept))
	signers := make([]common.Address, len(kept))
	for i, s := range kept {
		keptSeals[i], signers[i] = s.seal, s.signer
	}
	items := slices.Clone(extraItems)
	var err error
	if items[extraIdxCommitSeals], err = rlp.EncodeToBytes(keptSeals); err != nil {
		return nil, "", nil, fmt.Errorf("encode commit seals: %w", err)
	}
	out, err := withExtraData(h, items)

	return out, detected, signers, err
}

type signedSeal struct {
	signer common.Address
	seal   []byte
}

// validatorSeals returns the canonical form of the seals over digest that
// recover to one of validators, one per signer, in ascending signer order.
func validatorSeals(digest []byte, seals [][]byte, validators []common.Address) []signedSeal {
	signed := make([]signedSeal, 0, len(seals))
	for _, seal := range seals {
		signer, canonical, err := ecrecover.Recover(digest, seal)
		if err == nil && slices.Contains(validators, signer) {
			signed = append(signed, signedSeal{signer: signer, seal: canonical})
		}
	}
	slices.SortStableFunc(signed, func(a, b signedSeal) int { return a.signer.Cmp(b.signer) })

	return slices.CompactFunc(signed, func(a, b signedSeal) bool { return a.signer == b.signer })
}

// commitSealDigest is the digest commit seals sign under consensus, matching
// _commitSealDigest in BesuQBFTLightClient.sol and BesuIBFT2LightClient.sol:
// the header hash with the commit seals emptied from extraData for QBFT, or
// dropped from it for IBFT2.
func commitSealDigest(h *types.Header, extraItems []rlp.RawValue, consensus Consensus) ([]byte, error) {
	var signingItems []rlp.RawValue
	switch consensus {
	case QBFT:
		signingItems = slices.Clone(extraItems)
		signingItems[extraIdxCommitSeals] = emptyRLPList
	case IBFT2:
		signingItems = extraItems[:extraIdxCommitSeals]
	default:
		return nil, fmt.Errorf("no commit seal digest for besu consensus %q", consensus)
	}

	signing, err := withExtraData(h, signingItems)
	if err != nil {
		return nil, err
	}

	encoded, err := rlp.EncodeToBytes(signing)
	if err != nil {
		return nil, fmt.Errorf("encode signing header: %w", err)
	}

	return crypto.Keccak256(encoded), nil
}

// withExtraData returns a copy of h whose extraData is the RLP list of items.
func withExtraData(h *types.Header, items []rlp.RawValue) (*types.Header, error) {
	out := types.CopyHeader(h)
	var err error
	if out.Extra, err = rlp.EncodeToBytes(items); err != nil {
		return nil, fmt.Errorf("encode extra data: %w", err)
	}

	return out, nil
}

// ReadSealedHeader reads and parses the Besu header at number, or the head
// when number is nil.
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

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

// Besu BFT extraData layout, matching Header.decodeRlp in
// ibc-contracts/ibc-solidity/contracts/light-clients/besu/utils/Header.sol.
const (
	extraDataItemCount  = 5
	extraIdxValidators  = 1
	extraIdxCommitSeals = 4
)

// emptyRLPList replaces the commit seals in the QBFT signing form of extraData.
var emptyRLPList = rlp.RawValue{0xc0}

// ParsedHeader is a Besu header and its commit seals in the form the light
// client accepts, plus the few fields decoded from it that light-client
// payloads need; consensus validity is checked by the contract.
type ParsedHeader struct {
	// RLP is the header in its commit seal signing form, whose hash the seals
	// sign: extraData without the seal list for IBFT2, or with it emptied for
	// QBFT.
	RLP []byte
	// CommitSeals are the Signers' seals in the same order, canonical, with
	// 27/28 recovery IDs.
	CommitSeals [][]byte
	Height      uint64
	Timestamp   uint64
	StateRoot   common.Hash
	Validators  []common.Address
	// Signers are the validators whose commit seals the header keeps, in
	// ascending order: the signers the contract recovers.
	Signers []common.Address
	// Consensus is the protocol whose digest the commit seals sign.
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

// ParseSealedHeader encodes a node-supplied header in the signing form its
// commit seals sign, takes the seals in the form the light client requires,
// and reads the fields needed for payloads. go-ethereum keeps every optional
// post-merge field the node returned, so the encoding reproduces the signed
// bytes. Validators come from extraData, not a consensus RPC. The consensus
// is the one whose digest the commit seals sign.
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

	consensus, seals, signers, err := normalizeCommitSeals(h, extraItems, header.Validators)
	if err != nil {
		return nil, fmt.Errorf("header %d: %w", header.Height, err)
	}
	header.Consensus, header.CommitSeals, header.Signers = consensus, seals, signers

	if header.RLP, err = signingHeader(h, extraItems, consensus); err != nil {
		return nil, fmt.Errorf("encode header %d: %w", header.Height, err)
	}

	return header, nil
}

// HeightNotAfter is a height at most head's whose header has a timestamp at
// most notAfter, or 1 when there is none, since genesis has no seals. The
// light client requires blocks at least a second apart, so it steps back one
// block per second head is later than notAfter; with slower blocks it may
// land below the newest such header.
func HeightNotAfter(head *ParsedHeader, notAfter uint64) uint64 {
	if head.Timestamp <= notAfter || head.Height <= 1 {
		return head.Height
	}
	return head.Height - min(head.Height-1, head.Timestamp-notAfter)
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

// normalizeCommitSeals returns the consensus whose digest h's commit seals
// sign, which must be the only one under which any of them recovers to a
// validator, and only the seals the contract accepts: canonical with 27/28
// recovery IDs, from one of validators, one per signer, in strictly
// ascending order of signer, with those signers. The contract rejects the
// whole update for any other seal.
func normalizeCommitSeals(
	h *types.Header,
	extraItems []rlp.RawValue,
	validators []common.Address,
) (Consensus, [][]byte, []common.Address, error) {
	var seals [][]byte
	if err := rlp.DecodeBytes(extraItems[extraIdxCommitSeals], &seals); err != nil {
		return "", nil, nil, fmt.Errorf("commit seals: %w", err)
	}

	if len(seals) == 0 {
		return "", nil, nil, errors.New("no commit seals")
	}

	var (
		detected Consensus
		kept     []signedSeal
	)
	for _, consensus := range Consensuses {
		digest, err := commitSealDigest(h, extraItems, consensus)
		if err != nil {
			return "", nil, nil, err
		}
		signed := validatorSeals(digest, seals, validators)
		if len(signed) == 0 {
			continue
		}
		if detected != "" {
			return "", nil, nil, fmt.Errorf(
				"commit seals recover to header validators under both %s and %s",
				detected,
				consensus,
			)
		}
		detected, kept = consensus, signed
	}
	if detected == "" {
		return "", nil, nil, errors.New("no commit seal recovers to a header validator")
	}

	keptSeals := make([][]byte, len(kept))
	signers := make([]common.Address, len(kept))
	for i, s := range kept {
		keptSeals[i], signers[i] = s.seal, s.signer
	}

	return detected, keptSeals, signers, nil
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

// commitSealDigest is the digest commit seals sign under consensus, the hash
// of h's signing form.
func commitSealDigest(h *types.Header, extraItems []rlp.RawValue, consensus Consensus) ([]byte, error) {
	signing, err := signingHeader(h, extraItems, consensus)
	if err != nil {
		return nil, err
	}

	return crypto.Keccak256(signing), nil
}

// signingHeader is the RLP of h in the form commit seals sign under
// consensus: the commit seals emptied from extraData for QBFT, or dropped
// from it for IBFT2.
func signingHeader(h *types.Header, extraItems []rlp.RawValue, consensus Consensus) ([]byte, error) {
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

	return encoded, nil
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

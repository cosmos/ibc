// SPDX-License-Identifier: Apache-2.0

package besutest

import (
	"crypto/ecdsa"
	"encoding/binary"
	"fmt"
	"slices"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/rlp"

	"github.com/cosmos/ibc/cli/besu"
	"github.com/cosmos/ibc/cli/besu/internal/headerlayout"
)

// Builder mutates a real Besu QBFT header and re-seals it with test keys, so
// tests can exercise validator turnover, heights and timestamps the fixture
// does not contain.
type Builder struct {
	items      []rlp.RawValue
	extraItems []rlp.RawValue
}

// NewBuilder starts from an existing header's RLP, typically a fixture header.
func NewBuilder(headerRLP []byte) (*Builder, error) {
	var items []rlp.RawValue
	if err := rlp.DecodeBytes(headerRLP, &items); err != nil {
		return nil, fmt.Errorf("decode header: %w", err)
	}

	if len(items) < headerlayout.MinHeaderItems {
		return nil, fmt.Errorf("header has %d items, want at least %d", len(items), headerlayout.MinHeaderItems)
	}

	var extraData []byte
	if err := rlp.DecodeBytes(items[headerlayout.IdxExtraData], &extraData); err != nil {
		return nil, fmt.Errorf("decode extra data: %w", err)
	}

	var extraItems []rlp.RawValue
	if err := rlp.DecodeBytes(extraData, &extraItems); err != nil {
		return nil, fmt.Errorf("decode extra data list: %w", err)
	}

	if len(extraItems) != headerlayout.ExtraDataItemCount {
		return nil, fmt.Errorf("extra data has %d items, want %d", len(extraItems), headerlayout.ExtraDataItemCount)
	}

	return &Builder{items: items, extraItems: extraItems}, nil
}

// MustBuilder is NewBuilder that panics, for test setup.
func MustBuilder(headerRLP []byte) *Builder {
	builder, err := NewBuilder(headerRLP)
	if err != nil {
		panic(err)
	}

	return builder
}

func (b *Builder) SetHeight(height uint64) *Builder {
	b.items[headerlayout.IdxNumber] = mustRLP(height)
	return b
}

func (b *Builder) SetTimestamp(timestamp uint64) *Builder {
	b.items[headerlayout.IdxTimestamp] = mustRLP(timestamp)
	return b
}

// SetValidators writes validators in ascending address order, as Besu does.
func (b *Builder) SetValidators(validators []common.Address) *Builder {
	sorted := slices.Clone(validators)
	slices.SortFunc(sorted, func(a, b common.Address) int { return a.Cmp(b) })
	b.extraItems[headerlayout.ExtraIdxValidators] = mustRLP(sorted)
	return b
}

func (b *Builder) SetCommitSeals(seals [][]byte) *Builder {
	b.extraItems[headerlayout.ExtraIdxCommitSeals] = mustRLP(seals)
	return b
}

// Sign replaces the commit seals with one seal per key over the header's QBFT
// digest, in key order.
func (b *Builder) Sign(keys ...*ecdsa.PrivateKey) (*Builder, error) {
	header, err := b.Header()
	if err != nil {
		return nil, err
	}

	digest, err := header.CommitSealDigest()
	if err != nil {
		return nil, err
	}

	seals := make([][]byte, len(keys))
	for i, key := range keys {
		seals[i], err = crypto.Sign(digest.Bytes(), key)
		if err != nil {
			return nil, fmt.Errorf("sign seal %d: %w", i, err)
		}
	}

	return b.SetCommitSeals(seals), nil
}

// MustSign is Sign that panics, for test setup.
func (b *Builder) MustSign(keys ...*ecdsa.PrivateKey) *Builder {
	signed, err := b.Sign(keys...)
	if err != nil {
		panic(err)
	}

	return signed
}

// Encode returns the header RLP.
func (b *Builder) Encode() ([]byte, error) {
	extraData, err := rlp.EncodeToBytes(b.extraItems)
	if err != nil {
		return nil, fmt.Errorf("encode extra data: %w", err)
	}

	items := make([]rlp.RawValue, len(b.items))
	copy(items, b.items)

	items[headerlayout.IdxExtraData], err = rlp.EncodeToBytes(extraData)
	if err != nil {
		return nil, fmt.Errorf("encode extra data field: %w", err)
	}

	encoded, err := rlp.EncodeToBytes(items)
	if err != nil {
		return nil, fmt.Errorf("encode header: %w", err)
	}

	return encoded, nil
}

// MustEncode is Encode that panics, for test setup.
func (b *Builder) MustEncode() []byte {
	encoded, err := b.Encode()
	if err != nil {
		panic(err)
	}

	return encoded
}

// Header encodes and parses the current state.
func (b *Builder) Header() (*besu.Header, error) {
	encoded, err := b.Encode()
	if err != nil {
		return nil, err
	}

	return besu.ParseHeader(encoded)
}

// Keys derives n distinct deterministic secp256k1 keys.
func Keys(n int) []*ecdsa.PrivateKey {
	keys := make([]*ecdsa.PrivateKey, n)
	for i := range keys {
		seed := make([]byte, 32)
		binary.BigEndian.PutUint64(seed[24:], uint64(i)+1) //nolint:gosec // small positive test index

		key, err := crypto.ToECDSA(seed)
		if err != nil {
			panic(err)
		}

		keys[i] = key
	}

	return keys
}

// Addresses maps keys to their addresses, preserving order.
func Addresses(keys []*ecdsa.PrivateKey) []common.Address {
	addresses := make([]common.Address, len(keys))
	for i, key := range keys {
		addresses[i] = crypto.PubkeyToAddress(key.PublicKey)
	}

	return addresses
}

func mustRLP(value any) rlp.RawValue {
	encoded, err := rlp.EncodeToBytes(value)
	if err != nil {
		panic(err)
	}

	return encoded
}

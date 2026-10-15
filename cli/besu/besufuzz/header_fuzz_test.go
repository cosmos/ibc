// SPDX-License-Identifier: Apache-2.0

package besufuzz

import (
	"crypto/ecdsa"
	"encoding/binary"
	"math/big"
	"slices"
	"sync"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/rlp"
	"github.com/stretchr/testify/require"

	"github.com/cosmos/ibc/cli/besu"
	"github.com/cosmos/ibc/cli/besu/besutest"
)

// Besu QBFT extraData: vanity, validators, vote, round, commit seals.
const (
	extraIdxValidators  = 1
	extraIdxCommitSeals = 4
)

var emptyRLPList = rlp.RawValue{0xc0}

// FuzzParseSealedHeader parses node-supplied extraData. Whatever it accepts
// must keep the commit seal digest and only reorder the seals.
func FuzzParseSealedHeader(f *testing.F) {
	for _, seed := range seedExtras() {
		f.Add(seed, uint64(1), uint64(1_700_000_000))
	}

	f.Fuzz(func(t *testing.T, extra []byte, number, timestamp uint64) {
		header := newHeader(number, timestamp)
		header.Extra = extra

		parsed, err := besu.ParseSealedHeader(header)
		if err != nil {
			return
		}

		checkParsedHeader(t, header, parsed)
	})
}

// FuzzParseSealedHeaderSigned builds validly sealed headers, which raw
// extraData almost never is, and checks the seals come out in signer order.
func FuzzParseSealedHeaderSigned(f *testing.F) {
	f.Add([]byte("seed"), uint8(4), uint64(1), uint64(1_700_000_000))
	f.Add([]byte{}, uint8(1), uint64(0), uint64(0))

	f.Fuzz(func(t *testing.T, keySeed []byte, count uint8, number, timestamp uint64) {
		keys := deriveKeys(keySeed, int(count%8)+1)
		header := newHeader(number, timestamp)

		var err error
		header.Extra, err = sealedExtra(header, keys)
		require.NoError(t, err)

		parsed, err := besu.ParseSealedHeader(header)
		require.NoError(t, err, "parsing validly sealed header")

		checkParsedHeader(t, header, parsed)

		want := make([]common.Address, len(keys))
		for i, key := range keys {
			want[i] = crypto.PubkeyToAddress(key.PublicKey)
		}
		require.Equal(t, want, parsed.Validators)

		signers := sealSigners(t, parsed.RLP)
		for i := 1; i < len(signers); i++ {
			require.Negative(t, signers[i-1].Cmp(signers[i]), "seal signers not strictly ascending: %v", signers)
		}
	})
}

func checkParsedHeader(t *testing.T, header *types.Header, parsed *besu.ParsedHeader) {
	t.Helper()

	var decoded types.Header
	require.NoError(t, rlp.DecodeBytes(parsed.RLP, &decoded), "parsed RLP does not decode")
	require.Equal(t, header.Number.Uint64(), parsed.Height)
	require.Equal(t, parsed.Height, decoded.Number.Uint64())
	require.Equal(t, parsed.Timestamp, decoded.Time)
	require.Equal(t, parsed.StateRoot, decoded.Root)

	inputItems, inputSeals := splitExtra(t, header.Extra)
	outputItems, outputSeals := splitExtra(t, decoded.Extra)
	require.Equal(
		t, sealDigest(t, header, inputItems), sealDigest(t, &decoded, outputItems),
		"reordering seals changed the commit seal digest",
	)
	require.ElementsMatch(t, inputSeals, outputSeals, "seals were changed, not just reordered")

	reparsed, err := besu.ParseSealedHeader(&decoded)
	require.NoError(t, err, "reparsing parsed header")
	require.Equal(t, parsed.RLP, reparsed.RLP, "parsing is not idempotent")
}

func splitExtra(t *testing.T, extra []byte) ([]rlp.RawValue, [][]byte) {
	t.Helper()

	var items []rlp.RawValue
	require.NoError(t, rlp.DecodeBytes(extra, &items))
	var seals [][]byte
	require.NoError(t, rlp.DecodeBytes(items[extraIdxCommitSeals], &seals))

	return items, seals
}

// sealDigest is the hash commit seals sign: the header with no seals.
func sealDigest(t *testing.T, header *types.Header, items []rlp.RawValue) common.Hash {
	t.Helper()

	items = slices.Clone(items)
	items[extraIdxCommitSeals] = emptyRLPList
	signing := types.CopyHeader(header)
	var err error
	signing.Extra, err = rlp.EncodeToBytes(items)
	require.NoError(t, err)

	return signing.Hash()
}

func sealSigners(t *testing.T, headerRLP []byte) []common.Address {
	t.Helper()

	var header types.Header
	require.NoError(t, rlp.DecodeBytes(headerRLP, &header))
	items, seals := splitExtra(t, header.Extra)
	digest := sealDigest(t, &header, items)

	signers := make([]common.Address, len(seals))
	for i, seal := range seals {
		pubkey, err := crypto.SigToPub(digest[:], seal)
		require.NoError(t, err, "seal %d", i)
		signers[i] = crypto.PubkeyToAddress(*pubkey)
	}

	return signers
}

func newHeader(number, timestamp uint64) *types.Header {
	return &types.Header{
		Number:     new(big.Int).SetUint64(number),
		Time:       timestamp,
		Difficulty: big.NewInt(1),
		Root:       common.HexToHash("0x01"),
	}
}

// deriveKeys derives count distinct private keys from seed.
func deriveKeys(seed []byte, count int) []*ecdsa.PrivateKey {
	keys := make([]*ecdsa.PrivateKey, 0, count)
	for i := uint64(0); len(keys) < count; i++ {
		key, err := crypto.ToECDSA(crypto.Keccak256(seed, binary.BigEndian.AppendUint64(nil, i)))
		if err == nil {
			keys = append(keys, key)
		}
	}

	return keys
}

// sealedExtra is QBFT extraData naming keys as validators, with a commit seal
// from each in key order.
func sealedExtra(header *types.Header, keys []*ecdsa.PrivateKey) ([]byte, error) {
	validators := make([]common.Address, len(keys))
	for i, key := range keys {
		validators[i] = crypto.PubkeyToAddress(key.PublicKey)
	}

	vanity, err := rlp.EncodeToBytes(make([]byte, 32))
	if err != nil {
		return nil, err
	}
	encodedValidators, err := rlp.EncodeToBytes(validators)
	if err != nil {
		return nil, err
	}
	round, err := rlp.EncodeToBytes(uint32(0))
	if err != nil {
		return nil, err
	}
	items := []rlp.RawValue{vanity, encodedValidators, emptyRLPList, round, emptyRLPList}

	signing := types.CopyHeader(header)
	if signing.Extra, err = rlp.EncodeToBytes(items); err != nil {
		return nil, err
	}
	digest := signing.Hash()

	seals := make([][]byte, len(keys))
	for i, key := range keys {
		if seals[i], err = crypto.Sign(digest[:], key); err != nil {
			return nil, err
		}
	}
	if items[extraIdxCommitSeals], err = rlp.EncodeToBytes(seals); err != nil {
		return nil, err
	}

	return rlp.EncodeToBytes(items)
}

// seedExtras is built once: libFuzzer builds re-run the fuzz function for
// every input.
var seedExtras = sync.OnceValue(func() [][]byte {
	seeds := [][]byte{{0xc0}, {0x80}}

	if fixture, err := besutest.LoadFixture(); err == nil {
		for _, update := range []besutest.UpdateFixture{fixture.AdjacentUpdate, fixture.NonAdjacentUpdate} {
			var header types.Header
			if rlp.DecodeBytes(update.HeaderRLP, &header) == nil {
				seeds = append(seeds, header.Extra)
			}
		}
	}

	header := newHeader(1, 1_700_000_000)
	for _, count := range []int{1, 4} {
		if extra, err := sealedExtra(header, deriveKeys([]byte("seed"), count)); err == nil {
			seeds = append(seeds, extra)
		}
	}

	return seeds
})

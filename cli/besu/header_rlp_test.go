// SPDX-License-Identifier: Apache-2.0

package besu_test

import (
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/rlp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cosmos/ibc/cli/besu"
	"github.com/cosmos/ibc/cli/besu/besutest"
)

func TestParseHeaderFixture(t *testing.T) {
	fixture := besutest.MustFixture(t)

	for _, update := range []besutest.UpdateFixture{fixture.AdjacentUpdate, fixture.NonAdjacentUpdate} {
		header, err := besu.ParseHeader(update.HeaderRLP)
		require.NoError(t, err)

		assert.Equal(t, update.Height, header.Height)
		assert.Equal(t, update.ExpectedTimestamp, header.Timestamp)
		assert.Equal(t, update.ExpectedValidators, header.Validators)
		assert.Equal(t, []byte(update.HeaderRLP), header.RLP)
	}
}

func TestParseHeaderRejectsMalformed(t *testing.T) {
	fixture := besutest.MustFixture(t)
	template := fixture.AdjacentUpdate.HeaderRLP

	t.Run("not a list", func(t *testing.T) {
		_, err := besu.ParseHeader([]byte{0x80})
		require.ErrorIs(t, err, besu.ErrInvalidHeader)
	})

	t.Run("invalid state root", func(t *testing.T) {
		var items []rlp.RawValue
		require.NoError(t, rlp.DecodeBytes(template, &items))
		for _, root := range []any{make([]byte, 31), make([]byte, 33), []any{}} {
			var err error
			items[3], err = rlp.EncodeToBytes(root)
			require.NoError(t, err)
			raw, err := rlp.EncodeToBytes(items)
			require.NoError(t, err)
			_, err = besu.ParseHeader(raw)
			require.ErrorIs(t, err, besu.ErrInvalidHeader)
		}
	})
}

func TestParseSealedHeader(t *testing.T) {
	fixture := besutest.MustFixture(t)
	update := fixture.AdjacentUpdate
	var header types.Header
	require.NoError(t, rlp.DecodeBytes(update.HeaderRLP, &header))

	got, err := besu.ParseSealedHeader(&header)
	require.NoError(t, err)
	assert.Equal(t, []byte(update.HeaderRLP), got.RLP)
	assert.Equal(t, update.Height, got.Height)
	assert.Equal(t, update.ExpectedTimestamp, got.Timestamp)
	assert.Equal(t, update.ExpectedStateRoot, got.StateRoot)
	assert.Equal(t, update.ExpectedValidators, got.Validators)

	_, err = besu.ParseSealedHeader(nil)
	require.Error(t, err)
}

func TestParseHeaderDefersBFTValidationToContract(t *testing.T) {
	template := besutest.MustFixture(t).AdjacentUpdate.HeaderRLP
	for _, tc := range []struct {
		name  string
		index int
		value any
	}{
		{"ommers hash", 1, common.Hash{}},
		{"difficulty", 7, uint64(0)},
		{"mix hash", 13, common.Hash{}},
		{"nonce", 14, types.BlockNonce{1}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var items []rlp.RawValue
			require.NoError(t, rlp.DecodeBytes(template, &items))
			var err error
			items[tc.index], err = rlp.EncodeToBytes(tc.value)
			require.NoError(t, err)
			raw, err := rlp.EncodeToBytes(items)
			require.NoError(t, err)
			header, err := besu.ParseHeader(raw)
			require.NoError(t, err)
			require.Equal(t, raw, header.RLP)
		})
	}
}

// The parser extracts validator addresses without imposing consensus rules.
func TestParseHeaderDefersValidatorValidationToContract(t *testing.T) {
	template := besutest.MustFixture(t).AdjacentUpdate.HeaderRLP
	a, b := common.HexToAddress("0x01"), common.HexToAddress("0x02")
	for name, validators := range map[string][]common.Address{
		"empty":     nil,
		"zero":      {{}},
		"duplicate": {a, a},
		"unsorted":  {b, a},
	} {
		t.Run(name, func(t *testing.T) {
			var items []rlp.RawValue
			require.NoError(t, rlp.DecodeBytes(template, &items))
			var extra []byte
			require.NoError(t, rlp.DecodeBytes(items[12], &extra))
			var fields []rlp.RawValue
			require.NoError(t, rlp.DecodeBytes(extra, &fields))
			var err error
			fields[1], err = rlp.EncodeToBytes(validators)
			require.NoError(t, err)
			extra, err = rlp.EncodeToBytes(fields)
			require.NoError(t, err)
			items[12], err = rlp.EncodeToBytes(extra)
			require.NoError(t, err)
			raw, err := rlp.EncodeToBytes(items)
			require.NoError(t, err)
			header, err := besu.ParseHeader(raw)
			require.NoError(t, err)
			require.Equal(t, raw, header.RLP)
			require.Len(t, header.Validators, len(validators))
			for i, validator := range validators {
				require.Equal(t, validator, header.Validators[i])
			}
		})
	}
}

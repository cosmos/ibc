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

func TestParseSealedHeaderFixture(t *testing.T) {
	fixture := besutest.MustFixture(t)

	for _, update := range []besutest.UpdateFixture{fixture.AdjacentUpdate, fixture.NonAdjacentUpdate} {
		header := besutest.ParseHeader(t, update.HeaderRLP)

		assert.Equal(t, update.Height, header.Height)
		assert.Equal(t, update.ExpectedTimestamp, header.Timestamp)
		assert.Equal(t, update.ExpectedStateRoot, header.StateRoot)
		assert.Equal(t, update.ExpectedValidators, header.Validators)
		assert.Equal(t, []byte(update.HeaderRLP), header.RLP)
	}
}

func TestParseSealedHeaderRejectsMalformed(t *testing.T) {
	var header types.Header
	require.NoError(t, rlp.DecodeBytes(besutest.MustFixture(t).AdjacentUpdate.HeaderRLP, &header))

	_, err := besu.ParseSealedHeader(nil)
	require.ErrorContains(t, err, "nil header")

	header.Extra = []byte{0x80}
	_, err = besu.ParseSealedHeader(&header)
	require.ErrorContains(t, err, "extra data list")
}

func TestParseSealedHeaderDefersBFTValidationToContract(t *testing.T) {
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
			header := besutest.ParseHeader(t, raw)
			require.Equal(t, raw, header.RLP)
		})
	}
}

// The parser extracts validator addresses without imposing consensus rules.
func TestParseSealedHeaderDefersValidatorValidationToContract(t *testing.T) {
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
			header := besutest.ParseHeader(t, raw)
			require.Equal(t, raw, header.RLP)
			require.Len(t, header.Validators, len(validators))
			for i, validator := range validators {
				require.Equal(t, validator, header.Validators[i])
			}
		})
	}
}

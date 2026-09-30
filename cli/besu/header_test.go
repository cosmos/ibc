// SPDX-License-Identifier: Apache-2.0

package besu_test

import (
	"testing"

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

	header.Extra, err = rlp.EncodeToBytes([][]byte{{}, {}, {}, {}})
	require.NoError(t, err)
	_, err = besu.ParseSealedHeader(&header)
	require.ErrorContains(t, err, "4 extra data items, want 5")
}

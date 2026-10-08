// SPDX-License-Identifier: Apache-2.0

package besu

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/rlp"
	"github.com/stretchr/testify/require"
)

// TestCommitSealDigestRecoversValidators checks the digest against the
// header's own validator set: with a wrong digest the fixture's seals would
// recover to addresses outside it. besutest imports this package, so the
// fixture is read directly.
func TestCommitSealDigestRecoversValidators(t *testing.T) {
	type update struct {
		HeaderRLP          hexutil.Bytes    `json:"headerRlp"`
		ExpectedValidators []common.Address `json:"expectedValidators"`
	}
	var fixture struct {
		AdjacentUpdate    update `json:"adjacentUpdate"`
		NonAdjacentUpdate update `json:"nonAdjacentUpdate"`
	}
	raw, err := os.ReadFile("besutest/testdata/qbft.json")
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal(raw, &fixture))

	for _, u := range []update{fixture.AdjacentUpdate, fixture.NonAdjacentUpdate} {
		var header types.Header
		require.NoError(t, rlp.DecodeBytes(u.HeaderRLP, &header))
		var extraItems []rlp.RawValue
		require.NoError(t, rlp.DecodeBytes(header.Extra, &extraItems))
		var seals [][]byte
		require.NoError(t, rlp.DecodeBytes(extraItems[extraIdxCommitSeals], &seals))
		require.NotEmpty(t, seals)

		digest, err := commitSealDigest(&header, extraItems)
		require.NoError(t, err)

		var previous common.Address
		for i, seal := range seals {
			pubkey, err := crypto.SigToPub(digest, seal)
			require.NoError(t, err)
			signer := crypto.PubkeyToAddress(*pubkey)
			require.Contains(t, u.ExpectedValidators, signer, "seal %d", i)
			// The fixture is in the order the contract accepts.
			require.Positive(t, signer.Cmp(previous), "seal %d out of order", i)
			previous = signer
		}
	}
}

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

// TestCommitSealDigestRecoversValidators checks each consensus's digest
// against the header's own validator set. besutest imports this package, so
// the fixtures are read directly.
func TestCommitSealDigestRecoversValidators(t *testing.T) {
	for _, consensus := range Consensuses {
		t.Run(string(consensus), func(t *testing.T) {
			testCommitSealDigestRecoversValidators(t, consensus)
		})
	}
}

func testCommitSealDigestRecoversValidators(t *testing.T, consensus Consensus) {
	type update struct {
		HeaderRLP          hexutil.Bytes    `json:"headerRlp"`
		ExpectedValidators []common.Address `json:"expectedValidators"`
	}
	var fixture struct {
		AdjacentUpdate    update `json:"adjacentUpdate"`
		NonAdjacentUpdate update `json:"nonAdjacentUpdate"`
	}
	raw, err := os.ReadFile("besutest/testdata/" + string(consensus) + ".json")
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

		digest, err := commitSealDigest(&header, extraItems, consensus)
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

// CommitSealDigest exposes commitSealDigest to FuzzParseSealedHeader, which is
// in besu_test so it can seed from besutest.
var CommitSealDigest = commitSealDigest

// SPDX-License-Identifier: Apache-2.0

package besu

import (
	"encoding/json"
	"os"
	"slices"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/rlp"
	"github.com/stretchr/testify/require"
)

// TestCommitSealDigestRecoversValidators checks that each consensus's
// signing form of a node's header is the fixture's header RLP, and that the
// fixture's seals recover from its hash to the header's own validators.
// besutest imports this package, so the fixtures are read directly.
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
		CommitSeals        []hexutil.Bytes  `json:"commitSeals"`
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
		require.NotEmpty(t, u.CommitSeals)
		var header types.Header
		require.NoError(t, rlp.DecodeBytes(u.HeaderRLP, &header))
		var extraItems []rlp.RawValue
		require.NoError(t, rlp.DecodeBytes(header.Extra, &extraItems))

		// The node's header carries the seals, with Besu's 0/1 recovery IDs.
		seals := make([][]byte, len(u.CommitSeals))
		for i, seal := range u.CommitSeals {
			seals[i] = slices.Clone(seal)
			seals[i][crypto.RecoveryIDOffset] -= 27
		}
		encoded, err := rlp.EncodeToBytes(seals)
		require.NoError(t, err)
		extraItems = append(extraItems[:extraIdxCommitSeals], encoded)

		signing, err := signingHeader(&header, extraItems, consensus)
		require.NoError(t, err)
		require.Equal(t, []byte(u.HeaderRLP), signing)
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

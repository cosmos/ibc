// SPDX-License-Identifier: Apache-2.0

package attestation

import (
	"context"
	"crypto/ecdsa"
	"log/slog"
	"math/big"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	attestorevm "github.com/cosmos/ibc/cli/attestor/evm"
	"github.com/cosmos/ibc/cli/internal/chains"
	"github.com/cosmos/ibc/cli/internal/service/attestor"
	"github.com/cosmos/ibc/cli/internal/tests/mocks"
	v2 "github.com/cosmos/ibc/cli/internal/types/v2"
)

// testKey derives a signing key from name, so a test's attestation set can be
// built from attestor names.
func testKey(t *testing.T, name string) *ecdsa.PrivateKey {
	t.Helper()

	key, err := crypto.ToECDSA(crypto.Keccak256([]byte(name)))
	require.NoError(t, err)

	return key
}

// signerSet is an attestation set holding the testKey signer of each name.
func signerSet(t *testing.T, names ...string) map[common.Address]struct{} {
	t.Helper()

	set := make(map[common.Address]struct{}, len(names))
	for _, name := range names {
		set[crypto.PubkeyToAddress(testKey(t, name).PublicKey)] = struct{}{}
	}

	return set
}

// newTestGenerator builds a Generator whose attestation set holds every
// attestor's testKey signer.
func newTestGenerator(t *testing.T, attestors []attestor.Attestor, threshold int, chain chains.Client) *Generator {
	t.Helper()

	names := make([]string, len(attestors))
	for i, a := range attestors {
		names[i] = a.Name()
	}

	set := &attestationSet{signers: signerSet(t, names...), threshold: threshold}

	return newGenerator("client-chain", "client-0", attestors, set, chain, slog.Default())
}

// signedAttestor builds a attestor.MockAttestor whose StateAttestation call
// returns attestedData signed by name's testKey over the correct
// domain-tagged digest.
func signedAttestor(t *testing.T, name string, attestedData []byte) *attestor.MockAttestor {
	t.Helper()

	key := testKey(t, name)
	digest := attestorevm.Digest(attestorevm.TagStateAttestation, attestedData)
	sig, err := crypto.Sign(digest[:], key)
	require.NoError(t, err)

	a := attestor.NewMockAttestor(t)
	a.EXPECT().Name().Return(name).Maybe()
	a.EXPECT().Address().Return(crypto.PubkeyToAddress(key.PublicKey).Hex()).Maybe()
	a.EXPECT().StateAttestation(mock.Anything, mock.Anything).Return(
		attestor.Attestation{Height: 10, AttestedData: attestedData, Signature: sig}, nil,
	)

	return a
}

func TestQueryQuorum(t *testing.T) {
	ctx := context.Background()
	data := []byte("attested state data")

	t.Run("quorumMet", func(t *testing.T) {
		attestors := []attestor.Attestor{
			signedAttestor(t, "a1", data),
			signedAttestor(t, "a2", data),
		}

		result, err := newTestGenerator(t, attestors, 2, nil).queryStateQuorum(ctx, 10, data)
		require.NoError(t, err)
		require.Equal(t, data, result.AttestationData)
		require.Len(t, result.Signatures, 2)
	})

	t.Run("wrongClaimQuorumRejected", func(t *testing.T) {
		attestors := []attestor.Attestor{
			signedAttestor(t, "a1", []byte("wrong claim")),
			signedAttestor(t, "a2", []byte("wrong claim")),
		}
		_, err := newTestGenerator(t, attestors, 2, nil).queryStateQuorum(ctx, 10, data)
		require.ErrorContains(t, err, "quorum not met")
		require.ErrorContains(t, err, "attested data does not match expected claim")
	})

	t.Run("validQuorumWithWrongClaimQuorum", func(t *testing.T) {
		attestors := []attestor.Attestor{
			signedAttestor(t, "wrong1", []byte("wrong claim")),
			signedAttestor(t, "wrong2", []byte("wrong claim")),
			signedAttestor(t, "valid1", data),
			signedAttestor(t, "valid2", data),
		}
		result, err := newTestGenerator(t, attestors, 2, nil).queryStateQuorum(ctx, 10, data)
		require.NoError(t, err)
		require.Equal(t, data, result.AttestationData)
		require.Len(t, result.Signatures, 2)
	})

	t.Run("quorumNotMet", func(t *testing.T) {
		attestors := []attestor.Attestor{
			signedAttestor(t, "a1", data),
		}

		_, err := newTestGenerator(t, attestors, 2, nil).queryStateQuorum(ctx, 10, data)
		require.ErrorContains(t, err, "quorum not met")
	})

	t.Run("mismatchedDataExcluded", func(t *testing.T) {
		attestors := []attestor.Attestor{
			signedAttestor(t, "a1", data),
			signedAttestor(t, "a2", []byte("a different claim entirely")),
		}

		_, err := newTestGenerator(t, attestors, 2, nil).queryStateQuorum(ctx, 10, data)
		require.ErrorContains(
			t,
			err,
			"quorum not met",
			"the second attestor's disagreeing claim must not count toward quorum",
		)
	})

	t.Run("majorityQuorumMetDespiteFirstAttestorDisagreeing", func(t *testing.T) {
		attestors := []attestor.Attestor{
			signedAttestor(t, "a1", []byte("stale claim")),
			signedAttestor(t, "a2", data),
			signedAttestor(t, "a3", data),
		}

		result, err := newTestGenerator(t, attestors, 2, nil).queryStateQuorum(ctx, 10, data)
		require.NoError(t, err)
		require.Equal(t, data, result.AttestationData)
		require.Len(t, result.Signatures, 2)
	})

	t.Run("queryErrorExcludedNotFatal", func(t *testing.T) {
		erroring := attestor.NewMockAttestor(t)
		erroring.EXPECT().Name().Return("erroring").Maybe()
		erroring.EXPECT().StateAttestation(mock.Anything, mock.Anything).Return(attestor.Attestation{}, assert.AnError)

		attestors := []attestor.Attestor{
			signedAttestor(t, "a1", data),
			signedAttestor(t, "a2", data),
			erroring,
		}

		result, err := newTestGenerator(t, attestors, 2, nil).queryStateQuorum(ctx, 10, data)
		require.NoError(t, err)
		require.Len(t, result.Signatures, 2)
	})

	t.Run("badSignatureExcludedNotFatal", func(t *testing.T) {
		badAttestor := attestor.NewMockAttestor(t)
		badAttestor.EXPECT().Name().Return("bad").Maybe()
		badAttestor.EXPECT().StateAttestation(mock.Anything, mock.Anything).Return(
			attestor.Attestation{Height: 10, AttestedData: data, Signature: []byte("not a valid signature")}, nil,
		)

		attestors := []attestor.Attestor{
			signedAttestor(t, "a1", data),
			signedAttestor(t, "a2", data),
			badAttestor,
		}

		// threshold 2 still met by the two valid attestors despite the bad one
		result, err := newTestGenerator(t, attestors, 2, nil).queryStateQuorum(ctx, 10, data)
		require.NoError(t, err)
		require.Len(t, result.Signatures, 2)
	})

	// The light client rejects v of 0/1 and high s, so a proof must carry
	// neither even when attestors return them.
	t.Run("proofCarriesNormalizedSignatures", func(t *testing.T) {
		digest := attestorevm.Digest(attestorevm.TagStateAttestation, data)
		highS, err := crypto.Sign(digest[:], testKey(t, "high-s"))
		require.NoError(t, err)
		n := crypto.S256().Params().N
		new(big.Int).Sub(n, new(big.Int).SetBytes(highS[32:64])).FillBytes(highS[32:64])
		highS[64] ^= 1

		highSAttestor := attestor.NewMockAttestor(t)
		highSAttestor.EXPECT().Name().Return("high-s").Maybe()
		highSAttestor.EXPECT().StateAttestation(mock.Anything, mock.Anything).Return(
			attestor.Attestation{Height: 10, AttestedData: data, Signature: highS}, nil,
		)

		attestors := []attestor.Attestor{signedAttestor(t, "raw-v", data), highSAttestor}

		result, err := newTestGenerator(t, attestors, 2, nil).queryStateQuorum(ctx, 10, data)
		require.NoError(t, err)
		require.Len(t, result.Signatures, 2)
		for _, sig := range result.Signatures {
			require.Contains(t, []byte{27, 28}, sig[64])
			r, s := new(big.Int).SetBytes(sig[:32]), new(big.Int).SetBytes(sig[32:64])
			require.True(t, crypto.ValidateSignatureValues(sig[64]-27, r, s, true), "s above n/2")
		}
	})

	t.Run("duplicateSignerCountsOnce", func(t *testing.T) {
		digest := attestorevm.Digest(attestorevm.TagStateAttestation, data)
		sig, err := crypto.Sign(digest[:], testKey(t, "a1"))
		require.NoError(t, err)

		makeAttestor := func(name string) *attestor.MockAttestor {
			a := attestor.NewMockAttestor(t)
			a.EXPECT().Name().Return(name).Maybe()
			a.EXPECT().StateAttestation(mock.Anything, mock.Anything).Return(
				attestor.Attestation{Height: 10, AttestedData: data, Signature: sig}, nil,
			)

			return a
		}

		attestors := []attestor.Attestor{
			makeAttestor("a1"),
			makeAttestor("a2-same-key"),
		}

		_, err = newTestGenerator(t, attestors, 2, nil).queryStateQuorum(ctx, 10, data)
		require.ErrorContains(
			t,
			err,
			"quorum not met",
			"the same recovered signer answering twice must not count twice",
		)
	})

	// The light client reverts the whole proof on a signer outside its set, so
	// one is excluded and quorum holds only if members alone meet the threshold.
	t.Run("signerOutsideSetExcluded", func(t *testing.T) {
		set := &attestationSet{signers: signerSet(t, "a1", "a2"), threshold: 2}
		outsider := crypto.PubkeyToAddress(testKey(t, "outsider").PublicKey)

		for _, tt := range []struct {
			name    string
			members []string
			wantErr bool
		}{
			{name: "quorumMet", members: []string{"a1", "a2"}},
			{name: "quorumNotMet", members: []string{"a1"}, wantErr: true},
		} {
			t.Run(tt.name, func(t *testing.T) {
				attestors := []attestor.Attestor{signedAttestor(t, "outsider", data)}
				for _, name := range tt.members {
					attestors = append(attestors, signedAttestor(t, name, data))
				}

				result, err := newGenerator("client-chain", "client-0", attestors, set, nil, slog.Default()).
					queryStateQuorum(ctx, 10, data)
				if !tt.wantErr {
					require.NoError(t, err)
					require.Len(t, result.Signatures, 2)

					return
				}
				require.ErrorContains(t, err, "quorum not met: got 1 of 2")
				require.ErrorContains(
					t,
					err,
					`attestor "outsider": signer `+outsider.Hex()+" is not in the attestation set",
				)
			})
		}
	})
}

// heightAttestor builds a attestor.MockAttestor that only answers LatestHeight,
// for exercising latestProvableHeight in isolation from the
// attestation-signing quorum logic above.
func heightAttestor(t *testing.T, name string, height uint64) *attestor.MockAttestor {
	t.Helper()

	a := attestor.NewMockAttestor(t)
	a.EXPECT().Name().Return(name).Maybe()
	a.EXPECT().LatestHeight(mock.Anything).Return(height, nil)

	return a
}

func erroringHeightAttestor(t *testing.T, name string) *attestor.MockAttestor {
	t.Helper()

	a := attestor.NewMockAttestor(t)
	a.EXPECT().Name().Return(name).Maybe()
	a.EXPECT().LatestHeight(mock.Anything).Return(0, assert.AnError)

	return a
}

var someBlockTime = time.Unix(1700000000, 0).UTC()

func TestLatestProvableHeight(t *testing.T) {
	ctx := context.Background()

	t.Run("takesHighestHeightAtLeastThresholdAttestorsReached", func(t *testing.T) {
		attestors := []attestor.Attestor{
			heightAttestor(t, "a1", 100),
			heightAttestor(t, "a2", 80),
			heightAttestor(t, "a3", 120),
		}

		counterpartyChain := mocks.NewMockClient(t)
		counterpartyChain.EXPECT().
			GetBlockHeader(mock.Anything, uint64(100)).
			Return(v2.BlockHeader{Timestamp: someBlockTime}, nil)

		height, timestamp, err := latestProvableHeight(ctx, slog.Default(), attestors, 2, counterpartyChain)
		require.NoError(t, err)
		require.Equal(
			t,
			uint64(100),
			height,
			"a single lagging attestor (a2) must not drag the resolved height below what threshold=2 others (a1, a3) already agree on",
		)
		require.Equal(t, someBlockTime, timestamp)
	})

	t.Run("laggingAttestorDoesNotBreakLiveness", func(t *testing.T) {
		attestors := []attestor.Attestor{
			heightAttestor(t, "a1", 100),
			heightAttestor(t, "a2", 95),
			erroringHeightAttestor(t, "down"),
		}

		counterpartyChain := mocks.NewMockClient(t)
		counterpartyChain.EXPECT().
			GetBlockHeader(mock.Anything, uint64(95)).
			Return(v2.BlockHeader{Timestamp: someBlockTime}, nil)

		height, _, err := latestProvableHeight(ctx, slog.Default(), attestors, 2, counterpartyChain)
		require.NoError(t, err)
		require.Equal(t, uint64(95), height)
	})

	t.Run("quorumNotMet", func(t *testing.T) {
		attestors := []attestor.Attestor{
			heightAttestor(t, "a1", 100),
			erroringHeightAttestor(t, "down1"),
			erroringHeightAttestor(t, "down2"),
		}

		// no GetBlockHeader expectation: quorum failure must short-circuit
		// before ever consulting the chain
		counterpartyChain := mocks.NewMockClient(t)

		_, _, err := latestProvableHeight(ctx, slog.Default(), attestors, 2, counterpartyChain)
		require.ErrorContains(t, err, "quorum not met")
	})

	t.Run("timestampLookupError", func(t *testing.T) {
		attestors := []attestor.Attestor{
			heightAttestor(t, "a1", 100),
			heightAttestor(t, "a2", 100),
		}

		counterpartyChain := mocks.NewMockClient(t)
		counterpartyChain.EXPECT().GetBlockHeader(mock.Anything, uint64(100)).Return(v2.BlockHeader{}, assert.AnError)

		_, _, err := latestProvableHeight(ctx, slog.Default(), attestors, 2, counterpartyChain)
		require.ErrorContains(t, err, "getting header")
	})
}

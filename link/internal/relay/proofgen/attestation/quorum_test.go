package attestation

import (
	"context"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/crypto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/cosmos/ibc/link/internal/service/attestor"
	attestorevm "github.com/cosmos/ibc/link/internal/service/attestor/evm"
	"github.com/cosmos/ibc/link/internal/tests/mocks"
	v2 "github.com/cosmos/ibc/link/internal/types/v2"
)

// signedAttestor builds a attestor.MockAttestor whose StateAttestation call
// returns attestedData signed by a freshly generated key over the correct
// domain-tagged digest.
func signedAttestor(t *testing.T, name string, attestedData []byte) *attestor.MockAttestor {
	t.Helper()

	key, err := crypto.GenerateKey()
	require.NoError(t, err)

	digest := attestorevm.Digest(attestorevm.TagStateAttestation, attestedData)
	sig, err := crypto.Sign(digest[:], key)
	require.NoError(t, err)

	a := attestor.NewMockAttestor(t)
	a.EXPECT().Name().Return(name).Maybe()
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

		result, err := queryStateQuorum(ctx, attestors, 2, 10)
		require.NoError(t, err)
		require.Equal(t, data, result.AttestationData)
		require.Len(t, result.Signatures, 2)
	})

	t.Run("quorumNotMet", func(t *testing.T) {
		attestors := []attestor.Attestor{
			signedAttestor(t, "a1", data),
		}

		_, err := queryStateQuorum(ctx, attestors, 2, 10)
		require.ErrorContains(t, err, "quorum not met")
	})

	t.Run("mismatchedDataExcluded", func(t *testing.T) {
		attestors := []attestor.Attestor{
			signedAttestor(t, "a1", data),
			signedAttestor(t, "a2", []byte("a different claim entirely")),
		}

		_, err := queryStateQuorum(ctx, attestors, 2, 10)
		require.ErrorContains(
			t,
			err,
			"quorum not met",
			"the second attestor's disagreeing claim must not count toward quorum",
		)
	})

	t.Run("majorityQuorumMetDespiteFirstAttestorDisagreeing", func(t *testing.T) {
		// a1 answers first (by configured order) with a stale/wrong claim; a2
		// and a3 agree with each other and alone meet the threshold. Quorum
		// must be reduced by grouping on value, not by anchoring to whichever
		// response happens to come first.
		attestors := []attestor.Attestor{
			signedAttestor(t, "a1", []byte("stale claim")),
			signedAttestor(t, "a2", data),
			signedAttestor(t, "a3", data),
		}

		result, err := queryStateQuorum(ctx, attestors, 2, 10)
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

		result, err := queryStateQuorum(ctx, attestors, 2, 10)
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
		result, err := queryStateQuorum(ctx, attestors, 2, 10)
		require.NoError(t, err)
		require.Len(t, result.Signatures, 2)
	})

	t.Run("duplicateSignerCountsOnce", func(t *testing.T) {
		key, err := crypto.GenerateKey()
		require.NoError(t, err)

		digest := attestorevm.Digest(attestorevm.TagStateAttestation, data)
		sig, err := crypto.Sign(digest[:], key)
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

		_, err = queryStateQuorum(ctx, attestors, 2, 10)
		require.ErrorContains(
			t,
			err,
			"quorum not met",
			"the same recovered signer answering twice must not count twice",
		)
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

		height, timestamp, err := latestProvableHeight(ctx, attestors, 2, counterpartyChain)
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

		height, _, err := latestProvableHeight(ctx, attestors, 2, counterpartyChain)
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

		_, _, err := latestProvableHeight(ctx, attestors, 2, counterpartyChain)
		require.ErrorContains(t, err, "quorum not met")
	})

	t.Run("timestampLookupError", func(t *testing.T) {
		attestors := []attestor.Attestor{
			heightAttestor(t, "a1", 100),
			heightAttestor(t, "a2", 100),
		}

		counterpartyChain := mocks.NewMockClient(t)
		counterpartyChain.EXPECT().GetBlockHeader(mock.Anything, uint64(100)).Return(v2.BlockHeader{}, assert.AnError)

		_, _, err := latestProvableHeight(ctx, attestors, 2, counterpartyChain)
		require.ErrorContains(t, err, "getting header")
	})
}

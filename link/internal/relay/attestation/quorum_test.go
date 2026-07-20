package attestation

import (
	"context"
	"testing"

	"connectrpc.com/connect"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	ibcattestor "github.com/cosmos/ibc/link/internal/types/ibcattestor"
)

// signedAttestor builds a namedAttestorClient whose StateAttestation call
// returns attestedData signed by a freshly generated key over the correct
// domain-tagged digest.
func signedAttestor(t *testing.T, name string, attestedData []byte) namedAttestorClient {
	t.Helper()

	key, err := crypto.GenerateKey()
	require.NoError(t, err)

	digest := taggedDigest(attestedData, attestationTypeState)
	sig, err := crypto.Sign(digest[:], key)
	require.NoError(t, err)

	client := ibcattestor.NewMockAttestationServiceClient(t)
	client.EXPECT().StateAttestation(mock.Anything, mock.Anything).Return(
		connect.NewResponse(&ibcattestor.StateAttestationResponse{
			Attestation: &ibcattestor.Attestation{
				Height:       10,
				AttestedData: attestedData,
				Signature:    sig,
			},
		}), nil,
	)

	return namedAttestorClient{Name: name, Client: client}
}

func TestQueryQuorum(t *testing.T) {
	ctx := context.Background()
	data := []byte("attested state data")

	t.Run("quorumMet", func(t *testing.T) {
		attestors := []namedAttestorClient{
			signedAttestor(t, "a1", data),
			signedAttestor(t, "a2", data),
		}

		result, err := queryStateQuorum(ctx, attestors, 2, 10)
		require.NoError(t, err)
		require.Equal(t, data, result.AttestationData)
		require.Len(t, result.Signatures, 2)
	})

	t.Run("quorumNotMet", func(t *testing.T) {
		attestors := []namedAttestorClient{
			signedAttestor(t, "a1", data),
		}

		_, err := queryStateQuorum(ctx, attestors, 2, 10)
		require.ErrorContains(t, err, "quorum not met")
	})

	t.Run("mismatchedDataExcluded", func(t *testing.T) {
		attestors := []namedAttestorClient{
			signedAttestor(t, "a1", data),
			signedAttestor(t, "a2", []byte("a different claim entirely")),
		}

		_, err := queryStateQuorum(ctx, attestors, 2, 10)
		require.ErrorContains(t, err, "quorum not met", "the second attestor's disagreeing claim must not count toward quorum")
	})

	t.Run("queryErrorExcludedNotFatal", func(t *testing.T) {
		erroring := ibcattestor.NewMockAttestationServiceClient(t)
		erroring.EXPECT().StateAttestation(mock.Anything, mock.Anything).Return(nil, assert.AnError)

		attestors := []namedAttestorClient{
			signedAttestor(t, "a1", data),
			signedAttestor(t, "a2", data),
			{Name: "erroring", Client: erroring},
		}

		result, err := queryStateQuorum(ctx, attestors, 2, 10)
		require.NoError(t, err)
		require.Len(t, result.Signatures, 2)
	})

	t.Run("badSignatureExcludedNotFatal", func(t *testing.T) {
		badClient := ibcattestor.NewMockAttestationServiceClient(t)
		badClient.EXPECT().StateAttestation(mock.Anything, mock.Anything).Return(
			connect.NewResponse(&ibcattestor.StateAttestationResponse{
				Attestation: &ibcattestor.Attestation{
					Height:       10,
					AttestedData: data,
					Signature:    []byte("not a valid signature"),
				},
			}), nil,
		)

		attestors := []namedAttestorClient{
			signedAttestor(t, "a1", data),
			signedAttestor(t, "a2", data),
			{Name: "bad", Client: badClient},
		}

		// threshold 2 still met by the two valid attestors despite the bad one
		result, err := queryStateQuorum(ctx, attestors, 2, 10)
		require.NoError(t, err)
		require.Len(t, result.Signatures, 2)
	})

	t.Run("duplicateSignerCountsOnce", func(t *testing.T) {
		key, err := crypto.GenerateKey()
		require.NoError(t, err)

		digest := taggedDigest(data, attestationTypeState)
		sig, err := crypto.Sign(digest[:], key)
		require.NoError(t, err)

		makeClient := func() ibcattestor.AttestationServiceClient {
			client := ibcattestor.NewMockAttestationServiceClient(t)
			client.EXPECT().StateAttestation(mock.Anything, mock.Anything).Return(
				connect.NewResponse(&ibcattestor.StateAttestationResponse{
					Attestation: &ibcattestor.Attestation{Height: 10, AttestedData: data, Signature: sig},
				}), nil,
			)

			return client
		}

		attestors := []namedAttestorClient{
			{Name: "a1", Client: makeClient()},
			{Name: "a2-same-key", Client: makeClient()},
		}

		_, err = queryStateQuorum(ctx, attestors, 2, 10)
		require.ErrorContains(t, err, "quorum not met", "the same recovered signer answering twice must not count twice")
	})
}

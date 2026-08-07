package attestor

import (
	"context"
	"testing"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/cosmos/ibc/link/internal/config"
	"github.com/cosmos/ibc/link/internal/service/signer"
	v2 "github.com/cosmos/ibc/link/internal/types/v2"

	proto "github.com/cosmos/ibc/link/api/v2/attestor"
)

func TestService(t *testing.T) {
	sampleSigner, err := signer.GenerateLocalSecp256k1Signer()
	require.NoError(t, err)

	t.Run("duplicateAliases", func(t *testing.T) {
		// ARRANGE
		attestors := []Attestor{
			must(NewLocal(config.AttestorConfig{Alias: "alice", ChainID: "1", Name: "alice"}, stubChainClient(t, "1"), sampleSigner)),
			must(NewLocal(config.AttestorConfig{Alias: "alice", ChainID: "2", Name: "alice"}, stubChainClient(t, "2"), sampleSigner)),
		}

		// ACT
		service, err := New(attestors)

		// ASSERT
		require.ErrorContains(t, err, "attestor with alias alice already exists")
		assert.Nil(t, service)
	})

	t.Run("local", func(t *testing.T) {
		// ARRANGE
		ctx := context.Background()
		service, err := New([]Attestor{
			stubLocalAttestor(t, "1", "alice", sampleSigner),
			stubLocalAttestor(t, "2", "bob", sampleSigner),
			stubLocalAttestor(t, "3", "carol", sampleSigner),
		})
		require.NoError(t, err)

		for _, alias := range []string{"alice", "bob", "carol"} {
			t.Run(alias, func(t *testing.T) {
				// ACT
				height, err := service.LatestHeight(ctx, alias)

				// ASSERT
				require.NoError(t, err)
				assert.Equal(t, uint64(1), height)
			})
		}

		t.Run("not found", func(t *testing.T) {
			// ACT
			height, err := service.LatestHeight(ctx, "zoe")

			// ASSERT
			require.ErrorIs(t, err, ErrNotFound)
			assert.Zero(t, height)
		})
	})

	t.Run("remote", func(t *testing.T) {
		// ARRANGE
		ctx := context.Background()
		client := attestationClient{heights: map[string]uint64{
			"alice": 10,
			"bob":   20,
			"carol": 30,
		}}
		service, err := New([]Attestor{
			NewRemote("ethereum", "alice", "eth-alice", client),
			NewRemote("cosmos", "bob", "cosmos-bob", client),
			NewRemote("solana", "carol", "solana-carol", client),
		})
		require.NoError(t, err)

		for _, tt := range []struct {
			name           string
			alias          string
			expectedHeight uint64
		}{
			{
				name:           "alice",
				alias:          "eth-alice",
				expectedHeight: 10,
			},
			{
				name:           "bob",
				alias:          "cosmos-bob",
				expectedHeight: 20,
			},
			{
				name:           "carol",
				alias:          "solana-carol",
				expectedHeight: 30,
			},
		} {
			t.Run(tt.name, func(t *testing.T) {
				// ACT
				height, err := service.LatestHeight(ctx, tt.alias)

				// ASSERT
				require.NoError(t, err)
				assert.Equal(t, tt.expectedHeight, height)
			})
		}

		t.Run("not found", func(t *testing.T) {
			// ACT
			height, err := service.LatestHeight(ctx, "zoe")

			// ASSERT
			require.ErrorIs(t, err, ErrNotFound)
			assert.Zero(t, height)
		})
	})

	t.Run("mix", func(t *testing.T) {
		// ARRANGE
		ctx := context.Background()
		client := attestationClient{heights: map[string]uint64{"bob": 20}}
		service, err := New([]Attestor{
			NewRemote("ethereum", "alice", "eth-alice", client),
			NewRemote("cosmos", "bob", "cosmos-bob", client),
			NewRemote("solana", "carol", "solana-carol", client),
			stubLocalAttestor(t, "ethereum", "dave", sampleSigner),
		})
		require.NoError(t, err)

		// ACT
		remoteHeight, err := service.LatestHeight(ctx, "cosmos-bob")

		// ASSERT
		require.NoError(t, err)
		assert.Equal(t, uint64(20), remoteHeight)

		// ACT
		localHeight, err := service.LatestHeight(ctx, "dave")

		// ASSERT
		require.NoError(t, err)
		assert.Equal(t, uint64(1), localHeight)
	})
}

type attestationClient struct {
	heights map[string]uint64
}

var _ proto.AttestationServiceClient = attestationClient{}

func (c attestationClient) LatestHeight(
	_ context.Context,
	req *connect.Request[proto.LatestHeightRequest],
) (*connect.Response[proto.LatestHeightResponse], error) {
	return connect.NewResponse(&proto.LatestHeightResponse{Height: c.heights[req.Msg.Attestor]}), nil
}

func (c attestationClient) StateAttestation(
	_ context.Context,
	_ *connect.Request[proto.StateAttestationRequest],
) (*connect.Response[proto.StateAttestationResponse], error) {
	return nil, connect.NewError(connect.CodeUnimplemented, nil)
}

func (c attestationClient) PacketAttestation(
	_ context.Context,
	_ *connect.Request[proto.PacketAttestationRequest],
) (*connect.Response[proto.PacketAttestationResponse], error) {
	return nil, connect.NewError(connect.CodeUnimplemented, nil)
}

func stubLocalAttestor(t *testing.T, chainID, name string, backingSigner signer.Signer) *LocalAttestor {
	t.Helper()

	client := stubChainClient(t, chainID)
	client.EXPECT().
		GetBlockHeader(mock.Anything, uint64(v2.FinalizedBlock)).
		Return(v2.BlockHeader{Height: 1}, nil).
		Once()

	attestor, err := NewLocal(
		config.AttestorConfig{Alias: name, ChainID: chainID, Name: name},
		client,
		backingSigner,
	)
	require.NoError(t, err)

	return attestor
}

func must[T any](value T, err error) T {
	if err != nil {
		panic(err)
	}
	return value
}

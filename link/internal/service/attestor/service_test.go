package attestor

import (
	"context"
	"testing"

	"connectrpc.com/connect"
	"github.com/cosmos/ibc/link/internal/config"
	"github.com/cosmos/ibc/link/internal/service/signer"
	proto "github.com/cosmos/ibc/link/internal/types/v2/attestor"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestService(t *testing.T) {
	sampleSigner, err := signer.GenerateLocalSecp256k1Signer()
	require.NoError(t, err)

	t.Run("duplicateAliases", func(t *testing.T) {
		// ARRANGE
		attestors := []Attestor{
			must(NewLocal(config.AttestationConfig{ChainID: "1", Name: "alice"}, stubEvmClient(t, "1"), sampleSigner)),
			must(NewLocal(config.AttestationConfig{ChainID: "2", Name: "alice"}, stubEvmClient(t, "2"), sampleSigner)),
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
			must(NewLocal(config.AttestationConfig{ChainID: "1", Name: "alice"}, stubEvmClient(t, "1"), sampleSigner)),
			must(NewLocal(config.AttestationConfig{ChainID: "2", Name: "bob"}, stubEvmClient(t, "2"), sampleSigner)),
			must(NewLocal(config.AttestationConfig{ChainID: "3", Name: "carol"}, stubEvmClient(t, "3"), sampleSigner)),
		})
		require.NoError(t, err)

		for _, alias := range []string{"alice", "bob", "carol"} {
			t.Run(alias, func(t *testing.T) {
				// ACT
				height, err := service.LatestAttestableHeight(ctx, alias)

				// ASSERT
				require.NoError(t, err)
				assert.Equal(t, uint64(1), height)
			})
		}

		t.Run("not found", func(t *testing.T) {
			// ACT
			height, err := service.LatestAttestableHeight(ctx, "zoe")

			// ASSERT
			require.ErrorIs(t, err, ErrNotFound)
			assert.Zero(t, height)
		})
	})

	t.Run("remote", func(t *testing.T) {
		// ARRANGE
		ctx := context.Background()
		client := proto.NewMockAttestationServiceClient(t)
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
				// ARRANGE
				client.EXPECT().
					LatestAttestableHeight(mock.Anything, latestAttestableHeightRequest(tt.name)).
					Return(connect.NewResponse(&proto.LatestAttestableHeightResponse{
						Height: tt.expectedHeight,
					}), nil).
					Once()

				// ACT
				height, err := service.LatestAttestableHeight(ctx, tt.alias)

				// ASSERT
				require.NoError(t, err)
				assert.Equal(t, tt.expectedHeight, height)
			})
		}

		t.Run("not found", func(t *testing.T) {
			// ACT
			height, err := service.LatestAttestableHeight(ctx, "zoe")

			// ASSERT
			require.ErrorIs(t, err, ErrNotFound)
			assert.Zero(t, height)
		})
	})

	t.Run("mix", func(t *testing.T) {
		// ARRANGE
		ctx := context.Background()
		client := proto.NewMockAttestationServiceClient(t)
		service, err := New([]Attestor{
			NewRemote("ethereum", "alice", "eth-alice", client),
			NewRemote("cosmos", "bob", "cosmos-bob", client),
			NewRemote("solana", "carol", "solana-carol", client),
			must(NewLocal(config.AttestationConfig{ChainID: "ethereum", Name: "dave"}, stubEvmClient(t, "ethereum"), sampleSigner)),
		})
		require.NoError(t, err)
		client.EXPECT().
			LatestAttestableHeight(mock.Anything, latestAttestableHeightRequest("bob")).
			Return(connect.NewResponse(&proto.LatestAttestableHeightResponse{
				Height: 20,
			}), nil).
			Once()

		// ACT
		remoteHeight, err := service.LatestAttestableHeight(ctx, "cosmos-bob")

		// ASSERT
		require.NoError(t, err)
		assert.Equal(t, uint64(20), remoteHeight)

		// ACT
		localHeight, err := service.LatestAttestableHeight(ctx, "dave")

		// ASSERT
		require.NoError(t, err)
		assert.Equal(t, uint64(1), localHeight)
	})

}

func latestAttestableHeightRequest(attestor string) any {
	matcher := func(req *connect.Request[proto.LatestAttestableHeightRequest]) bool {
		return req.Msg.Attestor == attestor
	}

	return mock.MatchedBy(matcher)
}

func must[T any](value T, err error) T {
	if err != nil {
		panic(err)
	}
	return value
}

package attestor

import (
	"context"
	"testing"
	"time"

	"connectrpc.com/connect"
	proto "github.com/cosmos/ibc/link/internal/types/v2/attestor"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestService(t *testing.T) {
	t.Run("duplicateAliases", func(t *testing.T) {
		// ARRANGE
		attestors := []Attestor{
			NewLocal("1", "alice"),
			NewLocal("2", "alice"),
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
			NewLocal("1", "alice"),
			NewLocal("2", "bob"),
			NewLocal("3", "carol"),
		})
		require.NoError(t, err)

		start := uint64(time.Now().Unix())

		for _, alias := range []string{"alice", "bob", "carol"} {
			t.Run(alias, func(t *testing.T) {
				// ACT
				height, err := service.LatestAttestableHeight(ctx, alias)

				// ASSERT
				require.NoError(t, err)
				assert.GreaterOrEqual(t, height, start)
				assert.LessOrEqual(t, height, uint64(time.Now().Unix()))
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
			NewLocal("ethereum", "dave"),
		})
		require.NoError(t, err)
		start := uint64(time.Now().Unix())
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
		assert.GreaterOrEqual(t, localHeight, start)
		assert.LessOrEqual(t, localHeight, uint64(time.Now().Unix()))
	})

}

func latestAttestableHeightRequest(attestor string) any {
	matcher := func(req *connect.Request[proto.LatestAttestableHeightRequest]) bool {
		return req.Msg.Attestor == attestor
	}

	return mock.MatchedBy(matcher)
}

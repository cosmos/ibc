package attestor

import (
	"context"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cosmos/ibc/link/internal/service/signer"

	proto "github.com/cosmos/ibc/api/v2/attestor"
)

func TestService(t *testing.T) {
	sampleSigner, err := signer.GenerateLocalSecp256k1Signer()
	require.NoError(t, err)

	t.Run("duplicateAliases", func(t *testing.T) {
		// ARRANGE
		attestors := []Attestor{
			must(NewLocal("1", "alice", sampleSigner)),
			must(NewLocal("2", "alice", sampleSigner)),
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
			must(NewLocal("1", "alice", sampleSigner)),
			must(NewLocal("2", "bob", sampleSigner)),
			must(NewLocal("3", "carol", sampleSigner)),
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
		client := attestationClient{heights: map[string]uint64{"bob": 20}}
		service, err := New([]Attestor{
			NewRemote("ethereum", "alice", "eth-alice", client),
			NewRemote("cosmos", "bob", "cosmos-bob", client),
			NewRemote("solana", "carol", "solana-carol", client),
			must(NewLocal("ethereum", "dave", sampleSigner)),
		})
		require.NoError(t, err)
		start := uint64(time.Now().Unix())
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

type attestationClient struct {
	heights map[string]uint64
}

func (c attestationClient) LatestAttestableHeight(
	_ context.Context,
	req *connect.Request[proto.LatestAttestableHeightRequest],
) (*connect.Response[proto.LatestAttestableHeightResponse], error) {
	return connect.NewResponse(&proto.LatestAttestableHeightResponse{Height: c.heights[req.Msg.Attestor]}), nil
}

func must[T any](value T, err error) T {
	if err != nil {
		panic(err)
	}
	return value
}

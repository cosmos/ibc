// SPDX-License-Identifier: Apache-2.0

package remote

import (
	"context"
	"log/slog"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/require"

	channeltypesv2 "github.com/cosmos/ibc-go/v11/modules/core/04-channel/v2/types"
	proverv2 "github.com/cosmos/ibc/cli/api/v2/prover"
	v2 "github.com/cosmos/ibc/cli/internal/types/v2"
)

func TestProverRequestTimeout(t *testing.T) {
	client := timeoutProverClient{t: t}
	prover := &Prover{client: client, chainID: "chain-a", clientID: "client-0", logger: slog.Default()}
	ctx := context.Background()

	_, _, err := prover.LatestProvableHeight(ctx)
	require.NoError(t, err)

	_, err = prover.Prepare(ctx, 1, v2.ProofKindPacketCommitment, []channeltypesv2.Packet{{Sequence: 1}})
	require.NoError(t, err)
}

type timeoutProverClient struct {
	t *testing.T
}

func (c timeoutProverClient) LatestProvableHeight(
	ctx context.Context,
	_ *connect.Request[proverv2.LatestProvableHeightRequest],
) (*connect.Response[proverv2.LatestProvableHeightResponse], error) {
	c.requireDeadline(ctx)
	return connect.NewResponse(&proverv2.LatestProvableHeightResponse{}), nil
}

func (c timeoutProverClient) Prepare(
	ctx context.Context,
	_ *connect.Request[proverv2.PrepareRequest],
) (*connect.Response[proverv2.PrepareResponse], error) {
	c.requireDeadline(ctx)
	return connect.NewResponse(
		&proverv2.PrepareResponse{
			Result: &proverv2.PrepareResponse_Ready{Ready: &proverv2.BatchProofs{PacketProofs: [][]byte{{1}}}},
		},
	), nil
}

func (c timeoutProverClient) requireDeadline(ctx context.Context) {
	c.t.Helper()

	deadline, ok := ctx.Deadline()
	require.True(c.t, ok)
	require.WithinDuration(c.t, time.Now().Add(requestTimeout), deadline, time.Second)
}

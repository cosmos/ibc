// SPDX-License-Identifier: Apache-2.0

package remote

import (
	"context"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/require"

	channeltypesv2 "github.com/cosmos/ibc-go/v11/modules/core/04-channel/v2/types"
	proverv2 "github.com/cosmos/ibc/link/api/v2/prover"
	v2 "github.com/cosmos/ibc/link/internal/types/v2"
)

func TestProverRequestTimeout(t *testing.T) {
	client := timeoutProverClient{t: t}
	prover := &Prover{client: client, chainID: "chain-a", clientID: "client-0"}
	ctx := context.Background()

	_, _, err := prover.LatestProvableHeight(ctx)
	require.NoError(t, err)

	_, err = prover.StateProof(ctx, 1)
	require.NoError(t, err)

	_, err = prover.PacketProofs(ctx, 1, v2.ProofKindPacketCommitment, []channeltypesv2.Packet{{Sequence: 1}})
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

func (c timeoutProverClient) StateProof(
	ctx context.Context,
	_ *connect.Request[proverv2.StateProofRequest],
) (*connect.Response[proverv2.StateProofResponse], error) {
	c.requireDeadline(ctx)
	return connect.NewResponse(&proverv2.StateProofResponse{}), nil
}

func (c timeoutProverClient) PacketProofs(
	ctx context.Context,
	_ *connect.Request[proverv2.PacketProofsRequest],
) (*connect.Response[proverv2.PacketProofsResponse], error) {
	c.requireDeadline(ctx)
	return connect.NewResponse(&proverv2.PacketProofsResponse{Proofs: [][]byte{{0x1}}}), nil
}

func (c timeoutProverClient) requireDeadline(ctx context.Context) {
	c.t.Helper()

	deadline, ok := ctx.Deadline()
	require.True(c.t, ok)
	require.WithinDuration(c.t, time.Now().Add(requestTimeout), deadline, time.Second)
}

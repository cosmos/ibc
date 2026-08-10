// SPDX-License-Identifier: Apache-2.0

package attestor

import (
	"context"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/require"

	proto "github.com/cosmos/ibc/link/api/v2/attestor"
	attestordomain "github.com/cosmos/ibc/link/attestor"
)

func TestRemoteAttestorRequestTimeout(t *testing.T) {
	client := timeoutAttestationClient{t: t}
	remote := NewRemote("chain", "name", "alias", client)

	_, err := remote.LatestHeight(context.Background())
	require.NoError(t, err)
	_, err = remote.StateAttestation(context.Background(), 1)
	require.NoError(t, err)
	_, err = remote.PacketAttestation(context.Background(), attestordomain.PacketAttestationRequest{
		CommitmentType: attestordomain.CommitmentTypePacket,
	})
	require.NoError(t, err)
}

type timeoutAttestationClient struct {
	t *testing.T
}

func (c timeoutAttestationClient) LatestHeight(
	ctx context.Context,
	_ *connect.Request[proto.LatestHeightRequest],
) (*connect.Response[proto.LatestHeightResponse], error) {
	c.requireTimeout(ctx)
	return connect.NewResponse(&proto.LatestHeightResponse{}), nil
}

func (c timeoutAttestationClient) StateAttestation(
	ctx context.Context,
	_ *connect.Request[proto.StateAttestationRequest],
) (*connect.Response[proto.StateAttestationResponse], error) {
	c.requireTimeout(ctx)
	return connect.NewResponse(&proto.StateAttestationResponse{Attestation: &proto.Attestation{}}), nil
}

func (c timeoutAttestationClient) PacketAttestation(
	ctx context.Context,
	_ *connect.Request[proto.PacketAttestationRequest],
) (*connect.Response[proto.PacketAttestationResponse], error) {
	c.requireTimeout(ctx)
	return connect.NewResponse(&proto.PacketAttestationResponse{Attestation: &proto.Attestation{}}), nil
}

func (c timeoutAttestationClient) requireTimeout(ctx context.Context) {
	c.t.Helper()
	deadline, ok := ctx.Deadline()
	require.True(c.t, ok)
	require.WithinDuration(c.t, time.Now().Add(remoteRequestTimeout), deadline, time.Second)
}

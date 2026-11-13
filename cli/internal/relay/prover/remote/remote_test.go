// SPDX-License-Identifier: Apache-2.0

package remote

import (
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/require"

	channeltypesv2 "github.com/cosmos/ibc-go/v11/modules/core/04-channel/v2/types"
	proverv2 "github.com/lfdt-ibc/ibc/cli/api/v2/prover"
	"github.com/lfdt-ibc/ibc/cli/internal/config"
	"github.com/lfdt-ibc/ibc/cli/internal/network"
	v2 "github.com/lfdt-ibc/ibc/cli/internal/types/v2"
)

func TestProverRequestTimeout(t *testing.T) {
	for _, timeout := range []time.Duration{config.DefaultRemoteProverRequestTimeout, 5 * time.Minute} {
		for _, constructor := range []string{"New", "NewFromEndpoint"} {
			t.Run(constructor+"/"+timeout.String(), func(t *testing.T) {
				_, handler := proverv2.NewProverServiceHandler(timeoutProverClient{t: t, timeout: timeout})
				server := httptest.NewUnstartedServer(handler)
				server.EnableHTTP2 = true
				server.StartTLS()
				t.Cleanup(server.Close)

				var prover *Prover
				if constructor == "New" {
					prover = New(server.Client(), server.URL, "chain-a", "client-0", timeout, slog.Default())
				} else {
					endpoint := network.Endpoint{
						URL: server.URL,
						TLS: server.Client().Transport.(*http.Transport).TLSClientConfig.Clone(),
					}
					prover = NewFromEndpoint(endpoint, "chain-a", "client-0", timeout, slog.Default())
				}

				_, _, err := prover.LatestProvableHeight(t.Context())
				require.NoError(t, err)
				_, err = prover.ClientUpdatePayloads(t.Context(), 1)
				require.NoError(t, err)
				_, err = prover.PacketProofs(t.Context(), 1, v2.ProofKindPacketCommitment,
					[]channeltypesv2.Packet{{Sequence: 1}}, nil)
				require.NoError(t, err)
			})
		}
	}
}

func TestHTTPClientRequestTimeout(t *testing.T) {
	for _, timeout := range []time.Duration{250 * time.Millisecond, 5 * time.Minute} {
		client := newHTTPClient(nil, timeout)
		transport, ok := client.Transport.(*http.Transport)
		require.True(t, ok)
		require.Equal(t, timeout, transport.ResponseHeaderTimeout)
	}
}

func TestProverRequestCancellation(t *testing.T) {
	for _, method := range []string{"LatestProvableHeight", "ClientUpdatePayloads", "PacketProofs"} {
		for _, reason := range []string{"request timeout", "parent deadline", "parent cancellation"} {
			t.Run(method+"/"+reason, func(t *testing.T) {
				safetyCtx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
				defer cancel()
				ctx := safetyCtx
				const shortTimeout = 50 * time.Millisecond
				timeout := time.Minute
				wantCode := connect.CodeDeadlineExceeded
				switch reason {
				case "request timeout":
					timeout = shortTimeout
				case "parent deadline":
					var deadlineCancel context.CancelFunc
					ctx, deadlineCancel = context.WithTimeout(ctx, shortTimeout)
					defer deadlineCancel()
				case "parent cancellation":
					wantCode = connect.CodeCanceled
				}

				server := httptest.NewUnstartedServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
					if reason == "parent cancellation" {
						cancel()
					}
					<-r.Context().Done()
				}))
				server.EnableHTTP2 = true
				server.StartTLS()
				t.Cleanup(server.Close)
				endpoint := network.Endpoint{
					URL: server.URL,
					TLS: server.Client().Transport.(*http.Transport).TLSClientConfig.Clone(),
				}
				prover := NewFromEndpoint(endpoint, "chain-a", "client-0", timeout, slog.Default())
				var err error
				switch method {
				case "LatestProvableHeight":
					_, _, err = prover.LatestProvableHeight(ctx)
				case "ClientUpdatePayloads":
					_, err = prover.ClientUpdatePayloads(ctx, 1)
				case "PacketProofs":
					_, err = prover.PacketProofs(ctx, 1, v2.ProofKindPacketCommitment,
						[]channeltypesv2.Packet{{Sequence: 1}}, nil)
				}
				require.Error(t, err)
				require.Equal(t, wantCode, connect.CodeOf(err))
				require.NotErrorIs(t, safetyCtx.Err(), context.DeadlineExceeded,
					"outer safety deadline must not expire")
			})
		}
	}
}

type timeoutProverClient struct {
	t       *testing.T
	timeout time.Duration
}

func (c timeoutProverClient) LatestProvableHeight(
	ctx context.Context,
	_ *connect.Request[proverv2.LatestProvableHeightRequest],
) (*connect.Response[proverv2.LatestProvableHeightResponse], error) {
	c.requireDeadline(ctx)
	return connect.NewResponse(&proverv2.LatestProvableHeightResponse{}), nil
}

func (c timeoutProverClient) ClientUpdatePayloads(
	ctx context.Context,
	_ *connect.Request[proverv2.ClientUpdatePayloadsRequest],
) (*connect.Response[proverv2.ClientUpdatePayloadsResponse], error) {
	c.requireDeadline(ctx)
	return connect.NewResponse(&proverv2.ClientUpdatePayloadsResponse{}), nil
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
	require.WithinDuration(c.t, time.Now().Add(c.timeout), deadline, time.Second)
}

func TestPacketProofsRejectsAcknowledgementCountMismatch(t *testing.T) {
	for _, acks := range [][]channeltypesv2.Acknowledgement{nil, {{}, {}}} {
		prover := &Prover{}
		_, err := prover.PacketProofs(
			context.Background(),
			1,
			v2.ProofKindAcknowledgement,
			[]channeltypesv2.Packet{{Sequence: 1}},
			acks,
		)
		require.ErrorContains(t, err, "acknowledgement count must match packet count")
	}
}

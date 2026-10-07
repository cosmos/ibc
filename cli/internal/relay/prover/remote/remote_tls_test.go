// SPDX-License-Identifier: Apache-2.0

package remote

import (
	"context"
	"crypto/tls"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/require"

	proverv2 "github.com/cosmos/ibc/cli/api/v2/prover"
	"github.com/cosmos/ibc/cli/internal/config"
	"github.com/cosmos/ibc/cli/internal/network"
	"github.com/cosmos/ibc/cli/internal/testutil/certs"
)

// Probe exists because construction is inert: under TLS 1.3 the handshake
// succeeds and a rejected client certificate only surfaces on a request.
func TestProverProbe(t *testing.T) {
	ca := certs.NewCA(t)
	dir := t.TempDir()

	caFile := ca.WriteCA(t, dir)
	clientCert, clientKey := ca.WriteLeaf(t, dir, "client")

	addr := startTLSProver(t, ca.ServerTLS(t, "localhost", true))

	t.Run("succeeds against a reachable prover", func(t *testing.T) {
		prover := newProver(t, "https://"+addr, &config.TLSClientConfig{
			CAFile:     caFile,
			CertFile:   clientCert,
			KeyFile:    clientKey,
			ServerName: "localhost",
		})

		require.NoError(t, prover.Probe(context.Background()))
	})

	t.Run("reports a rejected client certificate", func(t *testing.T) {
		prover := newProver(t, "https://"+addr, &config.TLSClientConfig{
			CAFile:     caFile,
			ServerName: "localhost",
		})

		require.ErrorContains(t, prover.Probe(context.Background()), "probe")
	})

	t.Run("reports an unreachable prover", func(t *testing.T) {
		prover := NewFromEndpoint(network.Endpoint{URL: "https://127.0.0.1:1"}, "1", "client-0", slog.Default())

		require.Error(t, prover.Probe(context.Background()))
	})
}

// A plaintext prover keeps working, so an https-only change would be caught.
func TestProverPlaintextStillWorks(t *testing.T) {
	addr := startPlaintextProver(t)

	prover := NewFromEndpoint(network.Endpoint{URL: "http://" + addr}, "1", "client-0", slog.Default())

	require.NoError(t, prover.Probe(context.Background()))
}

func TestProverProbeServerErrors(t *testing.T) {
	for code, reachable := range map[connect.Code]bool{
		connect.CodeNotFound:           true,
		connect.CodeFailedPrecondition: true,
		connect.CodeInternal:           false,
		connect.CodeUnknown:            false,
		connect.CodeUnauthenticated:    false,
		connect.CodePermissionDenied:   false,
		connect.CodeUnimplemented:      false,
		connect.CodeUnavailable:        false,
	} {
		t.Run(code.String(), func(t *testing.T) {
			addr := startPlaintextProverWith(t, failingProverService{code: code})
			prover := NewFromEndpoint(network.Endpoint{URL: "http://" + addr}, "1", "client-0", slog.Default())

			err := prover.Probe(context.Background())
			if reachable {
				require.NoError(t, err)
			} else {
				require.ErrorContains(t, err, "probe")
			}
		})
	}
}

func newProver(t *testing.T, url string, cfg *config.TLSClientConfig) *Prover {
	t.Helper()

	endpoint, err := config.ResolveEndpoint(url, cfg, nil)
	require.NoError(t, err)

	return NewFromEndpoint(endpoint, "1", "client-0", slog.Default())
}

func startTLSProver(t *testing.T, tlsConfig *tls.Config) string {
	t.Helper()

	protocols := new(http.Protocols)
	protocols.SetHTTP1(true)
	protocols.SetHTTP2(true)

	srv := newProverServer(protocols)
	srv.TLSConfig = tlsConfig

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)

	go func() { _ = srv.ServeTLS(ln, "", "") }()
	t.Cleanup(func() { _ = srv.Close() })

	return ln.Addr().String()
}

func startPlaintextProver(t *testing.T) string {
	t.Helper()

	return startPlaintextProverWith(t, stubProverService{})
}

func startPlaintextProverWith(t *testing.T, svc proverv2.ProverServiceHandler) string {
	t.Helper()

	protocols := new(http.Protocols)
	protocols.SetHTTP1(true)
	protocols.SetUnencryptedHTTP2(true)

	srv := newProverServerWith(protocols, svc)

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)

	go func() { _ = srv.Serve(ln) }()
	t.Cleanup(func() { _ = srv.Close() })

	return ln.Addr().String()
}

func newProverServer(protocols *http.Protocols) *http.Server {
	return newProverServerWith(protocols, stubProverService{})
}

func newProverServerWith(protocols *http.Protocols, svc proverv2.ProverServiceHandler) *http.Server {
	path, handler := proverv2.NewProverServiceHandler(svc)
	mux := http.NewServeMux()
	mux.Handle(path, handler)

	return &http.Server{
		Handler:           mux,
		Protocols:         protocols,
		ReadHeaderTimeout: 3 * time.Second,
	}
}

type stubProverService struct{}

var _ proverv2.ProverServiceHandler = stubProverService{}

func (stubProverService) LatestProvableHeight(
	context.Context, *connect.Request[proverv2.LatestProvableHeightRequest],
) (*connect.Response[proverv2.LatestProvableHeightResponse], error) {
	return connect.NewResponse(&proverv2.LatestProvableHeightResponse{
		Height:    42,
		Timestamp: uint64(time.Now().Unix()),
	}), nil
}

func (stubProverService) ClientUpdatePayloads(
	context.Context, *connect.Request[proverv2.ClientUpdatePayloadsRequest],
) (*connect.Response[proverv2.ClientUpdatePayloadsResponse], error) {
	return connect.NewResponse(&proverv2.ClientUpdatePayloadsResponse{
		Payloads: [][]byte{[]byte("update-payload")},
	}), nil
}

func (stubProverService) PacketProofs(
	_ context.Context, req *connect.Request[proverv2.PacketProofsRequest],
) (*connect.Response[proverv2.PacketProofsResponse], error) {
	proofs := make([][]byte, len(req.Msg.GetPackets()))
	for i := range proofs {
		proofs[i] = []byte("packet-proof")
	}

	return connect.NewResponse(&proverv2.PacketProofsResponse{Proofs: proofs}), nil
}

// failingProverService answers LatestProvableHeight with code.
type failingProverService struct {
	stubProverService
	code connect.Code
}

func (s failingProverService) LatestProvableHeight(
	context.Context, *connect.Request[proverv2.LatestProvableHeightRequest],
) (*connect.Response[proverv2.LatestProvableHeightResponse], error) {
	return nil, connect.NewError(s.code, errors.New("stub"))
}

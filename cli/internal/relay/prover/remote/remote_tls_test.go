// SPDX-License-Identifier: Apache-2.0

package remote

import (
	"context"
	"crypto/tls"
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

// Under TLS 1.3 a rejected client certificate only surfaces on a request, so
// these make one rather than just constructing the prover.
func TestProverTLS(t *testing.T) {
	ca := certs.NewCA(t)
	dir := t.TempDir()

	caFile := ca.WriteCA(t, dir)
	clientCert, clientKey := ca.WriteLeaf(t, dir, "client")

	addr := startTLSProver(t, ca.ServerTLS(t, "localhost", true))

	t.Run("mTLS succeeds", func(t *testing.T) {
		prover := newProver(t, "https://"+addr, &config.TLSClientConfig{
			CAFile:     caFile,
			CertFile:   clientCert,
			KeyFile:    clientKey,
			ServerName: "localhost",
		})

		height, _, err := prover.LatestProvableHeight(context.Background())
		require.NoError(t, err)
		require.Equal(t, uint64(42), height)
	})

	t.Run("a missing client certificate is rejected", func(t *testing.T) {
		prover := newProver(t, "https://"+addr, &config.TLSClientConfig{
			CAFile:     caFile,
			ServerName: "localhost",
		})

		_, _, err := prover.LatestProvableHeight(context.Background())
		require.Error(t, err)
	})

	t.Run("an untrusted server is rejected", func(t *testing.T) {
		prover := newProver(t, "https://"+addr, &config.TLSClientConfig{
			CertFile:   clientCert,
			KeyFile:    clientKey,
			ServerName: "localhost",
		})

		_, _, err := prover.LatestProvableHeight(context.Background())
		require.Error(t, err)
	})
}

// A plaintext prover keeps working over h2c, so an https-only change would be
// caught.
func TestProverPlaintextStillWorks(t *testing.T) {
	addr := startPlaintextProver(t)

	prover := NewFromEndpoint(network.Endpoint{URL: "http://" + addr}, "1", "client-0", slog.Default())

	height, _, err := prover.LatestProvableHeight(context.Background())
	require.NoError(t, err)
	require.Equal(t, uint64(42), height)
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

	protocols := new(http.Protocols)
	protocols.SetHTTP1(true)
	protocols.SetUnencryptedHTTP2(true)

	srv := newProverServer(protocols)

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)

	go func() { _ = srv.Serve(ln) }()
	t.Cleanup(func() { _ = srv.Close() })

	return ln.Addr().String()
}

func newProverServer(protocols *http.Protocols) *http.Server {
	path, handler := proverv2.NewProverServiceHandler(stubProverService{})
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

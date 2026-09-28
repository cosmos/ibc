// SPDX-License-Identifier: Apache-2.0

package attestor

import (
	"context"
	"crypto/tls"
	"net"
	"net/http"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/require"

	proto "github.com/cosmos/ibc/cli/api/v2/attestor"
	"github.com/cosmos/ibc/cli/internal/config"
	"github.com/cosmos/ibc/cli/internal/testutil/certs"
)

// Exercises the whole chain for a remote attestor: config block -> *tls.Config
// -> transport -> mTLS handshake -> gRPC over HTTP/2.
func TestRemoteAttestorMutualTLS(t *testing.T) {
	ca := certs.NewCA(t)
	dir := t.TempDir()

	caFile := ca.WriteCA(t, dir)
	clientCert, clientKey := ca.WriteLeaf(t, dir, "client")

	addr := startTLSAttestor(t, ca.ServerTLS(t, "localhost", true))

	t.Run("connects when it presents a client certificate", func(t *testing.T) {
		tlsConfig := mustTLSConfig(t, &config.TLSClientConfig{
			CAFile:     caFile,
			CertFile:   clientCert,
			KeyFile:    clientKey,
			ServerName: "localhost",
		})

		remote, err := NewRemoteFromURL(context.Background(), "https://"+addr, "attestor-1", tlsConfig)
		require.NoError(t, err)
		require.Equal(t, "41001", remote.ChainID())

		height, err := remote.LatestHeight(context.Background())
		require.NoError(t, err)
		require.Equal(t, uint64(99), height)
	})

	t.Run("is refused without a client certificate", func(t *testing.T) {
		tlsConfig := mustTLSConfig(t, &config.TLSClientConfig{CAFile: caFile, ServerName: "localhost"})

		_, err := NewRemoteFromURL(context.Background(), "https://"+addr, "attestor-1", tlsConfig)
		require.Error(t, err, "server requires a client certificate")
	})

	t.Run("is refused when the server is not trusted", func(t *testing.T) {
		otherCA := certs.NewCA(t).WriteCA(t, t.TempDir())

		tlsConfig := mustTLSConfig(t, &config.TLSClientConfig{
			CAFile:     otherCA,
			CertFile:   clientCert,
			KeyFile:    clientKey,
			ServerName: "localhost",
		})

		_, err := NewRemoteFromURL(context.Background(), "https://"+addr, "attestor-1", tlsConfig)
		require.Error(t, err, "server certificate is signed by an unknown CA")
	})

	t.Run("plaintext dial against a tls listener fails", func(t *testing.T) {
		_, err := NewRemoteFromURL(context.Background(), "http://"+addr, "attestor-1", nil)
		require.Error(t, err)
	})
}

// insecureSkipVerify is the documented development escape hatch, so pin that
// it really does bypass verification rather than silently doing nothing.
func TestRemoteAttestorInsecureSkipVerify(t *testing.T) {
	ca := certs.NewCA(t)
	addr := startTLSAttestor(t, ca.ServerTLS(t, "localhost", false))

	t.Run("an untrusted server is rejected by default", func(t *testing.T) {
		tlsConfig := mustTLSConfig(t, &config.TLSClientConfig{ServerName: "localhost"})

		_, err := NewRemoteFromURL(context.Background(), "https://"+addr, "attestor-1", tlsConfig)
		require.Error(t, err)
	})

	t.Run("insecureSkipVerify accepts it", func(t *testing.T) {
		tlsConfig := mustTLSConfig(t, &config.TLSClientConfig{
			ServerName:         "localhost",
			InsecureSkipVerify: true,
		})

		remote, err := NewRemoteFromURL(context.Background(), "https://"+addr, "attestor-1", tlsConfig)
		require.NoError(t, err)
		require.Equal(t, "41001", remote.ChainID())
	})
}

// A client floor above what the server offers must fail rather than silently
// downgrade.
func TestRemoteAttestorMinVersionIsEnforced(t *testing.T) {
	ca := certs.NewCA(t)
	dir := t.TempDir()
	caFile := ca.WriteCA(t, dir)

	serverTLS := ca.ServerTLS(t, "localhost", false)
	serverTLS.MaxVersion = tls.VersionTLS12

	addr := startTLSAttestor(t, serverTLS)

	t.Run("1.2 floor connects to a 1.2 server", func(t *testing.T) {
		tlsConfig := mustTLSConfig(t, &config.TLSClientConfig{
			CAFile: caFile, ServerName: "localhost", MinVersion: "1.2",
		})

		_, err := NewRemoteFromURL(context.Background(), "https://"+addr, "attestor-1", tlsConfig)
		require.NoError(t, err)
	})

	t.Run("1.3 floor refuses a 1.2 server", func(t *testing.T) {
		tlsConfig := mustTLSConfig(t, &config.TLSClientConfig{
			CAFile: caFile, ServerName: "localhost", MinVersion: "1.3",
		})

		_, err := NewRemoteFromURL(context.Background(), "https://"+addr, "attestor-1", tlsConfig)
		require.Error(t, err)
	})
}

// attestors[].grpc is a bare host:port, so presence of the tls block is what
// selects https over http. Resolving against a real listener is the only way
// to show the scheme actually followed.
func TestResolveRemoteDerivesScheme(t *testing.T) {
	ca := certs.NewCA(t)
	dir := t.TempDir()
	caFile := ca.WriteCA(t, dir)

	tlsAddr := startTLSAttestor(t, ca.ServerTLS(t, "localhost", false))
	plaintextAddr := startPlaintextAttestor(t)

	t.Run("a tls block dials https", func(t *testing.T) {
		got, err := resolveRemote(context.Background(), config.AttestorConfig{
			Name: "attestor-1",
			Type: config.AttestorTypeRemote,
			GRPC: tlsAddr,
			TLS:  &config.TLSClientConfig{CAFile: caFile, ServerName: "localhost"},
		})
		require.NoError(t, err)
		require.Equal(t, "41001", got.ChainID())
	})

	t.Run("no tls block dials http", func(t *testing.T) {
		got, err := resolveRemote(context.Background(), config.AttestorConfig{
			Name: "attestor-1",
			Type: config.AttestorTypeRemote,
			GRPC: plaintextAddr,
		})
		require.NoError(t, err)
		require.Equal(t, "41001", got.ChainID())
	})

	t.Run("a tls block against a plaintext listener fails", func(t *testing.T) {
		_, err := resolveRemote(context.Background(), config.AttestorConfig{
			Name: "attestor-1",
			Type: config.AttestorTypeRemote,
			GRPC: plaintextAddr,
			TLS:  &config.TLSClientConfig{CAFile: caFile, ServerName: "localhost"},
		})
		require.Error(t, err)
	})

	t.Run("an unreadable certificate is reported as a tls error", func(t *testing.T) {
		_, err := resolveRemote(context.Background(), config.AttestorConfig{
			Name: "attestor-1",
			Type: config.AttestorTypeRemote,
			GRPC: tlsAddr,
			TLS:  &config.TLSClientConfig{CAFile: "/nonexistent/ca.crt"},
		})
		require.ErrorContains(t, err, "tls")
	})
}

// A remote attestor that cannot be resolved is skipped rather than fatal, so
// a certificate mistake silently shrinks the signature set.
func TestResolveFromConfigSkipsUnresolvableRemote(t *testing.T) {
	local, remote, err := ResolveFromConfig(context.Background(), config.Attestors{{
		Name: "attestor-1",
		Type: config.AttestorTypeRemote,
		GRPC: "127.0.0.1:1",
		TLS:  &config.TLSClientConfig{},
	}}, nil, nil)

	require.NoError(t, err)
	require.Empty(t, local)
	require.Empty(t, remote, "an unreachable attestor leaves the quorum without failing startup")
}

func mustTLSConfig(t *testing.T, cfg *config.TLSClientConfig) *tls.Config {
	t.Helper()

	tlsConfig, err := cfg.TLSConfig()
	require.NoError(t, err)

	return tlsConfig
}

// startTLSAttestor serves AttestationService over TLS and returns its address.
func startTLSAttestor(t *testing.T, tlsConfig *tls.Config) string {
	t.Helper()

	protocols := new(http.Protocols)
	protocols.SetHTTP1(true)
	protocols.SetHTTP2(true)

	srv := newAttestorServer(protocols)
	srv.TLSConfig = tlsConfig

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)

	go func() { _ = srv.ServeTLS(ln, "", "") }()
	t.Cleanup(func() { _ = srv.Close() })

	return ln.Addr().String()
}

// startPlaintextAttestor serves the same handler over h2c.
func startPlaintextAttestor(t *testing.T) string {
	t.Helper()

	protocols := new(http.Protocols)
	protocols.SetHTTP1(true)
	protocols.SetUnencryptedHTTP2(true)

	srv := newAttestorServer(protocols)

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)

	go func() { _ = srv.Serve(ln) }()
	t.Cleanup(func() { _ = srv.Close() })

	return ln.Addr().String()
}

func newAttestorServer(protocols *http.Protocols) *http.Server {
	path, handler := proto.NewAttestationServiceHandler(stubAttestationService{})
	mux := http.NewServeMux()
	mux.Handle(path, handler)

	return &http.Server{
		Handler:           mux,
		Protocols:         protocols,
		ReadHeaderTimeout: 3 * time.Second,
	}
}

type stubAttestationService struct{}

var _ proto.AttestationServiceHandler = stubAttestationService{}

func (stubAttestationService) LatestHeight(
	context.Context, *connect.Request[proto.LatestHeightRequest],
) (*connect.Response[proto.LatestHeightResponse], error) {
	return connect.NewResponse(&proto.LatestHeightResponse{Height: 99}), nil
}

func (stubAttestationService) StateAttestation(
	context.Context, *connect.Request[proto.StateAttestationRequest],
) (*connect.Response[proto.StateAttestationResponse], error) {
	return connect.NewResponse(&proto.StateAttestationResponse{Attestation: &proto.Attestation{}}), nil
}

func (stubAttestationService) PacketAttestation(
	context.Context, *connect.Request[proto.PacketAttestationRequest],
) (*connect.Response[proto.PacketAttestationResponse], error) {
	return connect.NewResponse(&proto.PacketAttestationResponse{Attestation: &proto.Attestation{}}), nil
}

func (stubAttestationService) Info(
	context.Context, *connect.Request[proto.InfoRequest],
) (*connect.Response[proto.InfoResponse], error) {
	return connect.NewResponse(&proto.InfoResponse{ChainId: "41001", Address: "0xabc"}), nil
}

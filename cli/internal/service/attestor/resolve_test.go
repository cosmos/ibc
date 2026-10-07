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
	"github.com/cosmos/ibc/cli/internal/chains"
	"github.com/cosmos/ibc/cli/internal/config"
	"github.com/cosmos/ibc/cli/internal/service/signer"
	"github.com/cosmos/ibc/cli/internal/testutil/certs"
)

func TestResolveFromConfig(t *testing.T) {
	ctx := context.Background()

	t.Run("resolvesLocalAndSkipsUnreachableRemote", func(t *testing.T) {
		backingSigner, err := signer.GenerateLocalSecp256k1Signer()
		require.NoError(t, err)

		signers := signer.NewSet()
		signers.Set("key", backingSigner)

		clients := chains.NewClientSet(map[string]chains.Client{"1": stubChainClient(t, "1")})

		entries := config.Attestors{
			{Name: "alice", Type: config.AttestorTypeLocal, ChainID: "1", Signer: "key"},
			{Name: "bob", Type: config.AttestorTypeRemote, GRPC: "127.0.0.1:0"},
		}

		local, remote, err := ResolveFromConfig(ctx, entries, clients, signers, ResolveOptions{})

		require.NoError(t, err)
		require.Len(t, local, 1)
		require.Equal(t, "alice", local[0].Name())
		require.Empty(t, remote, "an unreachable remote is skipped, not fatal")
	})

	t.Run("unreachableRemoteErrorsWhenRequired", func(t *testing.T) {
		entries := config.Attestors{
			{Name: "bob", Type: config.AttestorTypeRemote, GRPC: "127.0.0.1:0"},
		}

		_, _, err := ResolveFromConfig(
			ctx, entries, chains.NewClientSet(nil), signer.NewSet(), ResolveOptions{RequireReachable: true},
		)

		require.ErrorContains(t, err, "attestor bob")
	})

	t.Run("remoteBadTLSConfigErrorsFatally", func(t *testing.T) {
		entries := config.Attestors{{
			Name: "bob",
			Type: config.AttestorTypeRemote,
			GRPC: "127.0.0.1:0",
			TLS:  &config.TLSClientConfig{CAFile: "/nonexistent/ca.pem"},
		}}

		_, _, err := ResolveFromConfig(ctx, entries, chains.NewClientSet(nil), signer.NewSet(), ResolveOptions{})

		require.ErrorContains(t, err, "attestor bob")
		require.ErrorContains(t, err, "caFile")
	})

	t.Run("localMissingClientErrorsFatally", func(t *testing.T) {
		signers := signer.NewSet()
		s, err := signer.GenerateLocalSecp256k1Signer()
		require.NoError(t, err)
		signers.Set("key", s)

		entries := config.Attestors{
			{Name: "alice", Type: config.AttestorTypeLocal, ChainID: "unknown-chain", Signer: "key"},
		}

		_, _, err = ResolveFromConfig(ctx, entries, chains.NewClientSet(nil), signers, ResolveOptions{})

		require.ErrorContains(t, err, "attestor alice")
		require.ErrorContains(t, err, "client not found for chain unknown-chain")
	})

	t.Run("localMissingSignerErrorsFatally", func(t *testing.T) {
		clients := chains.NewClientSet(map[string]chains.Client{"1": stubChainClient(t, "1")})

		entries := config.Attestors{
			{Name: "alice", Type: config.AttestorTypeLocal, ChainID: "1", Signer: "missing"},
		}

		_, _, err := ResolveFromConfig(ctx, entries, clients, signer.NewSet(), ResolveOptions{})

		require.ErrorContains(t, err, "attestor alice")
		require.ErrorContains(t, err, "unknown signer missing")
	})
}

// attestors[].grpc is a bare host:port, so presence of the tls block is what
// selects https over http. Resolving against a real listener is the only way
// to show the scheme actually followed.
func TestResolveRemoteDerivesScheme(t *testing.T) {
	ca := certs.NewCA(t)
	dir := t.TempDir()
	caFile := ca.WriteCA(t, dir)

	clientCert, clientKey := ca.WriteLeaf(t, dir, "client")

	tlsAddr := startTLSAttestor(t, ca.ServerTLS(t, "localhost", true))

	plaintextAddr := startPlaintextAttestor(t)

	t.Run("a tls block dials https", func(t *testing.T) {
		got, err := resolveRemote(t, config.AttestorConfig{
			Name: "attestor-1",
			Type: config.AttestorTypeRemote,
			GRPC: tlsAddr,
			TLS: &config.TLSClientConfig{
				CAFile:     caFile,
				CertFile:   clientCert,
				KeyFile:    clientKey,
				ServerName: "localhost",
			},
		})
		require.NoError(t, err)
		require.Equal(t, "41001", got.ChainID())
	})

	t.Run("no tls block dials http", func(t *testing.T) {
		got, err := resolveRemote(t, config.AttestorConfig{
			Name: "attestor-1",
			Type: config.AttestorTypeRemote,
			GRPC: plaintextAddr,
		})
		require.NoError(t, err)
		require.Equal(t, "41001", got.ChainID())
	})

	t.Run("a tls block against a plaintext listener fails", func(t *testing.T) {
		_, err := resolveRemote(t, config.AttestorConfig{
			Name: "attestor-1",
			Type: config.AttestorTypeRemote,
			GRPC: plaintextAddr,
			TLS:  &config.TLSClientConfig{CAFile: caFile, ServerName: "localhost"},
		})
		require.Error(t, err)
	})
}

func resolveRemote(t *testing.T, entry config.AttestorConfig) (Attestor, error) {
	t.Helper()

	endpoint, err := remoteEndpoint(entry)
	require.NoError(t, err)

	return NewRemoteFromEndpoint(context.Background(), entry.Name, endpoint)
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

// startPlaintextAttestor serves AttestationService without TLS over HTTP/1.1
// only, so it also pins that plaintext attestors don't require h2c.
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

type stubAttestationService struct {
	proto.UnimplementedAttestationServiceHandler
}

func (stubAttestationService) Info(
	context.Context, *connect.Request[proto.InfoRequest],
) (*connect.Response[proto.InfoResponse], error) {
	return connect.NewResponse(&proto.InfoResponse{ChainId: "41001", Address: "0xabc"}), nil
}

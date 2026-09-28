// SPDX-License-Identifier: Apache-2.0

package signer

import (
	"context"
	"crypto/tls"
	"net"
	"path/filepath"
	"testing"

	"github.com/cosmos/kms/gen/signerservice"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"

	"github.com/cosmos/ibc/cli/internal/config"
	"github.com/cosmos/ibc/cli/internal/testutil/certs"
)

// The signer is the only surface on gRPC-Go rather than ConnectRPC, so its
// transport credentials are exercised end to end rather than by inspection.
func TestRemoteSignerMutualTLS(t *testing.T) {
	ca := certs.NewCA(t)
	dir := t.TempDir()

	caFile := ca.WriteCA(t, dir)
	clientCert, clientKey := ca.WriteLeaf(t, dir, "client")

	addr := startTLSKMS(t, ca.ServerTLS(t, "localhost", true))

	t.Run("signs when it presents a client certificate", func(t *testing.T) {
		tlsConfig := mustSignerTLSConfig(t, &config.TLSClientConfig{
			CAFile:     caFile,
			CertFile:   clientCert,
			KeyFile:    clientKey,
			ServerName: "localhost",
		})

		s, err := NewRemoteFromURL(context.Background(), addr, "key-1", tlsConfig)
		require.NoError(t, err)
		require.Equal(t, EDDSA, s.Type())

		sig, err := s.Sign(context.Background(), []byte("message"))
		require.NoError(t, err)
		require.Equal(t, []byte("signature"), sig)
	})

	t.Run("is refused without a client certificate", func(t *testing.T) {
		tlsConfig := mustSignerTLSConfig(t, &config.TLSClientConfig{CAFile: caFile, ServerName: "localhost"})

		_, err := NewRemoteFromURL(context.Background(), addr, "key-1", tlsConfig)
		require.Error(t, err, "KMS requires a client certificate")
	})

	t.Run("is refused when the KMS is not trusted", func(t *testing.T) {
		otherCA := certs.NewCA(t).WriteCA(t, t.TempDir())

		tlsConfig := mustSignerTLSConfig(t, &config.TLSClientConfig{
			CAFile:     otherCA,
			CertFile:   clientCert,
			KeyFile:    clientKey,
			ServerName: "localhost",
		})

		_, err := NewRemoteFromURL(context.Background(), addr, "key-1", tlsConfig)
		require.Error(t, err)
	})

	t.Run("plaintext credentials are refused by a tls listener", func(t *testing.T) {
		_, err := NewRemoteFromURL(context.Background(), addr, "key-1", nil)
		require.Error(t, err)
	})
}

// NewSignerFromConfig is where a signer's tls block is resolved, so cover the
// config-driven path rather than only the direct constructor.
func TestNewSignerFromConfigTLS(t *testing.T) {
	ca := certs.NewCA(t)
	dir := t.TempDir()

	caFile := ca.WriteCA(t, dir)
	clientCert, clientKey := ca.WriteLeaf(t, dir, "client")

	addr := startTLSKMS(t, ca.ServerTLS(t, "localhost", true))

	t.Run("resolves a remote signer over mTLS", func(t *testing.T) {
		s, alias, err := NewSignerFromConfig(context.Background(), config.SignerConfig{
			Alias:       "kms",
			Type:        config.SignerRemote,
			GRPC:        addr,
			RemoteKeyID: "key-1",
			TLS: &config.TLSClientConfig{
				CAFile:     caFile,
				CertFile:   clientCert,
				KeyFile:    clientKey,
				ServerName: "localhost",
			},
		})
		require.NoError(t, err)
		require.Equal(t, "kms", alias)
		require.False(t, s.IsLocal())
	})

	t.Run("a signer without a tls block cannot reach a tls KMS", func(t *testing.T) {
		_, _, err := NewSignerFromConfig(context.Background(), config.SignerConfig{
			Alias:       "kms",
			Type:        config.SignerRemote,
			GRPC:        addr,
			RemoteKeyID: "key-1",
		})
		require.Error(t, err)
	})

	t.Run("an unreadable certificate fails before dialing", func(t *testing.T) {
		_, _, err := NewSignerFromConfig(context.Background(), config.SignerConfig{
			Alias:       "kms",
			Type:        config.SignerRemote,
			GRPC:        addr,
			RemoteKeyID: "key-1",
			TLS:         &config.TLSClientConfig{CertFile: clientCert, KeyFile: filepath.Join(dir, "absent.key")},
		})
		require.ErrorContains(t, err, "tls")
	})
}

func mustSignerTLSConfig(t *testing.T, cfg *config.TLSClientConfig) *tls.Config {
	t.Helper()

	tlsConfig, err := cfg.TLSConfig()
	require.NoError(t, err)

	return tlsConfig
}

func startTLSKMS(t *testing.T, tlsConfig *tls.Config) string {
	t.Helper()

	srv := grpc.NewServer(grpc.Creds(credentials.NewTLS(tlsConfig)))
	signerservice.RegisterSignerServiceServer(srv, stubSignerService{})

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)

	go func() { _ = srv.Serve(ln) }()
	t.Cleanup(srv.Stop)

	return ln.Addr().String()
}

type stubSignerService struct {
	signerservice.UnimplementedSignerServiceServer
}

func (stubSignerService) GetKey(
	_ context.Context, req *signerservice.GetKeyRequest,
) (*signerservice.GetKeyResponse, error) {
	return &signerservice.GetKeyResponse{
		Key: &signerservice.Key{
			Id:     req.GetId(),
			Pubkey: []byte("public-key"),
			Scheme: signerservice.SignatureScheme_ED25519,
		},
	}, nil
}

func (stubSignerService) Sign(
	context.Context, *signerservice.SignRequest,
) (*signerservice.SignResponse, error) {
	return &signerservice.SignResponse{Signature: []byte("signature")}, nil
}

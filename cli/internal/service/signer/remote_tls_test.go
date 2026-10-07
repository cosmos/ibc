// SPDX-License-Identifier: Apache-2.0

package signer

import (
	"context"
	"crypto/tls"
	"net"
	"testing"

	"github.com/cosmos/kms/gen/signerservice"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"

	"github.com/cosmos/ibc/cli/internal/config"
	"github.com/cosmos/ibc/cli/internal/testutil/certs"
)

// The signer's only TLS logic is picking credentials.NewTLS over insecure
// for a configured tls block; handshake behavior itself is gRPC-Go's.
func TestNewSignerFromConfigTLS(t *testing.T) {
	ca := certs.NewCA(t)
	dir := t.TempDir()

	caFile := ca.WriteCA(t, dir)
	clientCert, clientKey := ca.WriteLeaf(t, dir, "client")

	addr := startTLSKMS(t, ca.ServerTLS(t, "localhost", true))

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

	sig, err := s.Sign(context.Background(), []byte("message"))
	require.NoError(t, err)
	require.Equal(t, []byte("signature"), sig)
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

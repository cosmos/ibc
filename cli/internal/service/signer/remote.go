// SPDX-License-Identifier: Apache-2.0

package signer

import (
	"context"
	"log/slog"
	"time"

	"github.com/cosmos/kms/gen/signerservice"
	"github.com/pkg/errors"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"

	"github.com/cosmos/ibc/cli/internal/network"
	"github.com/cosmos/ibc/cli/keyfile"
)

// RemoteSigner wraps KMS remote signer.
type RemoteSigner struct {
	client signerservice.SignerServiceClient
	keyID  string

	key     *signerservice.Key
	keyType keyfile.Type

	logger *slog.Logger
}

var _ Signer = &RemoteSigner{}

func NewRemote(ctx context.Context, client signerservice.SignerServiceClient, keyID string) (*RemoteSigner, error) {
	s := &RemoteSigner{
		client:  client,
		keyID:   keyID,
		key:     nil,
		keyType: "",
		logger:  slog.With("module", "signer", "source", "remote", "key_id", keyID),
	}

	if err := s.setup(ctx); err != nil {
		return nil, errors.Wrap(err, "setup failed")
	}

	return s, nil
}

// NewRemoteFromEndpoint dials the KMS at endpoint, over TLS when
// endpoint.TLS is set, and resolves keyID.
func NewRemoteFromEndpoint(ctx context.Context, keyID string, endpoint network.Endpoint) (*RemoteSigner, error) {
	grpcClient, err := newGRPCClient(endpoint)
	if err != nil {
		return nil, errors.Wrap(err, "unable to create grpc client")
	}

	signerClient := signerservice.NewSignerServiceClient(grpcClient)

	s, err := NewRemote(ctx, signerClient, keyID)
	if err != nil {
		if errClose := grpcClient.Close(); errClose != nil {
			slog.Error("failed to close grpc client", "err", errClose)
		}

		return nil, err
	}

	return s, nil
}

func (r *RemoteSigner) IsLocal() bool      { return false }
func (r *RemoteSigner) Type() keyfile.Type { return r.keyType }

func (r *RemoteSigner) PublicKey() []byte {
	return r.key.Pubkey
}

func (r *RemoteSigner) Sign(ctx context.Context, message []byte) ([]byte, error) {
	r.logger.Debug("Sending sign request", "message", message)

	resp, err := r.client.Sign(ctx, &signerservice.SignRequest{
		KeyId:   r.keyID,
		Payload: bytesToPayload(message),
	})
	if err != nil {
		return nil, errors.Wrap(err, "sign request failed")
	}

	return resp.Signature, nil
}

// fetch key's information from KMS and set fields
func (r *RemoteSigner) setup(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	resp, err := r.client.GetKey(ctx, &signerservice.GetKeyRequest{Id: r.keyID})
	switch {
	case err != nil:
		return errors.Wrap(err, "get key request failed")
	case resp.Key == nil:
		return errors.New("get key response did not include key")
	}

	r.key = resp.Key

	r.keyType, err = keyTypeFromProto(resp.Key.Scheme)
	if err != nil {
		return err
	}

	return nil
}

func keyTypeFromProto(scheme signerservice.SignatureScheme) (keyfile.Type, error) {
	switch scheme {
	case signerservice.SignatureScheme_ED25519:
		return EDDSA, nil
	case signerservice.SignatureScheme_ECDSA_SECP256K1ETH:
		return ECDSA, nil
	default:
		return "", errors.Errorf("unsupported remote key scheme: %s", scheme)
	}
}

func newGRPCClient(endpoint network.Endpoint) (*grpc.ClientConn, error) {
	creds := insecure.NewCredentials()
	if endpoint.TLS != nil {
		creds = credentials.NewTLS(endpoint.TLS)
	}

	return grpc.NewClient(endpoint.URL, grpc.WithTransportCredentials(creds))
}

func bytesToPayload(message []byte) *signerservice.Payload {
	return &signerservice.Payload{
		Kind: &signerservice.Payload_Generic{
			Generic: message,
		},
	}
}

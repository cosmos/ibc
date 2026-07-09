package signer

import (
	"context"
	"log/slog"

	"github.com/cosmos/kms/gen/signerservice"
	"github.com/pkg/errors"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// RemoteSigner wraps KMS remote signer.
type RemoteSigner struct {
	client signerservice.SignerServiceClient
	keyID  string

	key     *signerservice.Key
	keyType KeyType

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

func NewRemoteFromURL(ctx context.Context, grpcURL, keyID string) (*RemoteSigner, error) {
	grpcClient, err := newGRPCClientFromURL(grpcURL)
	if err != nil {
		return nil, errors.Wrap(err, "unable to create grpc client")
	}

	signerClient := signerservice.NewSignerServiceClient(grpcClient)

	return NewRemote(ctx, signerClient, keyID)
}

func (r *RemoteSigner) IsLocal() bool { return false }
func (r *RemoteSigner) Type() KeyType { return r.keyType }

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

func keyTypeFromProto(scheme signerservice.SignatureScheme) (KeyType, error) {
	switch scheme {
	case signerservice.SignatureScheme_ED25519:
		return EDDSA, nil
	case signerservice.SignatureScheme_ECDSA_SECP256K1ETH:
		return ECDSA, nil
	default:
		return "", errors.Errorf("unsupported remote key scheme: %s", scheme)
	}
}

// todo: revisit security if needed. we can convert `signer.grpc string` to `signer.grpc{<options>}`
func newGRPCClientFromURL(url string) (*grpc.ClientConn, error) {
	return grpc.NewClient(url, grpc.WithTransportCredentials(insecure.NewCredentials()))
}

func bytesToPayload(message []byte) *signerservice.Payload {
	return &signerservice.Payload{
		Kind: &signerservice.Payload_Generic{
			Generic: message,
		},
	}
}

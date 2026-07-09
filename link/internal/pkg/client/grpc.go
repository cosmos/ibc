package client

import (
	"crypto/tls"
	"net/url"

	"github.com/pkg/errors"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
)

func NewGRPCClientFromURL(url string) (*grpc.ClientConn, error) {
	creds, err := grpcCredentials(url)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create grpc credentials")
	}

	return grpc.NewClient(url, grpc.WithTransportCredentials(creds))
}

func grpcCredentials(grpcURL string) (credentials.TransportCredentials, error) {
	parsed, err := url.Parse(grpcURL)
	if err != nil {
		return nil, errors.Wrap(err, "invalid signer grpc url")
	}

	switch parsed.Scheme {
	case "https":
		return credentials.NewTLS(&tls.Config{MinVersion: tls.VersionTLS12}), nil
	case "http":
		return insecure.NewCredentials(), nil
	case "":
		return insecure.NewCredentials(), nil
	default:
		return insecure.NewCredentials(), nil
	}
}

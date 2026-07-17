// Package proofapi connects to the proof api that builds relay transactions.
package proofapi

import (
	"crypto/tls"
	"net/http"

	"connectrpc.com/connect"

	"github.com/cosmos/ibc/link/internal/config"

	proto "github.com/cosmos/ibc/link/internal/types/proofapi"
)

// relay transactions carry proofs and can exceed default message limits
const maxResponseBytes = 10 * 1024 * 1024

// NewClient creates a proof api client speaking gRPC.
func NewClient(cfg config.ProofAPIConfig) proto.ProofApiServiceClient {
	protocols := new(http.Protocols)
	protocols.SetHTTP1(true)
	protocols.SetHTTP2(true)

	transport := &http.Transport{Protocols: protocols}

	baseURL := "http://" + cfg.GRPC
	if cfg.TLSEnabled {
		baseURL = "https://" + cfg.GRPC
		transport.TLSClientConfig = &tls.Config{MinVersion: tls.VersionTLS13}
	} else {
		protocols.SetUnencryptedHTTP2(true)
	}

	return proto.NewProofApiServiceClient(
		&http.Client{Transport: transport},
		baseURL,
		connect.WithGRPC(),
		connect.WithReadMaxBytes(maxResponseBytes),
	)
}

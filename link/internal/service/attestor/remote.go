package attestor

import (
	"context"
	"log/slog"
	"net/http"

	"connectrpc.com/connect"

	proto "github.com/cosmos/ibc/link/api/v2/attestor"
)

// RemoteAttestor provides attestation data from a remote gRPC service.
// Used in one-way A->B relaying path.
type RemoteAttestor struct {
	chainID string
	name    string
	alias   string
	client  proto.AttestationServiceClient
	logger  *slog.Logger
}

var _ Attestor = &RemoteAttestor{}

func NewRemoteFromURL(chainID, name, alias, grpcURL string) *RemoteAttestor {
	var (
		httpClient  = newConnectHTTPClient()
		protoClient = proto.NewAttestationServiceClient(httpClient, grpcURL, connect.WithGRPC())
	)

	return NewRemote(chainID, name, alias, protoClient)
}

func NewRemote(chainID, name, alias string, client proto.AttestationServiceClient) *RemoteAttestor {
	return &RemoteAttestor{
		chainID: chainID,
		name:    name,
		alias:   alias,
		client:  client,
		logger:  slog.With("module", "attestor", "name", attestorFQN("remote", chainID, name)),
	}
}

func (a *RemoteAttestor) LatestAttestableHeight(ctx context.Context) (uint64, error) {
	req := &proto.LatestAttestableHeightRequest{
		Attestor: a.name,
	}

	res, err := a.client.LatestAttestableHeight(ctx, connect.NewRequest(req))
	if err != nil {
		return 0, err
	}

	return res.Msg.Height, nil
}

func (a *RemoteAttestor) Name() string    { return a.name }
func (a *RemoteAttestor) Alias() string   { return a.alias }
func (a *RemoteAttestor) ChainID() string { return a.chainID }
func (a *RemoteAttestor) IsLocal() bool   { return false }

// https://connectrpc.com/docs/go/getting-started/#make-requests
// todo: revisit these params
func newConnectHTTPClient() *http.Client {
	protocols := new(http.Protocols)
	protocols.SetHTTP1(true)
	protocols.SetHTTP2(true)
	protocols.SetUnencryptedHTTP2(true)

	return &http.Client{
		Transport: &http.Transport{Protocols: protocols},
	}
}

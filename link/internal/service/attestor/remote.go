package attestor

import (
	"context"
	"log/slog"
	"net/http"

	"connectrpc.com/connect"
	"github.com/pkg/errors"

	proto "github.com/cosmos/ibc/link/internal/types/v2/attestor"
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

func (a *RemoteAttestor) StateAttestation(ctx context.Context, height uint64) (Attestation, error) {
	req := &proto.StateAttestationRequest{
		Attestor: a.name,
		Height:   height,
	}

	res, err := a.client.StateAttestation(ctx, connect.NewRequest(req))
	if err != nil {
		return Attestation{}, err
	}

	return attestationFromProto(res.Msg.Attestation)
}

func (a *RemoteAttestor) PacketAttestation(ctx context.Context, req PacketAttestationRequest) (Attestation, error) {
	protoReq := &proto.PacketAttestationRequest{
		Attestor:       a.name,
		Packets:        req.Packets,
		Height:         req.Height,
		CommitmentType: CommitmentTypeToProto(req.CommitmentType),
	}

	res, err := a.client.PacketAttestation(ctx, connect.NewRequest(protoReq))
	if err != nil {
		return Attestation{}, err
	}

	return attestationFromProto(res.Msg.Attestation)
}

func (a *RemoteAttestor) Name() string    { return a.name }
func (a *RemoteAttestor) Alias() string   { return a.alias }
func (a *RemoteAttestor) ChainID() string { return a.chainID }
func (a *RemoteAttestor) IsLocal() bool   { return false }

func attestationFromProto(a *proto.Attestation) (Attestation, error) {
	if a == nil {
		return Attestation{}, errors.New("attestation is nil")
	}

	return Attestation{
		Height: a.Height,
	}, nil
}

func CommitmentTypeToProto(ct CommitmentType) proto.CommitmentType {
	switch ct {
	case CommitmentTypePacket:
		return proto.CommitmentType_COMMITMENT_TYPE_PACKET
	case CommitmentTypeAck:
		return proto.CommitmentType_COMMITMENT_TYPE_ACK
	case CommitmentTypeReceipt:
		return proto.CommitmentType_COMMITMENT_TYPE_RECEIPT
	default:
		return proto.CommitmentType_COMMITMENT_TYPE_INVALID
	}
}

func CommitmentTypeFromProto(ct proto.CommitmentType) (CommitmentType, error) {
	switch ct {
	case proto.CommitmentType_COMMITMENT_TYPE_PACKET:
		return CommitmentTypePacket, nil
	case proto.CommitmentType_COMMITMENT_TYPE_ACK:
		return CommitmentTypeAck, nil
	case proto.CommitmentType_COMMITMENT_TYPE_RECEIPT:
		return CommitmentTypeReceipt, nil
	default:
		return CommitmentTypeInvalid, errors.Errorf("unsupported commitment type: %s", ct)
	}
}

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

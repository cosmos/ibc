package attestor

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"connectrpc.com/connect"
	"github.com/pkg/errors"

	proto "github.com/cosmos/ibc/link/api/v2/attestor"
)

// RemoteAttestor provides attestation data from a remote gRPC service.
// Used in one-way A->B relaying path. Constructed only after resolution has
// already learned its address/finality offset via Info -- fully resolved and
// immutable once built.
type RemoteAttestor struct {
	chainID        string
	name           string
	address        string
	finalityOffset uint64
	client         proto.AttestationServiceClient
	logger         *slog.Logger
}

var _ Attestor = &RemoteAttestor{}

const remoteRequestTimeout = 5 * time.Second

func NewRemoteFromURL(chainID, name, address string, finalityOffset uint64, grpcURL string) *RemoteAttestor {
	var (
		httpClient  = newConnectHTTPClient()
		protoClient = proto.NewAttestationServiceClient(httpClient, grpcURL, connect.WithGRPC())
	)

	return NewRemote(chainID, name, address, finalityOffset, protoClient)
}

func NewRemote(
	chainID, name, address string,
	finalityOffset uint64,
	client proto.AttestationServiceClient,
) *RemoteAttestor {
	return &RemoteAttestor{
		chainID:        chainID,
		name:           name,
		address:        address,
		finalityOffset: finalityOffset,
		client:         client,
		logger:         slog.With("module", "attestor", "name", attestorFQN("remote", chainID, name)),
	}
}

// QueryInfo queries a remote attestor endpoint's Info RPC for the attestor
// identified by name, without constructing a full RemoteAttestor -- used
// during quorum resolution before it's known whether this candidate should
// be kept.
func QueryInfo(ctx context.Context, grpcURL, name string) (chainID, address string, finalityOffset uint64, err error) {
	ctx, cancel := context.WithTimeout(ctx, remoteRequestTimeout)
	defer cancel()

	httpClient := newConnectHTTPClient()
	protoClient := proto.NewAttestationServiceClient(httpClient, grpcURL, connect.WithGRPC())

	res, err := protoClient.Info(ctx, connect.NewRequest(&proto.InfoRequest{Attestor: name}))
	if err != nil {
		return "", "", 0, err
	}

	return res.Msg.ChainId, res.Msg.Address, res.Msg.FinalityOffset, nil
}

func (a *RemoteAttestor) LatestHeight(ctx context.Context) (uint64, error) {
	ctx, cancel := context.WithTimeout(ctx, remoteRequestTimeout)
	defer cancel()

	req := &proto.LatestHeightRequest{
		Attestor: a.name,
	}

	res, err := a.client.LatestHeight(ctx, connect.NewRequest(req))
	if err != nil {
		return 0, err
	}

	return res.Msg.Height, nil
}

func (a *RemoteAttestor) StateAttestation(ctx context.Context, height uint64) (Attestation, error) {
	ctx, cancel := context.WithTimeout(ctx, remoteRequestTimeout)
	defer cancel()

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
	ctx, cancel := context.WithTimeout(ctx, remoteRequestTimeout)
	defer cancel()

	ct, err := CommitmentTypeToProto(req.CommitmentType)
	if err != nil {
		return Attestation{}, err
	}

	protoReq := &proto.PacketAttestationRequest{
		Attestor:       a.name,
		Packets:        req.Packets,
		Height:         req.Height,
		CommitmentType: ct,
	}

	res, err := a.client.PacketAttestation(ctx, connect.NewRequest(protoReq))
	if err != nil {
		return Attestation{}, err
	}

	return attestationFromProto(res.Msg.Attestation)
}

func (a *RemoteAttestor) Name() string           { return a.name }
func (a *RemoteAttestor) Alias() string          { return a.name }
func (a *RemoteAttestor) ChainID() string        { return a.chainID }
func (a *RemoteAttestor) IsLocal() bool          { return false }
func (a *RemoteAttestor) Address() string        { return a.address }
func (a *RemoteAttestor) FinalityOffset() uint64 { return a.finalityOffset }

func attestationFromProto(a *proto.Attestation) (Attestation, error) {
	if a == nil {
		return Attestation{}, errors.New("attestation is nil")
	}

	var timestamp *time.Time
	if a.Timestamp != nil {
		t := time.Unix(int64(*a.Timestamp), 0)
		timestamp = &t
	}

	return Attestation{
		Height:       a.Height,
		Timestamp:    timestamp,
		AttestedData: a.AttestedData,
		Signature:    a.Signature,
	}, nil
}

func CommitmentTypeToProto(ct CommitmentType) (proto.CommitmentType, error) {
	switch ct {
	case CommitmentTypePacket:
		return proto.CommitmentType_COMMITMENT_TYPE_PACKET, nil
	case CommitmentTypeAck:
		return proto.CommitmentType_COMMITMENT_TYPE_ACK, nil
	case CommitmentTypeReceipt:
		return proto.CommitmentType_COMMITMENT_TYPE_RECEIPT, nil
	default:
		return 0, errors.Errorf("unsupported commitment type: %d", ct)
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

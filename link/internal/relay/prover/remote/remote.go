// SPDX-License-Identifier: Apache-2.0

// Package remote implements the relayer's Prover against a ProverService, so a
// custom light client is a service rather than code in the relayer.
package remote

import (
	"context"
	"net/http"
	"time"

	"connectrpc.com/connect"
	"github.com/pkg/errors"

	channeltypesv2 "github.com/cosmos/ibc-go/v11/modules/core/04-channel/v2/types"
	proverv2 "github.com/cosmos/ibc/link/api/v2/prover"
	v2 "github.com/cosmos/ibc/link/internal/types/v2"
)

// Prover proves one light client remotely. Every request names the client, so
// one service can serve many.
type Prover struct {
	client   proverv2.ProverServiceClient
	chainID  string
	clientID string
}

func New(httpClient connect.HTTPClient, url, chainID, clientID string) *Prover {
	return &Prover{
		client:   proverv2.NewProverServiceClient(httpClient, url, connect.WithGRPC()),
		chainID:  chainID,
		clientID: clientID,
	}
}

// NewFromURL dials url with the default client, which needs no extra setup for
// an https url.
//
// TODO: custom CAs, client certificates, and request timeouts need a client
// passed to New instead; RemoteProverParams is where those settings belong.
func NewFromURL(url, chainID, clientID string) *Prover {
	return New(http.DefaultClient, url, chainID, clientID)
}

func (p *Prover) target() *proverv2.Client {
	return &proverv2.Client{ChainId: p.chainID, ClientId: p.clientID}
}

func (p *Prover) LatestProvableHeight(ctx context.Context) (uint64, time.Time, error) {
	res, err := p.client.LatestProvableHeight(ctx, connect.NewRequest(&proverv2.LatestProvableHeightRequest{
		Client: p.target(),
	}))
	if err != nil {
		return 0, time.Time{}, errors.Wrap(err, "remote prover: latest provable height")
	}

	return res.Msg.GetHeight(), time.Unix(res.Msg.GetTimestamp(), 0).UTC(), nil
}

func (p *Prover) StateProof(ctx context.Context, height uint64) ([]byte, error) {
	res, err := p.client.StateProof(ctx, connect.NewRequest(&proverv2.StateProofRequest{
		Client: p.target(),
		Height: height,
	}))
	if err != nil {
		return nil, errors.Wrap(err, "remote prover: state proof")
	}

	return res.Msg.GetProof(), nil
}

func (p *Prover) PacketProofs(
	ctx context.Context,
	height uint64,
	kind v2.ProofKind,
	packets []channeltypesv2.Packet,
) ([][]byte, error) {
	protoKind, err := proofKindToProto(kind)
	if err != nil {
		return nil, err
	}

	res, err := p.client.PacketProofs(ctx, connect.NewRequest(&proverv2.PacketProofsRequest{
		Client:  p.target(),
		Height:  height,
		Kind:    protoKind,
		Packets: packetsToProto(packets),
	}))
	if err != nil {
		return nil, errors.Wrap(err, "remote prover: packet proofs")
	}

	proofs := res.Msg.GetProofs()
	// Proofs match packets by index, so a short response misattributes them.
	if len(proofs) != len(packets) {
		return nil, errors.Errorf(
			"remote prover returned %d proofs for %d packets", len(proofs), len(packets),
		)
	}

	return proofs, nil
}

func proofKindToProto(kind v2.ProofKind) (proverv2.ProofKind, error) {
	switch kind {
	case v2.ProofKindPacketCommitment:
		return proverv2.ProofKind_PROOF_KIND_PACKET_COMMITMENT, nil
	case v2.ProofKindAcknowledgement:
		return proverv2.ProofKind_PROOF_KIND_ACKNOWLEDGEMENT, nil
	case v2.ProofKindReceiptAbsence:
		return proverv2.ProofKind_PROOF_KIND_RECEIPT_ABSENCE, nil
	default:
		return proverv2.ProofKind_PROOF_KIND_UNSPECIFIED,
			errors.Errorf("remote prover: proof kind %d has no wire representation", kind)
	}
}

// packetsToProto converts without reshaping: an empty list stays nil, so a
// proof covers exactly the packet that was sent.
func packetsToProto(packets []channeltypesv2.Packet) []*proverv2.Packet {
	if len(packets) == 0 {
		return nil
	}

	out := make([]*proverv2.Packet, len(packets))
	for i, packet := range packets {
		out[i] = &proverv2.Packet{
			Sequence:          packet.Sequence,
			SourceClient:      packet.SourceClient,
			DestinationClient: packet.DestinationClient,
			TimeoutTimestamp:  packet.TimeoutTimestamp,
			Payloads:          payloadsToProto(packet.Payloads),
		}
	}

	return out
}

func payloadsToProto(payloads []channeltypesv2.Payload) []*proverv2.Payload {
	if len(payloads) == 0 {
		return nil
	}

	out := make([]*proverv2.Payload, len(payloads))
	for i, payload := range payloads {
		out[i] = &proverv2.Payload{
			SourcePort:      payload.SourcePort,
			DestinationPort: payload.DestinationPort,
			Version:         payload.Version,
			Encoding:        payload.Encoding,
			Value:           payload.Value,
		}
	}

	return out
}

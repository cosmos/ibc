// SPDX-License-Identifier: Apache-2.0

// Package remote implements the relayer's Prover against a ProverService over
// gRPC, so a custom light client can be supported by running a service rather
// than by linking Go code into the relayer.
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

// Prover proves one configured light client by calling a remote service. The
// chain and client ids are sent on every request, so one service can serve
// many clients.
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

// NewFromURL dials url with a plain HTTP/2 client.
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
	// Proofs are matched to packets by index, so a short response would
	// silently attach the wrong proof to a packet.
	if len(proofs) != len(packets) {
		return nil, errors.Errorf(
			"remote prover returned %d proofs for %d packets", len(proofs), len(packets),
		)
	}

	return proofs, nil
}

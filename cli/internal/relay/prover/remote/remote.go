// SPDX-License-Identifier: Apache-2.0

// Package remote implements the relayer's Prover against a ProverService, so a
// custom light client is a service rather than code in the relayer.
package remote

import (
	"context"
	"log/slog"
	"net/url"
	"time"

	"connectrpc.com/connect"
	"github.com/pkg/errors"

	channeltypesv2 "github.com/cosmos/ibc-go/v11/modules/core/04-channel/v2/types"
	proverv2 "github.com/cosmos/ibc/cli/api/v2/prover"
	"github.com/cosmos/ibc/cli/internal/network"
	v2 "github.com/cosmos/ibc/cli/internal/types/v2"
)

const requestTimeout = time.Minute

// Prover proves one light client remotely. Every request names the client, so
// one service can serve many.
type Prover struct {
	client   proverv2.ProverServiceClient
	chainID  string
	clientID string
	logger   *slog.Logger
}

func New(httpClient connect.HTTPClient, url, chainID, clientID string, logger *slog.Logger) *Prover {
	return &Prover{
		client:   proverv2.NewProverServiceClient(httpClient, url, connect.WithGRPC()),
		chainID:  chainID,
		clientID: clientID,
		logger: logger.With(
			"module", "remoteProver",
			"chainID", chainID,
			"clientID", clientID,
			"url", LogSafeURL(url),
		),
	}
}

// NewFromEndpoint dials endpoint with network.NewGRPCHTTPClient.
func NewFromEndpoint(endpoint network.Endpoint, chainID, clientID string, logger *slog.Logger) *Prover {
	return New(network.NewGRPCHTTPClient(endpoint), endpoint.URL, chainID, clientID, logger)
}

// Probe checks that the prover answers. An error the prover itself returned
// still counts as reachable (e.g. NotFound while it syncs), unless it rejects
// this caller, doesn't serve the ProverService, or is Unavailable, which is
// also what a proxy in front of a down prover answers with.
func (p *Prover) Probe(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, requestTimeout)
	defer cancel()

	_, err := p.client.LatestProvableHeight(ctx, connect.NewRequest(&proverv2.LatestProvableHeightRequest{
		Client: p.target(),
	}))
	if err == nil || (connect.IsWireError(err) && !rejectsProbe(connect.CodeOf(err))) {
		return nil
	}

	return errors.Wrap(err, "remote prover: probe")
}

func rejectsProbe(code connect.Code) bool {
	switch code {
	case connect.CodeUnauthenticated, connect.CodePermissionDenied, connect.CodeUnimplemented,
		connect.CodeUnavailable:
		return true
	default:
		return false
	}
}

// LogSafeURL reduces raw to scheme://host[:port] so it is safe to log.
// Validation doesn't reject userinfo, and providers commonly carry a
// credential either there (a token as the username, which url.Redacted
// leaves intact) or in the path, so neither is kept. Falls back to a fixed
// placeholder on a parse failure, which shouldn't happen for a URL that
// already passed config validation.
func LogSafeURL(raw string) string {
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Host == "" {
		return "(unparseable)"
	}

	return (&url.URL{Scheme: parsed.Scheme, Host: parsed.Host}).String()
}

func (p *Prover) target() *proverv2.Client {
	return &proverv2.Client{ChainId: p.chainID, ClientId: p.clientID}
}

func (p *Prover) LatestProvableHeight(ctx context.Context) (uint64, time.Time, error) {
	ctx, cancel := context.WithTimeout(ctx, requestTimeout)
	defer cancel()

	res, err := p.client.LatestProvableHeight(ctx, connect.NewRequest(&proverv2.LatestProvableHeightRequest{
		Client: p.target(),
	}))
	if err != nil {
		return 0, time.Time{}, errors.Wrap(err, "remote prover: latest provable height")
	}

	//nolint:gosec // seconds since the epoch, matching the ibc packet timestamp
	seconds := int64(res.Msg.GetTimestamp())

	p.logger.Debug("Resolved latest provable height", "height", res.Msg.GetHeight())

	return res.Msg.GetHeight(), time.Unix(seconds, 0).UTC(), nil
}

func (p *Prover) ClientUpdatePayloads(ctx context.Context, height uint64) ([][]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, requestTimeout)
	defer cancel()

	res, err := p.client.ClientUpdatePayloads(ctx, connect.NewRequest(&proverv2.ClientUpdatePayloadsRequest{
		Client: p.target(),
		Height: height,
	}))
	if err != nil {
		return nil, errors.Wrap(err, "remote prover: client update payloads")
	}

	p.logger.Debug("Fetched client update payloads", "height", height, "count", len(res.Msg.GetPayloads()))

	return res.Msg.GetPayloads(), nil
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

	ctx, cancel := context.WithTimeout(ctx, requestTimeout)
	defer cancel()

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
	if len(proofs) != len(packets) {
		return nil, errors.Errorf(
			"remote prover returned %d proofs for %d packets", len(proofs), len(packets),
		)
	}

	p.logger.Debug("Fetched packet proofs", "height", height, "kind", kind, "packets", len(packets))

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

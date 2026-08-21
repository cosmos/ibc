// SPDX-License-Identifier: Apache-2.0

// Package proverservice serves the attestation prover over ProverService, so
// the contract can be exercised without a second light-client implementation.
//
// A real custom light client implements the proto contract in whatever language
// it likes; it does not import this package.
package proverservice

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"connectrpc.com/connect"
	"github.com/pkg/errors"

	channeltypesv2 "github.com/cosmos/ibc-go/v11/modules/core/04-channel/v2/types"
	proverv2 "github.com/cosmos/ibc/link/api/v2/prover"
	"github.com/cosmos/ibc/link/internal/chains"
	"github.com/cosmos/ibc/link/internal/config"
	"github.com/cosmos/ibc/link/internal/relay/prover"
	"github.com/cosmos/ibc/link/internal/relay/prover/attestation"
	attestorservice "github.com/cosmos/ibc/link/internal/service/attestor"
	"github.com/cosmos/ibc/link/internal/service/signer"
	v2 "github.com/cosmos/ibc/link/internal/types/v2"
)

const readHeaderTimeout = 5 * time.Second

var errInternal = connect.NewError(connect.CodeInternal, errors.New("internal server error"))

// handler answers ProverService requests from a set of provers. A custom light
// client replaces it with its own implementation of the same contract.
type handler struct {
	logger *slog.Logger
	set    *prover.Set
}

var _ proverv2.ProverServiceHandler = (*handler)(nil)

// newServer serves provers over the ProverService contract.
func newServer(set *prover.Set) *http.Server {
	mux := http.NewServeMux()
	path, handler := proverv2.NewProverServiceHandler(&handler{
		logger: slog.With("module", "prover"),
		set:    set,
	})
	mux.Handle(path, handler)

	// gRPC needs HTTP/2, and the relayer dials over plain http.
	protocols := new(http.Protocols)
	protocols.SetHTTP1(true)
	protocols.SetUnencryptedHTTP2(true)

	return &http.Server{
		Handler:           mux,
		Protocols:         protocols,
		ReadHeaderTimeout: readHeaderTimeout,
	}
}

// NewAttestationServer serves an attestation prover per client end in the
// config.
func NewAttestationServer(ctx context.Context, configPath string) (*http.Server, error) {
	cfg, err := config.LoadFromFile(configPath, true, true)
	if err != nil {
		return nil, errors.Wrap(err, "load config")
	}

	clients, err := chains.NewClientSetFromConfig(cfg)
	if err != nil {
		return nil, errors.Wrap(err, "build chain clients")
	}

	signers, err := signer.NewSetFromConfig(ctx, cfg.Signers)
	if err != nil {
		return nil, errors.Wrap(err, "build signers")
	}

	local, remoteAttestors, err := attestorservice.ResolveFromConfig(ctx, cfg.Attestors, clients, signers)
	if err != nil {
		return nil, errors.Wrap(err, "resolve attestors")
	}

	attestors := make([]attestorservice.Attestor, 0, len(local)+len(remoteAttestors))
	attestors = append(attestors, local...)
	attestors = append(attestors, remoteAttestors...)

	provers := make(map[string]prover.Prover)

	for _, conn := range cfg.Relayer.Connections {
		for _, end := range []struct{ self, counterparty config.ClientEnd }{
			{conn.ClientA, conn.ClientB},
			{conn.ClientB, conn.ClientA},
		} {
			gen, resolveErr := attestation.ResolveGenerator(ctx, end.self, end.counterparty, clients, attestors)
			if resolveErr != nil {
				return nil, errors.Wrapf(resolveErr, "connection %q", conn.Alias)
			}

			provers[prover.Key(end.self.ChainID, end.self.ClientID)] = gen
		}
	}

	return newServer(prover.NewSet(provers)), nil
}

// prover resolves the client a request names; an unknown one is a caller error.
func (h *handler) prover(client *proverv2.Client) (prover.Prover, error) {
	chainID, clientID := client.GetChainId(), client.GetClientId()

	found, ok := h.set.Get(chainID, clientID)
	if !ok {
		return nil, connect.NewError(
			connect.CodeNotFound,
			errors.Errorf("no prover for client %q on chain %q", clientID, chainID),
		)
	}

	return found, nil
}

func (h *handler) LatestProvableHeight(
	ctx context.Context,
	req *connect.Request[proverv2.LatestProvableHeightRequest],
) (*connect.Response[proverv2.LatestProvableHeightResponse], error) {
	target, err := h.prover(req.Msg.GetClient())
	if err != nil {
		return nil, err
	}

	height, timestamp, err := target.LatestProvableHeight(ctx)
	if err != nil {
		h.logger.Error("LatestProvableHeight", "err", err)
		return nil, errInternal
	}

	return connect.NewResponse(&proverv2.LatestProvableHeightResponse{
		Height: height,
		//nolint:gosec // seconds since the epoch, matching the ibc packet timestamp
		Timestamp: uint64(timestamp.Unix()),
	}), nil
}

func (h *handler) StateProof(
	ctx context.Context,
	req *connect.Request[proverv2.StateProofRequest],
) (*connect.Response[proverv2.StateProofResponse], error) {
	target, err := h.prover(req.Msg.GetClient())
	if err != nil {
		return nil, err
	}

	proof, err := target.StateProof(ctx, req.Msg.GetHeight())
	if err != nil {
		h.logger.Error("StateProof", "err", err)
		return nil, errInternal
	}

	return connect.NewResponse(&proverv2.StateProofResponse{Proof: proof}), nil
}

func (h *handler) PacketProofs(
	ctx context.Context,
	req *connect.Request[proverv2.PacketProofsRequest],
) (*connect.Response[proverv2.PacketProofsResponse], error) {
	target, err := h.prover(req.Msg.GetClient())
	if err != nil {
		return nil, err
	}

	kind, err := proofKindFromProto(req.Msg.GetKind())
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}

	proofs, err := target.PacketProofs(
		ctx, req.Msg.GetHeight(), kind, packetsFromProto(req.Msg.GetPackets()),
	)
	if err != nil {
		h.logger.Error("PacketProofs", "err", err)
		return nil, errInternal
	}

	return connect.NewResponse(&proverv2.PacketProofsResponse{Proofs: proofs}), nil
}

func proofKindFromProto(kind proverv2.ProofKind) (v2.ProofKind, error) {
	switch kind {
	case proverv2.ProofKind_PROOF_KIND_PACKET_COMMITMENT:
		return v2.ProofKindPacketCommitment, nil
	case proverv2.ProofKind_PROOF_KIND_ACKNOWLEDGEMENT:
		return v2.ProofKindAcknowledgement, nil
	case proverv2.ProofKind_PROOF_KIND_RECEIPT_ABSENCE:
		return v2.ProofKindReceiptAbsence, nil
	default:
		return v2.ProofKindUnknown, errors.Errorf("unknown proof kind %v", kind)
	}
}

// packetsFromProto converts without reshaping, so a proof covers exactly the
// packet that was sent.
func packetsFromProto(packets []*proverv2.Packet) []channeltypesv2.Packet {
	if len(packets) == 0 {
		return nil
	}

	out := make([]channeltypesv2.Packet, len(packets))
	for i, packet := range packets {
		out[i] = channeltypesv2.Packet{
			Sequence:          packet.GetSequence(),
			SourceClient:      packet.GetSourceClient(),
			DestinationClient: packet.GetDestinationClient(),
			TimeoutTimestamp:  packet.GetTimeoutTimestamp(),
			Payloads:          payloadsFromProto(packet.GetPayloads()),
		}
	}

	return out
}

func payloadsFromProto(payloads []*proverv2.Payload) []channeltypesv2.Payload {
	if len(payloads) == 0 {
		return nil
	}

	out := make([]channeltypesv2.Payload, len(payloads))
	for i, payload := range payloads {
		out[i] = channeltypesv2.Payload{
			SourcePort:      payload.GetSourcePort(),
			DestinationPort: payload.GetDestinationPort(),
			Version:         payload.GetVersion(),
			Encoding:        payload.GetEncoding(),
			Value:           payload.GetValue(),
		}
	}

	return out
}

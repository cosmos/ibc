// SPDX-License-Identifier: Apache-2.0

package remote

import (
	"github.com/pkg/errors"

	channeltypesv2 "github.com/cosmos/ibc-go/v11/modules/core/04-channel/v2/types"
	proverv2 "github.com/cosmos/ibc/link/api/v2/prover"
	v2 "github.com/cosmos/ibc/link/internal/types/v2"
)

// ErrUnsupported is returned for values this build cannot represent on the
// wire, rather than silently sending a zero value.
var ErrUnsupported = errors.New("unsupported by the prover wire contract")

func proofKindToProto(kind v2.ProofKind) (proverv2.ProofKind, error) {
	switch kind {
	case v2.ProofKindPacketCommitment:
		return proverv2.ProofKind_PROOF_KIND_PACKET_COMMITMENT, nil
	case v2.ProofKindAcknowledgement:
		return proverv2.ProofKind_PROOF_KIND_ACKNOWLEDGEMENT, nil
	case v2.ProofKindReceiptAbsence:
		return proverv2.ProofKind_PROOF_KIND_RECEIPT_ABSENCE, nil
	default:
		return proverv2.ProofKind_PROOF_KIND_UNSPECIFIED, errors.Wrapf(ErrUnsupported, "proof kind %d", kind)
	}
}

// ProofKindFromProto is the inverse, for service implementations.
func ProofKindFromProto(kind proverv2.ProofKind) (v2.ProofKind, error) {
	switch kind {
	case proverv2.ProofKind_PROOF_KIND_PACKET_COMMITMENT:
		return v2.ProofKindPacketCommitment, nil
	case proverv2.ProofKind_PROOF_KIND_ACKNOWLEDGEMENT:
		return v2.ProofKindAcknowledgement, nil
	case proverv2.ProofKind_PROOF_KIND_RECEIPT_ABSENCE:
		return v2.ProofKindReceiptAbsence, nil
	default:
		return v2.ProofKindUnknown, errors.Wrapf(ErrUnsupported, "proof kind %v", kind)
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

// PacketsFromProto is the inverse, for service implementations. Conversion is
// an identity in both directions, including the empty case, so a proof is
// always generated over exactly the packet the relayer sent.
func PacketsFromProto(packets []*proverv2.Packet) []channeltypesv2.Packet {
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

// Package attestation generates recv/ack/timeout relay transactions in
// process by querying the attestor set configured for a client and
// aggregating quorum, replacing the external proof api.
package attestation

import (
	"crypto/sha256"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/pkg/errors"

	v2 "github.com/cosmos/ibc/link/internal/types/v2"
)

// Attestation type tags. AttestationLightClient.sol domain-separates the
// signed digest by claim kind: state claims (client updates) sign
// sha256(0x01 || sha256(attestationData)); packet claims (membership and
// non-membership) sign sha256(0x02 || sha256(attestationData)).
const (
	attestationTypeState  byte = 0x01
	attestationTypePacket byte = 0x02
)

// StateAttestation mirrors the Solidity IAttestationMsgs.StateAttestation
// struct: the light client's client-update claim.
type StateAttestation struct {
	Height    uint64
	Timestamp uint64
}

// PacketCompact mirrors IAttestationMsgs.PacketCompact: one packet's ICS-24
// path and the commitment or receipt value attested at that path.
type PacketCompact struct {
	Path       [32]byte
	Commitment [32]byte
}

// PacketAttestation mirrors IAttestationMsgs.PacketAttestation: the light
// client's membership or non-membership claim for a batch of packets, all at
// one height.
type PacketAttestation struct {
	Height  uint64
	Packets []PacketCompact
}

// AttestationProof mirrors IAttestationMsgs.AttestationProof: the calldata
// the light client decodes on updateClient, verifyMembership, and
// verifyNonMembership calls.
type AttestationProof struct {
	AttestationData []byte
	Signatures      [][]byte
}

var (
	packetType            abi.Type
	stateAttestationArgs  abi.Arguments
	packetAttestationArgs abi.Arguments
	attestationProofArgs  abi.Arguments
)

//nolint:gochecknoinits // one-time ABI type construction, mirrors abigen's own package-level MetaData pattern
func init() {
	payloadComponents := []abi.ArgumentMarshaling{
		{Name: "sourcePort", Type: "string"},
		{Name: "destPort", Type: "string"},
		{Name: "version", Type: "string"},
		{Name: "encoding", Type: "string"},
		{Name: "value", Type: "bytes"},
	}

	packetComponents := []abi.ArgumentMarshaling{
		{Name: "sequence", Type: "uint64"},
		{Name: "sourceClient", Type: "string"},
		{Name: "destClient", Type: "string"},
		{Name: "timeoutTimestamp", Type: "uint64"},
		{Name: "payloads", Type: "tuple[]", Components: payloadComponents},
	}

	var err error

	packetType, err = abi.NewType("tuple", "", packetComponents)
	if err != nil {
		panic(errors.Wrap(err, "constructing packet abi type"))
	}

	stateAttestationType, err := abi.NewType("tuple", "", []abi.ArgumentMarshaling{
		{Name: "height", Type: "uint64"},
		{Name: "timestamp", Type: "uint64"},
	})
	if err != nil {
		panic(errors.Wrap(err, "constructing state attestation abi type"))
	}

	packetCompactComponents := []abi.ArgumentMarshaling{
		{Name: "path", Type: "bytes32"},
		{Name: "commitment", Type: "bytes32"},
	}

	packetAttestationType, err := abi.NewType("tuple", "", []abi.ArgumentMarshaling{
		{Name: "height", Type: "uint64"},
		{Name: "packets", Type: "tuple[]", Components: packetCompactComponents},
	})
	if err != nil {
		panic(errors.Wrap(err, "constructing packet attestation abi type"))
	}

	attestationProofType, err := abi.NewType("tuple", "", []abi.ArgumentMarshaling{
		{Name: "attestationData", Type: "bytes"},
		{Name: "signatures", Type: "bytes[]"},
	})
	if err != nil {
		panic(errors.Wrap(err, "constructing attestation proof abi type"))
	}

	stateAttestationArgs = abi.Arguments{{Type: stateAttestationType}}
	packetAttestationArgs = abi.Arguments{{Type: packetAttestationType}}
	attestationProofArgs = abi.Arguments{{Type: attestationProofType}}
}

type abiPayload struct {
	SourcePort string
	DestPort   string
	Version    string
	Encoding   string
	Value      []byte
}

type abiPacket struct {
	Sequence         uint64
	SourceClient     string
	DestClient       string
	TimeoutTimestamp uint64
	Payloads         []abiPayload
}

// encodePacket abi-encodes a packet as the Solidity IICS26RouterMsgs.Packet
// tuple, the shape the attestor's PacketAttestationRequest.packets expects.
func encodePacket(packet v2.Packet) ([]byte, error) {
	payloads := make([]abiPayload, len(packet.Payloads))
	for i, p := range packet.Payloads {
		payloads[i] = abiPayload{
			SourcePort: p.SourcePort,
			DestPort:   p.DestPort,
			Version:    p.Version,
			Encoding:   p.Encoding,
			Value:      p.Value,
		}
	}

	value := abiPacket{
		Sequence:         packet.Sequence,
		SourceClient:     packet.SourceClient,
		DestClient:       packet.DestClient,
		TimeoutTimestamp: packet.TimeoutTimestamp,
		Payloads:         payloads,
	}

	encoded, err := (abi.Arguments{{Type: packetType}}).Pack(value)
	if err != nil {
		return nil, errors.Wrap(err, "encoding packet")
	}

	return encoded, nil
}

func encodeStateAttestation(s StateAttestation) ([]byte, error) {
	encoded, err := stateAttestationArgs.Pack(s)
	if err != nil {
		return nil, errors.Wrap(err, "encoding state attestation")
	}

	return encoded, nil
}

func decodeStateAttestation(data []byte) (StateAttestation, error) {
	values, err := stateAttestationArgs.Unpack(data)
	if err != nil {
		return StateAttestation{}, errors.Wrap(err, "decoding state attestation")
	}

	decoded, ok := abi.ConvertType(values[0], new(StateAttestation)).(*StateAttestation)
	if !ok {
		return StateAttestation{}, errors.New("decoding state attestation: unexpected type")
	}

	return *decoded, nil
}

func decodePacketAttestation(data []byte) (PacketAttestation, error) {
	values, err := packetAttestationArgs.Unpack(data)
	if err != nil {
		return PacketAttestation{}, errors.Wrap(err, "decoding packet attestation")
	}

	decoded, ok := abi.ConvertType(values[0], new(PacketAttestation)).(*PacketAttestation)
	if !ok {
		return PacketAttestation{}, errors.New("decoding packet attestation: unexpected type")
	}

	return *decoded, nil
}

func encodeAttestationProof(p AttestationProof) ([]byte, error) {
	encoded, err := attestationProofArgs.Pack(p)
	if err != nil {
		return nil, errors.Wrap(err, "encoding attestation proof")
	}

	return encoded, nil
}

// taggedDigest computes sha256(typeTag || sha256(attestationData)), the
// domain-separated digest AttestationLightClient.sol verifies signatures
// over. State claims use attestationTypeState; packet membership and
// non-membership claims use attestationTypePacket.
func taggedDigest(attestationData []byte, typeTag byte) [32]byte {
	inner := sha256.Sum256(attestationData)

	return sha256.Sum256(append([]byte{typeTag}, inner[:]...))
}

// recoverSigner recovers the address that produced sig (65-byte r||s||v)
// over digest. crypto.SigToPub requires v as a raw 0/1 recovery id, but
// attestors sign with Ethereum's legacy v (27/28), so it's normalized first.
func recoverSigner(digest [32]byte, sig []byte) (common.Address, error) {
	if len(sig) != 65 {
		return common.Address{}, errors.Errorf("signature must be 65 bytes, got %d", len(sig))
	}

	normalized := make([]byte, 65)
	copy(normalized, sig)

	if normalized[64] >= 27 {
		normalized[64] -= 27
	}

	pub, err := crypto.SigToPub(digest[:], normalized)
	if err != nil {
		return common.Address{}, errors.Wrap(err, "recovering signer public key")
	}

	return crypto.PubkeyToAddress(*pub), nil
}

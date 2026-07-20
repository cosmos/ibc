package attestation

import (
	"crypto/sha256"
	"testing"

	"github.com/ethereum/go-ethereum/crypto"
	"github.com/stretchr/testify/require"

	v2 "github.com/cosmos/ibc/link/internal/types/v2"
)

func TestEncodePacket(t *testing.T) {
	packet := v2.Packet{
		Sequence:         42,
		SourceClient:     "base-0",
		DestClient:       "ethereum-0",
		TimeoutTimestamp: 1234567890,
		Payloads: []v2.Payload{
			{SourcePort: "transfer", DestPort: "transfer", Version: "ics20-1", Encoding: "application/x-solidity-abi", Value: []byte{0xde, 0xad, 0xbe, 0xef}},
		},
	}

	encoded, err := encodePacket(packet)
	require.NoError(t, err)
	require.NotEmpty(t, encoded)

	// re-encoding the same packet must be deterministic
	again, err := encodePacket(packet)
	require.NoError(t, err)
	require.Equal(t, encoded, again)
}

func TestStateAttestationRoundTrip(t *testing.T) {
	original := StateAttestation{Height: 100, Timestamp: 1700000000}

	encoded, err := encodeStateAttestation(original)
	require.NoError(t, err)

	decoded, err := decodeStateAttestation(encoded)
	require.NoError(t, err)
	require.Equal(t, original, decoded)
}

func TestPacketAttestationRoundTrip(t *testing.T) {
	original := PacketAttestation{
		Height: 200,
		Packets: []PacketCompact{
			{Path: [32]byte{1, 2, 3}, Commitment: [32]byte{4, 5, 6}},
			{Path: [32]byte{7, 8, 9}, Commitment: [32]byte{}},
		},
	}

	encodedBytes, err := packetAttestationArgs.Pack(original)
	require.NoError(t, err)

	decoded, err := decodePacketAttestation(encodedBytes)
	require.NoError(t, err)
	require.Equal(t, original, decoded)
}

func TestAttestationProofEncode(t *testing.T) {
	proof := AttestationProof{
		AttestationData: []byte{0x01, 0x02, 0x03},
		Signatures:      [][]byte{make([]byte, 65), make([]byte, 65)},
	}

	encoded, err := encodeAttestationProof(proof)
	require.NoError(t, err)
	require.NotEmpty(t, encoded)
}

func TestTaggedDigestMatchesFormula(t *testing.T) {
	data := []byte("some attestation data")

	for _, tag := range []byte{attestationTypeState, attestationTypePacket} {
		got := taggedDigest(data, tag)

		inner := sha256.Sum256(data)
		want := sha256.Sum256(append([]byte{tag}, inner[:]...))

		require.Equal(t, want, got, "tag %x", tag)
	}
}

func TestRecoverSignerRoundTrip(t *testing.T) {
	key, err := crypto.GenerateKey()
	require.NoError(t, err)

	expected := crypto.PubkeyToAddress(key.PublicKey)

	digest := taggedDigest([]byte("attested data"), attestationTypePacket)

	sig, err := crypto.Sign(digest[:], key)
	require.NoError(t, err)
	require.Len(t, sig, 65)

	recovered, err := recoverSigner(digest, sig)
	require.NoError(t, err)
	require.Equal(t, expected, recovered)
}

func TestRecoverSignerRejectsWrongLength(t *testing.T) {
	_, err := recoverSigner([32]byte{}, []byte{0x01, 0x02})
	require.Error(t, err)
}

// TestRecoverSignerAcceptsLegacyVByte verifies recoverSigner works against
// signatures using Ethereum's legacy v (27/28), the convention the real
// attestor signs with, not just the raw 0/1 recovery id crypto.Sign produces.
func TestRecoverSignerAcceptsLegacyVByte(t *testing.T) {
	key, err := crypto.GenerateKey()
	require.NoError(t, err)

	expected := crypto.PubkeyToAddress(key.PublicKey)

	digest := taggedDigest([]byte("attested data"), attestationTypePacket)

	sig, err := crypto.Sign(digest[:], key)
	require.NoError(t, err)

	legacySig := make([]byte, 65)
	copy(legacySig, sig)
	legacySig[64] += 27

	recovered, err := recoverSigner(digest, legacySig)
	require.NoError(t, err)
	require.Equal(t, expected, recovered)
}

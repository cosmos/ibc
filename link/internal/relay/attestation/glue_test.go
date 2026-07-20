package attestation

import (
	"context"
	"testing"

	"connectrpc.com/connect"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/cosmos/ibc/link/internal/chains"

	ibcattestor "github.com/cosmos/ibc/link/internal/types/ibcattestor"
	proto "github.com/cosmos/ibc/link/internal/types/proofapi"
	v2 "github.com/cosmos/ibc/link/internal/types/v2"
)

// fullySignedAttestor builds a namedAttestorClient that answers LatestHeight,
// StateAttestation, and PacketAttestation with data signed by a freshly
// generated key over the correct domain-tagged digest for each.
func fullySignedAttestor(t *testing.T, name string, height uint64, stateData, packetData []byte) namedAttestorClient {
	t.Helper()

	key, err := crypto.GenerateKey()
	require.NoError(t, err)

	stateDigest := taggedDigest(stateData, attestationTypeState)
	stateSig, err := crypto.Sign(stateDigest[:], key)
	require.NoError(t, err)

	packetDigest := taggedDigest(packetData, attestationTypePacket)
	packetSig, err := crypto.Sign(packetDigest[:], key)
	require.NoError(t, err)

	client := ibcattestor.NewMockAttestationServiceClient(t)
	client.EXPECT().LatestHeight(mock.Anything, mock.Anything).Return(
		connect.NewResponse(&ibcattestor.LatestHeightResponse{Height: height}), nil,
	)
	client.EXPECT().StateAttestation(mock.Anything, mock.Anything).Return(
		connect.NewResponse(&ibcattestor.StateAttestationResponse{
			Attestation: &ibcattestor.Attestation{Height: height, AttestedData: stateData, Signature: stateSig},
		}), nil,
	)
	client.EXPECT().PacketAttestation(mock.Anything, mock.Anything).Return(
		connect.NewResponse(&ibcattestor.PacketAttestationResponse{
			Attestation: &ibcattestor.Attestation{Height: height, AttestedData: packetData, Signature: packetSig},
		}), nil,
	)

	return namedAttestorClient{Name: name, Client: client}
}

func testGlue(t *testing.T, chainClients map[string]chains.Client, attestors []namedAttestorClient) *Glue {
	t.Helper()

	return &Glue{
		clients:      chains.NewClientSet(chainClients),
		chainRouters: map[string]string{"src-chain": "0xSrcRouter", "dst-chain": "0xDstRouter"},
		byClient: map[string]resolvedClient{
			clientKey("dst-chain", "dst-0"): {threshold: 2, attestors: attestors},
			clientKey("src-chain", "src-0"): {threshold: 2, attestors: attestors},
		},
	}
}

func testTransferPacket() v2.Packet {
	return v2.Packet{
		Sequence:         5,
		SourceClient:     "src-0",
		DestClient:       "dst-0",
		TimeoutTimestamp: 1234567890,
		Payloads: []v2.Payload{
			{SourcePort: "transfer", DestPort: "transfer", Version: "ics20-1", Encoding: "application/x-solidity-abi", Value: []byte{0xaa, 0xbb}},
		},
	}
}

func testPacketAttestationData(t *testing.T, height uint64, packet v2.Packet) []byte {
	t.Helper()

	data, err := encodePacketAttestation(PacketAttestation{
		Height:  height,
		Packets: []PacketCompact{{Path: [32]byte{1}, Commitment: [32]byte{2}}},
	})
	require.NoError(t, err)
	_ = packet // the packet's identity isn't re-derived client-side; only the count and height are checked

	return data
}

func encodePacketAttestation(p PacketAttestation) ([]byte, error) {
	return packetAttestationArgs.Pack(p)
}

func TestGlueRelayRecv(t *testing.T) {
	ctx := context.Background()
	packet := testTransferPacket()
	txID := []byte{0xde, 0xad, 0xbe, 0xef}

	const height = uint64(42)

	stateData, err := encodeStateAttestation(StateAttestation{Height: height, Timestamp: 1000})
	require.NoError(t, err)

	packetData := testPacketAttestationData(t, height, packet)

	attestors := []namedAttestorClient{
		fullySignedAttestor(t, "a1", height, stateData, packetData),
		fullySignedAttestor(t, "a2", height, stateData, packetData),
	}

	srcChain := chains.NewMockClient(t)
	srcChain.EXPECT().TxPacketEvents(mock.Anything, txID).Return([]v2.PacketEvent{
		{Height: 10, Kind: v2.KindSendPacket, Packet: packet},
	}, nil)

	glue := testGlue(t, map[string]chains.Client{"src-chain": srcChain}, attestors)

	resp, err := glue.RelayByTx(ctx, connect.NewRequest(&proto.RelayByTxRequest{
		SrcChain:           "src-chain",
		DstChain:           "dst-chain",
		SourceTxIds:        [][]byte{txID},
		SrcClientId:        "src-0",
		DstClientId:        "dst-0",
		SrcPacketSequences: []uint64{5},
	}))
	require.NoError(t, err)
	require.Equal(t, "0xDstRouter", resp.Msg.GetAddress())
	require.NotEmpty(t, resp.Msg.GetTx())
}

func TestGlueRelayAck(t *testing.T) {
	ctx := context.Background()
	packet := testTransferPacket()
	txID := []byte{0x01, 0x02, 0x03, 0x04}
	ack := []byte{0xac, 0x01}

	const height = uint64(7)

	stateData, err := encodeStateAttestation(StateAttestation{Height: height, Timestamp: 500})
	require.NoError(t, err)

	packetData := testPacketAttestationData(t, height, packet)

	attestors := []namedAttestorClient{
		fullySignedAttestor(t, "a1", height, stateData, packetData),
		fullySignedAttestor(t, "a2", height, stateData, packetData),
	}

	dstChain := chains.NewMockClient(t)
	dstChain.EXPECT().TxPacketEvents(mock.Anything, txID).Return([]v2.PacketEvent{
		{Height: 10, Kind: v2.KindWriteAck, Packet: packet, Acks: [][]byte{ack}},
	}, nil)

	glue := testGlue(t, map[string]chains.Client{"dst-chain": dstChain}, attestors)

	resp, err := glue.RelayByTx(ctx, connect.NewRequest(&proto.RelayByTxRequest{
		SrcChain:           "dst-chain",
		DstChain:           "src-chain",
		SourceTxIds:        [][]byte{txID},
		SrcClientId:        "dst-0",
		DstClientId:        "src-0",
		DstPacketSequences: []uint64{5},
	}))
	require.NoError(t, err)
	require.Equal(t, "0xSrcRouter", resp.Msg.GetAddress())
	require.NotEmpty(t, resp.Msg.GetTx())
}

func TestGlueRelayTimeout(t *testing.T) {
	ctx := context.Background()
	packet := testTransferPacket()
	txID := []byte{0x0a, 0x0b}

	const height = uint64(3)

	stateData, err := encodeStateAttestation(StateAttestation{Height: height, Timestamp: 200})
	require.NoError(t, err)

	packetData := testPacketAttestationData(t, height, packet)

	attestors := []namedAttestorClient{
		fullySignedAttestor(t, "a1", height, stateData, packetData),
		fullySignedAttestor(t, "a2", height, stateData, packetData),
	}

	srcChain := chains.NewMockClient(t)
	srcChain.EXPECT().TxPacketEvents(mock.Anything, txID).Return([]v2.PacketEvent{
		{Height: 1, Kind: v2.KindSendPacket, Packet: packet},
	}, nil)

	glue := testGlue(t, map[string]chains.Client{"src-chain": srcChain}, attestors)

	resp, err := glue.RelayByTx(ctx, connect.NewRequest(&proto.RelayByTxRequest{
		SrcChain:           "dst-chain",
		DstChain:           "src-chain",
		TimeoutTxIds:       [][]byte{txID},
		SrcClientId:        "dst-0",
		DstClientId:        "src-0",
		DstPacketSequences: []uint64{5},
	}))
	require.NoError(t, err)
	require.Equal(t, "0xSrcRouter", resp.Msg.GetAddress())
	require.NotEmpty(t, resp.Msg.GetTx())
}

func TestGlueUnimplementedMethods(t *testing.T) {
	ctx := context.Background()
	glue := testGlue(t, map[string]chains.Client{}, nil)

	_, err := glue.CreateClient(ctx, connect.NewRequest(&proto.CreateClientRequest{}))
	require.Error(t, err)

	_, err = glue.UpdateClient(ctx, connect.NewRequest(&proto.UpdateClientRequest{}))
	require.Error(t, err)

	_, err = glue.Info(ctx, connect.NewRequest(&proto.InfoRequest{}))
	require.Error(t, err)
}

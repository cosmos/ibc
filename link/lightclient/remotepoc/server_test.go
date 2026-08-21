// SPDX-License-Identifier: Apache-2.0

package remotepoc

import (
	"context"
	"net/http/httptest"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/require"

	channeltypesv2 "github.com/cosmos/ibc-go/v11/modules/core/04-channel/v2/types"
	"github.com/cosmos/ibc/link/internal/relay/prover"
	"github.com/cosmos/ibc/link/internal/relay/prover/remote"
	v2 "github.com/cosmos/ibc/link/internal/types/v2"
)

// stubProver records what it was asked so the round trip can be asserted from
// the far side of the wire.
type stubProver struct {
	height     uint64
	timestamp  time.Time
	stateProof []byte
	proofs     [][]byte

	gotHeight  uint64
	gotKind    v2.ProofKind
	gotPackets []channeltypesv2.Packet
}

func (s *stubProver) LatestProvableHeight(context.Context) (uint64, time.Time, error) {
	return s.height, s.timestamp, nil
}

func (s *stubProver) StateProof(_ context.Context, height uint64) ([]byte, error) {
	s.gotHeight = height
	return s.stateProof, nil
}

func (s *stubProver) PacketProofs(
	_ context.Context,
	height uint64,
	kind v2.ProofKind,
	packets []channeltypesv2.Packet,
) ([][]byte, error) {
	s.gotHeight, s.gotKind, s.gotPackets = height, kind, packets
	return s.proofs, nil
}

func proverPair(t *testing.T, set *prover.Set, chainID, clientID string) *remote.Prover {
	t.Helper()

	server := httptest.NewUnstartedServer(NewServer(set).Handler)
	server.EnableHTTP2 = true
	server.StartTLS()
	t.Cleanup(server.Close)

	return remote.New(server.Client(), server.URL, chainID, clientID)
}

// The relayer reaches a custom light client only through this contract, so the
// round trip must preserve every value the internal interface carries.
func TestProverServiceRoundTrip(t *testing.T) {
	ctx := context.Background()
	stub := &stubProver{
		height:     4321,
		timestamp:  time.Unix(1700000000, 0).UTC(),
		stateProof: []byte("state-proof"),
		proofs:     [][]byte{[]byte("proof-a"), []byte("proof-b")},
	}
	set := prover.NewSet(map[string]prover.Prover{prover.Key("chain-a", "client-0"): stub})
	client := proverPair(t, set, "chain-a", "client-0")

	t.Run("latest provable height", func(t *testing.T) {
		height, timestamp, err := client.LatestProvableHeight(ctx)
		require.NoError(t, err)
		require.Equal(t, uint64(4321), height)
		require.Equal(t, stub.timestamp, timestamp)
	})

	t.Run("state proof", func(t *testing.T) {
		proof, err := client.StateProof(ctx, 99)
		require.NoError(t, err)
		require.Equal(t, []byte("state-proof"), proof)
		require.Equal(t, uint64(99), stub.gotHeight)
	})

	t.Run("packet proofs", func(t *testing.T) {
		packets := []channeltypesv2.Packet{
			{
				Sequence:          7,
				SourceClient:      "client-0",
				DestinationClient: "client-1",
				TimeoutTimestamp:  1800000000,
				Payloads: []channeltypesv2.Payload{{
					SourcePort:      "transfer",
					DestinationPort: "transfer",
					Version:         "ics20-2",
					Encoding:        "application/x-solidity-abi",
					Value:           []byte("payload"),
				}},
			},
			{Sequence: 8, SourceClient: "client-0", DestinationClient: "client-1"},
		}

		proofs, err := client.PacketProofs(ctx, 4321, v2.ProofKindReceiptAbsence, packets)
		require.NoError(t, err)
		require.Equal(t, [][]byte{[]byte("proof-a"), []byte("proof-b")}, proofs)

		// The payload must survive the wire, or the proof is generated over
		// something other than the packet that was sent.
		require.Equal(t, packets, stub.gotPackets)
		require.Equal(t, v2.ProofKindReceiptAbsence, stub.gotKind)
		require.Equal(t, uint64(4321), stub.gotHeight)
	})

	t.Run("the client is named on every request", func(t *testing.T) {
		// Resolution only succeeds when the request carries the right client,
		// so every call above proves it.
		_, err := proverPair(t, set, "chain-a", "wrong-client").StateProof(ctx, 1)
		require.Error(t, err)
	})
}

func TestProverServiceUnknownClient(t *testing.T) {
	client := proverPair(t, prover.NewSet(nil), "chain-z", "client-9")

	_, _, err := client.LatestProvableHeight(context.Background())
	require.Error(t, err)
	require.Equal(t, connect.CodeNotFound, connect.CodeOf(errors.Cause(err)))
}

// A short response would attach one packet's proof to another packet.
func TestProverServiceRejectsMismatchedProofCount(t *testing.T) {
	stub := &stubProver{proofs: [][]byte{[]byte("only-one")}}
	set := prover.NewSet(map[string]prover.Prover{prover.Key("chain-a", "client-0"): stub})
	client := proverPair(t, set, "chain-a", "client-0")

	_, err := client.PacketProofs(context.Background(), 1, v2.ProofKindPacketCommitment,
		[]channeltypesv2.Packet{{Sequence: 1}, {Sequence: 2}})
	require.ErrorContains(t, err, "returned 1 proofs for 2 packets")
}

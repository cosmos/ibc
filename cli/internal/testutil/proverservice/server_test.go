// SPDX-License-Identifier: Apache-2.0

package proverservice

import (
	"context"
	"log/slog"
	"net/http/httptest"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/require"

	channeltypesv2 "github.com/cosmos/ibc-go/v11/modules/core/04-channel/v2/types"
	"github.com/cosmos/ibc/cli/internal/relay/prover"
	"github.com/cosmos/ibc/cli/internal/relay/prover/remote"
	v2 "github.com/cosmos/ibc/cli/internal/types/v2"
)

// stubProver records what it was asked, so the far side of the wire can assert it.
type stubProver struct {
	height    uint64
	timestamp time.Time
	update    []byte
	advance   []byte
	proofs    [][]byte

	gotHeight  uint64
	gotKind    v2.ProofKind
	gotPackets []channeltypesv2.Packet
}

func (s *stubProver) LatestProvableHeight(context.Context) (uint64, time.Time, error) {
	return s.height, s.timestamp, nil
}

func (s *stubProver) Prepare(
	_ context.Context,
	height uint64,
	kind v2.ProofKind,
	packets []channeltypesv2.Packet,
) (*v2.Preparation, error) {
	s.gotHeight, s.gotKind, s.gotPackets = height, kind, packets
	if len(s.advance) > 0 {
		return &v2.Preparation{Advance: s.advance}, nil
	}
	return &v2.Preparation{Ready: &v2.BatchProofs{Update: s.update, PacketProofs: s.proofs}}, nil
}

func newClient(t *testing.T, set *prover.Set, chainID, clientID string) *remote.Prover {
	t.Helper()

	server := httptest.NewUnstartedServer(newServer(set).Handler)
	server.EnableHTTP2 = true
	server.StartTLS()
	t.Cleanup(server.Close)

	return remote.New(server.Client(), server.URL, chainID, clientID, slog.Default())
}

// A custom light client is reached only through this contract, so the round trip
// must preserve every value the internal interface carries.
func TestProverServiceRoundTrip(t *testing.T) {
	ctx := context.Background()
	stub := &stubProver{
		height:    4321,
		timestamp: time.Unix(1700000000, 0).UTC(),
		update:    []byte("state-proof"),
		proofs:    [][]byte{[]byte("proof-a"), []byte("proof-b")},
	}
	set := prover.NewSet(map[string]prover.Prover{prover.Key("chain-a", "client-0"): stub})
	client := newClient(t, set, "chain-a", "client-0")

	t.Run("latest provable height", func(t *testing.T) {
		height, timestamp, err := client.LatestProvableHeight(ctx)
		require.NoError(t, err)
		require.Equal(t, uint64(4321), height)
		require.Equal(t, stub.timestamp, timestamp)
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

		proofs, err := client.Prepare(ctx, 4321, v2.ProofKindReceiptAbsence, packets)
		require.NoError(t, err)
		require.Equal(t, [][]byte{[]byte("proof-a"), []byte("proof-b")}, proofs.Ready.PacketProofs)

		// A dropped field proves a different packet than the one sent.
		require.Equal(t, []byte("state-proof"), proofs.Ready.Update)
		require.Equal(t, packets, stub.gotPackets)
		require.Equal(t, v2.ProofKindReceiptAbsence, stub.gotKind)
		require.Equal(t, uint64(4321), stub.gotHeight)
	})
}

func TestProverServiceUnknownClient(t *testing.T) {
	client := newClient(t, prover.NewSet(nil), "chain-z", "client-9")

	_, _, err := client.LatestProvableHeight(context.Background())
	require.Error(t, err)
	require.Equal(t, connect.CodeNotFound, connect.CodeOf(errors.Cause(err)))
}

// A short response would attach one packet's proof to another packet.
func TestProverServiceRejectsMismatchedProofCount(t *testing.T) {
	stub := &stubProver{proofs: [][]byte{[]byte("only-one")}}
	set := prover.NewSet(map[string]prover.Prover{prover.Key("chain-a", "client-0"): stub})
	client := newClient(t, set, "chain-a", "client-0")

	_, err := client.Prepare(context.Background(), 1, v2.ProofKindPacketCommitment,
		[]channeltypesv2.Packet{{Sequence: 1}, {Sequence: 2}})
	require.Equal(t, connect.CodeInternal, connect.CodeOf(errors.Cause(err)))
}

func TestProverServiceAdvance(t *testing.T) {
	stub := &stubProver{advance: []byte("checkpoint")}
	client := newClient(
		t,
		prover.NewSet(map[string]prover.Prover{prover.Key("chain-a", "client-0"): stub}),
		"chain-a",
		"client-0",
	)
	result, err := client.Prepare(t.Context(), 99, v2.ProofKindPacketCommitment, nil)
	require.NoError(t, err)
	require.Nil(t, result.Ready)
	require.Equal(t, stub.advance, result.Advance)
	require.Equal(t, uint64(99), stub.gotHeight)
}

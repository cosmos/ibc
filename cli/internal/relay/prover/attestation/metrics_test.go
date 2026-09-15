// SPDX-License-Identifier: Apache-2.0

package attestation

import (
	"context"
	"errors"
	"log/slog"
	"testing"

	"connectrpc.com/connect"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/attribute"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"

	channeltypesv2 "github.com/cosmos/ibc-go/v11/modules/core/04-channel/v2/types"
	hostv2 "github.com/cosmos/ibc-go/v11/modules/core/24-host/v2"
	attestorevm "github.com/cosmos/ibc/cli/attestor/evm"
	"github.com/cosmos/ibc/cli/internal/service/attestor"
	"github.com/cosmos/ibc/cli/internal/tests/mocks"
	v2 "github.com/cosmos/ibc/cli/internal/types/v2"
)

func testMetrics(t *testing.T) *sdkmetric.ManualReader {
	t.Helper()
	reader := sdkmetric.NewManualReader()
	provider := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	instruments, err := newInstrumentation(provider.Meter("test"))
	require.NoError(t, err)
	previous := metrics
	metrics = instruments
	t.Cleanup(func() {
		metrics = previous
		require.NoError(t, provider.Shutdown(context.Background()))
	})
	return reader
}

func requireOutcomes(
	t *testing.T,
	reader *sdkmetric.ManualReader,
	kind, roundReason string,
	responses map[string]int64,
) {
	t.Helper()
	var collected metricdata.ResourceMetrics
	require.NoError(t, reader.Collect(context.Background(), &collected))
	want := map[string]map[string]int64{
		"aggregation_rounds_total": {"/" + roundReason: 1},
	}
	if len(responses) > 0 {
		want["aggregation_responses_total"] = responses
	}
	actual := make(map[string]map[string]int64)
	for _, scope := range collected.ScopeMetrics {
		for _, m := range scope.Metrics {
			sum, ok := m.Data.(metricdata.Sum[int64])
			require.True(t, ok)
			require.True(t, sum.IsMonotonic)
			actual[m.Name] = make(map[string]int64)
			for _, point := range sum.DataPoints {
				reason, ok := point.Attributes.Value("failure_reason")
				require.True(t, ok)
				result := "error"
				if reason.AsString() == "none" {
					result = "ok"
				}
				attrs := []attribute.KeyValue{
					attribute.String("chain_id", "client-chain"),
					attribute.String("client_id", "client-0"),
					attribute.String("proof_kind", kind),
					attribute.String("result", result),
					attribute.String("failure_reason", reason.AsString()),
				}
				name := ""
				if m.Name == "aggregation_responses_total" {
					value, exists := point.Attributes.Value("attestor")
					require.True(t, exists)
					name = value.AsString()
					attrs = append(attrs, attribute.String("attestor", name))
				}
				wantAttrs := attribute.NewSet(attrs...)
				require.Equal(t, wantAttrs.ToSlice(), point.Attributes.ToSlice())
				actual[m.Name][name+"/"+reason.AsString()] += point.Value
			}
		}
	}
	require.Equal(t, want, actual)
}

func TestStateAggregationMetrics(t *testing.T) {
	for _, scenario := range []string{"success", "spare", "insufficient", "duplicate", "mismatch", "signature", "rpc", "timeout", "canceled", "lookup"} {
		t.Run(scenario, func(t *testing.T) {
			reader := testMetrics(t)
			ctx := context.Background()
			chain := mocks.NewMockClient(t)
			var lookupErr error
			if scenario == "lookup" {
				lookupErr = errors.New("node unavailable")
			}
			chain.EXPECT().
				GetBlockHeader(mock.Anything, uint64(10)).
				Return(v2.BlockHeader{Timestamp: someBlockTime}, lookupErr).
				Once()
			data, err := attestorevm.EncodeStateAttestation(10, uint64(someBlockTime.Unix()))
			require.NoError(t, err)
			var attestors []attestor.Attestor
			responses := map[string]int64{}
			if scenario != "lookup" {
				attestors = append(attestors, signedAttestor(t, "valid", data))
				responses["valid/none"] = 1
			}
			threshold := 1
			round := "none"
			switch scenario {
			case "spare":
				attestors = append(attestors, signedAttestor(t, "spare", data))
				responses["spare/none"] = 1
			case "insufficient":
				threshold, round = 2, "quorum_not_met"
			case "duplicate":
				threshold, round = 2, "quorum_not_met"
				attestors = append(attestors, attestors[0])
				responses["valid/duplicate_signer"] = 1
			case "mismatch":
				attestors = append(attestors, signedAttestor(t, "wrong", []byte("different claim")))
				responses["wrong/claim_mismatch"] = 1
			case "signature", "rpc", "timeout", "canceled":
				bad := attestor.NewMockAttestor(t)
				bad.EXPECT().Name().Return("bad").Maybe()
				response := attestor.Attestation{AttestedData: data, Signature: []byte("bad signature")}
				var rpcErr error
				reason := "invalid_signature"
				switch scenario {
				case "rpc":
					rpcErr, reason = errors.New("transport failed"), "rpc_error"
				case "timeout":
					rpcErr, reason = connect.NewError(connect.CodeDeadlineExceeded, errors.New("deadline")), "timeout"
				case "canceled":
					var cancel context.CancelFunc
					ctx, cancel = context.WithCancel(ctx)
					cancel()
					rpcErr, reason = context.Canceled, "canceled"
					threshold, round = 2, "canceled"
				}
				bad.EXPECT().StateAttestation(mock.Anything, uint64(10)).Return(response, rpcErr).Once()
				attestors = append(attestors, bad)
				responses["bad/"+reason] = 1
			case "lookup":
				round = "expected_claim_lookup"
			}
			gen := New(attestors, threshold, chain, slog.Default())
			gen.chainID, gen.clientID = "client-chain", "client-0"
			_, err = gen.StateProof(ctx, 10)
			require.Equal(t, round == "none", err == nil)
			requireOutcomes(t, reader, "state", round, responses)
		})
	}
}

func TestPacketAggregationMetrics(t *testing.T) {
	for _, kind := range []v2.ProofKind{v2.ProofKindPacketCommitment, v2.ProofKindAcknowledgement, v2.ProofKindReceiptAbsence, v2.ProofKindUnknown} {
		t.Run(metricProofKind(kind), func(t *testing.T) {
			reader := testMetrics(t)
			packet := channeltypesv2.Packet{
				Sequence:          1,
				SourceClient:      "src",
				DestinationClient: "dst",
				TimeoutTimestamp:  1000,
			}
			chain := mocks.NewMockClient(t)
			var acknowledgements []channeltypesv2.Acknowledgement
			compact := attestorevm.PacketCompact{}
			switch kind {
			case v2.ProofKindPacketCommitment:
				compact.Path = crypto.Keccak256Hash(hostv2.PacketCommitmentKey(packet.SourceClient, packet.Sequence))
				compact.Commitment = [32]byte(channeltypesv2.CommitPacket(packet))
			case v2.ProofKindAcknowledgement:
				compact.Path = crypto.Keccak256Hash(
					hostv2.PacketAcknowledgementKey(packet.DestinationClient, packet.Sequence),
				)
				ack := channeltypesv2.NewAcknowledgement([]byte("ack"))
				acknowledgements = []channeltypesv2.Acknowledgement{ack}
				compact.Commitment = [32]byte(channeltypesv2.CommitAcknowledgement(ack))
			case v2.ProofKindReceiptAbsence:
				compact.Path = crypto.Keccak256Hash(hostv2.PacketReceiptKey(packet.DestinationClient, packet.Sequence))
			}
			var attestors []attestor.Attestor
			responses := map[string]int64{}
			round := "internal"
			if kind != v2.ProofKindUnknown {
				attestors = []attestor.Attestor{
					signedPacketAttestor(t, "valid", 20, []attestorevm.PacketCompact{compact}),
				}
				responses["valid/none"] = 1
				round = "none"
			}
			gen := New(attestors, 1, chain, slog.Default())
			gen.chainID, gen.clientID = "client-chain", "client-0"
			_, err := gen.PacketProofs(
				context.Background(),
				20,
				kind,
				[]channeltypesv2.Packet{packet},
				acknowledgements,
			)
			require.Equal(t, round == "none", err == nil)
			requireOutcomes(t, reader, metricProofKind(kind), round, responses)
		})
	}
}

func TestAggregationMetricsLookupAndHeightDiscovery(t *testing.T) {
	t.Run("missingAcknowledgementEmitsNoResponses", func(t *testing.T) {
		reader := testMetrics(t)
		chain := mocks.NewMockClient(t)
		packet := channeltypesv2.Packet{Sequence: 1, DestinationClient: "dst"}
		gen := New(nil, 1, chain, slog.Default())
		gen.chainID, gen.clientID = "client-chain", "client-0"
		_, err := gen.PacketProofs(
			context.Background(),
			20,
			v2.ProofKindAcknowledgement,
			[]channeltypesv2.Packet{packet},
			nil,
		)
		require.ErrorContains(t, err, "acknowledgement count must match packet count")
		requireOutcomes(t, reader, "acknowledgement", "internal", nil)
	})

	t.Run("heightDiscoveryEmitsNeitherCounter", func(t *testing.T) {
		reader := testMetrics(t)
		chain := mocks.NewMockClient(t)
		chain.EXPECT().
			GetBlockHeader(mock.Anything, uint64(10)).
			Return(v2.BlockHeader{Timestamp: someBlockTime}, nil).
			Once()
		gen := New([]attestor.Attestor{heightAttestor(t, "a1", 10)}, 1, chain, slog.Default())
		_, _, err := gen.LatestProvableHeight(context.Background())
		require.NoError(t, err)
		var collected metricdata.ResourceMetrics
		require.NoError(t, reader.Collect(context.Background(), &collected))
		require.Empty(t, collected.ScopeMetrics)
	})
}

func TestResponseErrorReasons(t *testing.T) {
	for _, tt := range []struct {
		err    error
		reason string
	}{
		{context.Canceled, "canceled"},
		{context.DeadlineExceeded, "timeout"},
		{connect.NewError(connect.CodeCanceled, errors.New("remote canceled")), "canceled"},
		{connect.NewError(connect.CodeDeadlineExceeded, errors.New("remote timeout")), "timeout"},
		{connect.NewError(connect.CodeUnavailable, errors.New("unavailable")), "rpc_error"},
	} {
		require.Equal(t, tt.reason, responseErrorReason(tt.err))
	}
}

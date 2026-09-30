// SPDX-License-Identifier: Apache-2.0

package attestation

import (
	"context"
	"errors"
	"log/slog"
	"testing"

	"github.com/ethereum/go-ethereum/crypto"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/attribute"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"

	attestorevm "github.com/cosmos/ibc/cli/attestor/evm"
	"github.com/cosmos/ibc/cli/internal/service/attestor"
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

func TestAttestationDataMatches(t *testing.T) {
	ctx := context.Background()
	expected := []byte("expected claim")
	key, err := crypto.GenerateKey()
	require.NoError(t, err)
	digest := attestorevm.Digest(attestorevm.TagStateAttestation, expected)
	signature, err := crypto.Sign(digest[:], key)
	require.NoError(t, err)

	for _, tt := range []struct {
		name               string
		data, signature    []byte
		queryErr           error
		matches, wantError bool
	}{
		{name: "match", data: expected, signature: signature, matches: true},
		{name: "mismatch", data: []byte("other claim"), signature: signature, wantError: true},
		{name: "matchBeforeSignatureValidation", data: expected, matches: true, wantError: true},
		{name: "rpcFailure", queryErr: errors.New("unavailable"), wantError: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			reader := testMetrics(t)
			a := attestor.NewMockAttestor(t)
			a.EXPECT().Name().Return("attestor-0")
			gen := New("client-chain", "client-0", nil, 1, nil, slog.Default())
			response := gen.queryOne(ctx, a, attestorevm.TagStateAttestation, expected,
				func(context.Context, attestor.Attestor) (attestor.Attestation, error) {
					return attestor.Attestation{AttestedData: tt.data, Signature: tt.signature}, tt.queryErr
				})
			require.Equal(t, tt.wantError, response.err != nil)

			var collected metricdata.ResourceMetrics
			require.NoError(t, reader.Collect(ctx, &collected))
			if tt.queryErr != nil {
				require.Empty(t, collected.ScopeMetrics)
				return
			}
			require.Len(t, collected.ScopeMetrics, 1)
			require.Len(t, collected.ScopeMetrics[0].Metrics, 1)
			m := collected.ScopeMetrics[0].Metrics[0]
			require.Equal(t, "attestation_data_matches_total", m.Name)
			sum, ok := m.Data.(metricdata.Sum[int64])
			require.True(t, ok)
			require.True(t, sum.IsMonotonic)
			require.Len(t, sum.DataPoints, 1)
			require.Equal(t, int64(1), sum.DataPoints[0].Value)
			require.Equal(t, attribute.NewSet(
				attribute.String("chain_id", "client-chain"),
				attribute.String("client_id", "client-0"),
				attribute.String("attestor", "attestor-0"),
				attribute.Bool("result", tt.matches),
			), sum.DataPoints[0].Attributes)
		})
	}
}

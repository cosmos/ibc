// SPDX-License-Identifier: Apache-2.0

package otel

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/metric"
)

func TestRegisterMetrics(t *testing.T) {
	type instrumentation struct {
		Requests metric.Int64Counter
	}

	expected := &instrumentation{}
	actual := RegisterMetrics("test", func(meter metric.Meter) (*instrumentation, error) {
		counter, err := meter.Int64Counter("test_requests_total")
		require.NoError(t, err)
		expected.Requests = counter

		return expected, nil
	})

	require.Same(t, expected, actual)
	require.NotNil(t, actual.Requests)
	require.NotPanics(t, func() {
		actual.Requests.Add(context.Background(), 1)
	})
}

func TestRegisterMetricsPanicsOnConstructionFailure(t *testing.T) {
	type instrumentation struct{}

	require.PanicsWithError(t,
		"failed to construct metrics for test: construction failed",
		func() {
			RegisterMetrics("test", func(metric.Meter) (*instrumentation, error) {
				return nil, errors.New("construction failed")
			})
		},
	)
}

func TestRegisterMetricsPanicsOnNilInstrumentation(t *testing.T) {
	type instrumentation struct{}

	require.PanicsWithError(t,
		"failed to construct metrics for test: constructor returned nil",
		func() {
			RegisterMetrics("test", func(metric.Meter) (*instrumentation, error) {
				return nil, nil
			})
		},
	)
}

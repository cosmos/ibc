// SPDX-License-Identifier: Apache-2.0

package processors

import (
	"bytes"
	"log/slog"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	v2 "github.com/cosmos/ibc/cli/internal/types/v2"
)

func TestValidateRelaySimulation(t *testing.T) {
	updateSucceeded := v2.OperationResult{Type: v2.OperationTypeUpdate, Success: true}
	updateFailed := v2.OperationResult{Type: v2.OperationTypeUpdate, FailureReason: "update reverted"}
	packetSucceeded := v2.OperationResult{Type: v2.OperationTypePacket, Success: true}
	packetFailed := v2.OperationResult{Type: v2.OperationTypePacket, FailureReason: "packet reverted"}
	for _, tc := range []struct {
		name       string
		operations []v2.OperationResult
		wantErr    bool
	}{
		{name: "partial packet failure", operations: []v2.OperationResult{updateSucceeded, packetFailed, packetSucceeded}},
		{name: "failed update with packet success", operations: []v2.OperationResult{updateFailed, packetSucceeded}},
		{name: "updates alone cannot qualify packets", operations: []v2.OperationResult{updateSucceeded, packetFailed}, wantErr: true},
		{name: "update after failed packet", operations: []v2.OperationResult{packetFailed, updateSucceeded}, wantErr: true},
		{name: "all failed", operations: []v2.OperationResult{updateFailed, packetFailed}, wantErr: true},
		{name: "no update needed", operations: []v2.OperationResult{packetSucceeded}},
		{name: "update only", operations: []v2.OperationResult{updateFailed, updateSucceeded}},
		{name: "failed update only", operations: []v2.OperationResult{updateFailed}, wantErr: true},
		{name: "empty batch", wantErr: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := validateRelaySimulation(t.Context(), slog.Default(), v2.RelaySimulation{Operations: tc.operations})
			if tc.wantErr {
				require.ErrorContains(t, err, "no packet calls can currently succeed")
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestRelaySimulationFailureDetails(t *testing.T) {
	for _, tc := range []struct {
		name    string
		success bool
	}{
		{"all packets fail", false},
		{"partial success", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// ARRANGE
			var logs bytes.Buffer
			logger := slog.New(slog.NewTextHandler(&logs, &slog.HandlerOptions{Level: slog.LevelDebug}))
			simulation := v2.RelaySimulation{
				Operations: []v2.OperationResult{
					{Type: v2.OperationTypeUpdate, FailureReason: "IBCClientNotFound(client-0)"},
					{Type: v2.OperationTypePacket, Success: tc.success, FailureReason: "empty revert data"},
					{Type: v2.OperationTypeUpdate, Success: true},
					{Type: v2.OperationTypePacket, FailureReason: "RateLimitExceeded()"},
					{Type: v2.OperationTypePacket, FailureReason: "0xdeadbeef"},
				},
			}

			// ACT
			err := validateRelaySimulation(t.Context(), logger, simulation)

			// ASSERT
			var details string
			if tc.success {
				require.NoError(t, err)
				details = logs.String()
			} else {
				require.ErrorContains(t, err, "no packet calls can currently succeed")
				details = err.Error()
			}
			require.Contains(t, details, "call[0]: IBCClientNotFound(client-0)")
			require.Contains(t, details, "call[3]: RateLimitExceeded()")
			require.Contains(t, details, "call[4]: 0xdeadbeef")
			require.NotContains(t, details, "call[2]:")
		})
	}
}

func TestRelaySimulationFailureDetailsBounded(t *testing.T) {
	simulation := v2.RelaySimulation{Operations: make([]v2.OperationResult, 8)}
	for i := range simulation.Operations {
		simulation.Operations[i] = v2.OperationResult{
			Type:          v2.OperationTypePacket,
			FailureReason: strings.Repeat("x", 1024),
		}
	}
	err := validateRelaySimulation(t.Context(), slog.Default(), simulation)
	require.Error(t, err)
	require.Contains(t, err.Error(), "...;")
	require.NotContains(t, err.Error(), "call[5]:")
	require.Less(t, len(err.Error()), 1500)
}

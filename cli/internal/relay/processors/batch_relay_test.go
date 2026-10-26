// SPDX-License-Identifier: Apache-2.0

package processors

import (
	"log/slog"
	"testing"

	"github.com/stretchr/testify/require"

	v2 "github.com/cosmos/ibc/cli/internal/types/v2"
)

func TestValidateRelaySimulation(t *testing.T) {
	update := v2.OperationResult{Type: v2.OperationTypeUpdate, Success: true}
	misbehavior := v2.OperationResult{Type: v2.OperationTypeUpdate, Success: true, Misbehavior: true}
	packet := v2.OperationResult{Type: v2.OperationTypePacket, Success: true}
	failedPacket := v2.OperationResult{Type: v2.OperationTypePacket, FailureReason: "boom"}

	for _, tc := range []struct {
		name    string
		ops     []v2.OperationResult
		wantErr string
	}{
		{"update and packet", []v2.OperationResult{update, packet}, ""},
		{"update only", []v2.OperationResult{update}, ""},
		{"one packet succeeds", []v2.OperationResult{packet, failedPacket}, ""},
		{"no packet succeeds", []v2.OperationResult{update, failedPacket}, "no packet calls"},
		{"misbehavior with packets", []v2.OperationResult{misbehavior, packet}, "would report misbehavior"},
		{"misbehavior after another update", []v2.OperationResult{update, misbehavior}, "would report misbehavior"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := validateRelaySimulation(t.Context(), slog.Default(), v2.RelaySimulation{Operations: tc.ops})
			if tc.wantErr == "" {
				require.NoError(t, err)
				return
			}
			require.ErrorContains(t, err, tc.wantErr)
		})
	}
}

package ibclink_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/cosmos/ibc/link/e2e/e2etest"
	"github.com/cosmos/ibc/link/harness"
	"github.com/cosmos/ibc/link/harness/topology"
)

func TestGMPCall_AutoRelay(t *testing.T) {
	tests := []struct {
		name  string
		route string
	}{
		{"A_to_B", topology.RouteAtoB},
		{"B_to_A", topology.RouteBtoA},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			run := e2etest.Start(t, e2etest.SelectedTopology(t))
			ctx := t.Context()

			out, err := run.GMP(ctx, harness.GMP{Route: tt.route})
			require.NoError(t, err)
			require.NoError(t, out.VerifyComplete(ctx))
		})
	}
}

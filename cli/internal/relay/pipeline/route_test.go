// SPDX-License-Identifier: Apache-2.0

package pipeline

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/cosmos/ibc/cli/internal/relay/prover"
)

func TestResolveRoute(t *testing.T) {
	src, dst := testRoute.SourceChainID, testRoute.DestinationChainID

	tests := []struct {
		name    string
		without func(deps *Deps)
		wantErr string
	}{
		{
			"sourceChain",
			func(d *Deps) { delete(d.Chains.(staticChains), src) },
			"no configured chain client for source chain 1",
		},
		{
			"destinationChain",
			func(d *Deps) { delete(d.Chains.(staticChains), dst) },
			"no configured chain client for destination chain 8453",
		},
		{"sourceProver", func(d *Deps) {
			delete(d.Provers.(staticProvers), prover.Key(src, testRoute.SourceClientID))
		}, `no prover configured for client "base-0" on chain "1"`},
		{"destinationProver", func(d *Deps) {
			delete(d.Provers.(staticProvers), prover.Key(dst, testRoute.DestinationClientID))
		}, `no prover configured for client "ethereum-0" on chain "8453"`},
		{
			"sourceTxBuilder",
			func(d *Deps) { delete(d.TxBuilders.(staticTxBuilders), src) },
			"no tx builder configured for source chain 1",
		},
		{
			"destinationTxBuilder",
			func(d *Deps) { delete(d.TxBuilders.(staticTxBuilders), dst) },
			"no tx builder configured for destination chain 8453",
		},
		{
			"sourceTxSubmitter",
			func(d *Deps) { delete(d.TxSubmitters.(staticTxSubmitters), src) },
			"no configured tx submitter for source chain 1",
		},
		{"destinationTxSubmitter", func(d *Deps) {
			delete(d.TxSubmitters.(staticTxSubmitters), dst)
		}, "no configured tx submitter for destination chain 8453"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, deps := newPipelineEnv(t)
			tt.without(&deps)

			_, err := resolveRoute(deps, testRoute, Options{})
			require.ErrorContains(t, err, tt.wantErr)
		})
	}

	t.Run("resolvesEveryEnd", func(t *testing.T) {
		env, deps := newPipelineEnv(t)

		route, err := resolveRoute(deps, testRoute, Options{})
		require.NoError(t, err)
		require.Equal(t, testRoute, route.Route)
		require.Same(t, env.srcClient, route.Source.Chain)
		require.Same(t, env.srcProver, route.Source.Prover)
		require.Same(t, env.srcTxBuilder, route.Source.TxBuilder)
		require.Same(t, env.srcTxSubmitter, route.Source.TxSubmitter)
		require.Same(t, env.dstClient, route.Destination.Chain)
		require.Same(t, env.dstProver, route.Destination.Prover)
		require.Same(t, env.dstTxBuilder, route.Destination.TxBuilder)
		require.Same(t, env.dstTxSubmitter, route.Destination.TxSubmitter)
	})
}

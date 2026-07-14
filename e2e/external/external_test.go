// Package external_test covers harness connectivity to an out-of-band chain.
package external_test

import (
	"math/big"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/cosmos/ibc/e2e/e2etest"
	"github.com/cosmos/ibc/e2e/internal/harness/chain/evm/anvil"
	"github.com/cosmos/ibc/e2e/internal/harness/environment"
	"github.com/cosmos/ibc/e2e/internal/harness/ibclink/wire"
	"github.com/cosmos/ibc/e2e/internal/synthetic"
	"github.com/cosmos/ibc/e2e/internal/testapp"
)

// externalChainID differs from managedChainID so live validate exercises chain-id on a second node.
const (
	managedChainID                                 = 31337
	externalChainID                                = 31347
	externalEndpoint environment.EndpointBindingID = "external-chain-b"
)

func TestExternalChain_EnvironmentConnectsButDoesNotOwn(t *testing.T) {
	e2etest.RequireAnvilLane(t)
	ctx := t.Context()

	oob, err := anvil.Start(ctx, anvil.Spec{
		ID:      "chain-b-external",
		ChainID: externalChainID,
		LogPath: filepath.Join(t.TempDir(), "external-anvil.log"),
	})
	require.NoError(t, err)
	t.Cleanup(func() {
		if stopErr := oob.Stop(); stopErr != nil {
			t.Errorf("stop external Anvil: %v", stopErr)
		}
	})

	startHeight, err := oob.Height(ctx)
	require.NoError(t, err)
	signers := synthetic.NewSigners(t)
	addresses := signers.Addresses()
	require.NoError(t, oob.EnsureEOABalance(ctx, addresses.Application, synthetic.RequiredSignerBalance()))
	require.NoError(t, oob.EnsureEOABalance(ctx, addresses.Relayer, synthetic.RequiredSignerBalance()))

	suite := e2etest.SuiteFor(environment.Spec{Chains: []environment.ChainSpec{
		environment.ManagedAnvil{ID: e2etest.ChainA, EVMChainID: managedChainID},
		environment.AttachedEVM{
			ID: e2etest.ChainB, EVMChainID: externalChainID, Endpoint: externalEndpoint,
			Timing: environment.Timing{
				CompletionBudget: 60 * time.Second,
				SettleWindow:     1500 * time.Millisecond,
				PollInterval:     100 * time.Millisecond,
			},
		},
	}}, environment.Runtime{Endpoints: map[environment.EndpointBindingID]environment.EndpointBinding{
		externalEndpoint: {RPCURL: oob.RPCURL()},
	}})

	// Subtest teardown must finish before the out-of-band liveness probe below, or the check is vacuous.
	t.Run("environment", func(t *testing.T) {
		env := e2etest.Start(t, suite)
		route := synthetic.AtoB(e2etest.ChainA, e2etest.ChainB)
		driver, deployment := synthetic.Deploy(t, env, signers, route)
		ift := synthetic.BindIFT(t, env, deployment, signers, route)
		relayer := synthetic.StartRelayer(t, driver, env)
		rctx := t.Context()

		transfer, sendErr := ift.Send(rctx, testapp.IFTRequest{Amount: big.NewInt(1_500_000)})
		require.NoError(t, sendErr)

		attached, chainErr := env.Chain(e2etest.ChainB)
		require.NoError(t, chainErr)
		_, awaitErr := synthetic.AwaitState(
			rctx,
			relayer,
			transfer.Packet(),
			wire.PacketComplete,
			attached.Timing(),
		)
		require.NoError(t, awaitErr)
		require.NoError(t, transfer.VerifyDelivered(rctx))
		require.NoError(t, transfer.VerifyEscrowed(rctx))

		mining, capabilityErr := attached.Mining()
		require.Nil(t, mining)
		require.ErrorIs(t, capabilityErr, environment.ErrCapabilityUnavailable,
			"an attached Chain must not advertise mining control")
	})

	afterHeight, err := oob.Height(ctx)
	require.NoError(t, err, "Environment teardown must not have stopped the out-of-band external node")
	require.GreaterOrEqual(t, afterHeight, startHeight, "external node kept running across Environment teardown")
}

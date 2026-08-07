package e2e_test

import (
	"math/big"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cosmos/ibc/e2e/internal/e2etest"
	"github.com/cosmos/ibc/e2e/internal/harness/chain/evm/anvil"
	"github.com/cosmos/ibc/e2e/internal/harness/environment"
	relayerv2 "github.com/cosmos/ibc/link/api/v2/relayer"
)

// externalChainID differs from managedChainID so live validate exercises chain-id on a second node.
const (
	managedChainID                                 = 31337
	externalChainID                                = 31347
	externalEndpoint environment.EndpointBindingID = "external-chain-b"
)

func TestAttachedChainRemainsCallerOwned(t *testing.T) {
	t.Parallel()
	e2etest.RequireAnvilLane(t)
	ctx := t.Context()

	oob, err := anvil.Start(ctx, anvil.Spec{
		ID:      "chain-b-external",
		ChainID: externalChainID,
		LogPath: filepath.Join(t.TempDir(), "external-anvil.log"),
	})
	require.NoError(t, err)
	t.Cleanup(func() {
		assert.NoError(t, oob.Stop(), "stop external Anvil")
	})

	startHeight, err := oob.Height(ctx)
	require.NoError(t, err)
	signers := e2etest.NewSigners(t)
	addresses := signers.Addresses()
	require.NoError(t, oob.EnsureEOABalance(ctx, addresses.Application, e2etest.RequiredSignerBalance()))
	require.NoError(t, oob.EnsureEOABalance(ctx, addresses.Relayer, e2etest.RequiredSignerBalance()))
	require.NoError(t, oob.EnsureEOABalance(ctx, e2etest.ProtocolAuthorityAddress(), e2etest.RequiredSignerBalance()))

	spec := dummyClientMeshSpec([]environment.ChainSpec{
		environment.ManagedAnvil{ID: e2etest.ChainA, EVMChainID: managedChainID},
		environment.AttachedEVM{
			ID: e2etest.ChainB, EVMChainID: externalChainID, Endpoint: externalEndpoint,
			Timing: environment.Timing{
				CompletionBudget: 60 * time.Second,
				SettleWindow:     1500 * time.Millisecond,
				PollInterval:     100 * time.Millisecond,
			},
		},
	})
	runtime := e2etest.RuntimeWithProtocolDeployer(environment.Runtime{
		Endpoints: map[environment.EndpointBindingID]environment.EndpointBinding{
			externalEndpoint: {RPCURL: oob.RPCURL()},
		},
	})

	// Subtest teardown must finish before the out-of-band liveness probe below, or the check is vacuous.
	t.Run("environment", func(t *testing.T) {
		env := e2etest.Start(t, spec, runtime)
		route := e2etest.AtoB(e2etest.ChainA, e2etest.ChainB)
		driver, deployment := e2etest.Deploy(t, env, signers, route)
		transferApp := e2etest.BindTransfer(t, env, deployment, signers, route)
		relayer := e2etest.StartRelayer(t, driver, env)
		rctx := t.Context()

		transfer, sendErr := transferApp.Send(rctx, e2etest.TransferRequest{Amount: big.NewInt(1_500_000)})
		require.NoError(t, sendErr)

		attached, chainErr := env.Chain(e2etest.ChainB)
		require.NoError(t, chainErr)
		_, awaitErr := e2etest.AwaitState(rctx, relayer, transfer.Packet(),
			relayerv2.PacketState_PACKET_STATE_SUCCEEDED, attached.Timing())
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

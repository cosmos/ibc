package e2e_test

import (
	"math/big"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/cosmos/ibc/e2e/internal/e2etest"
	"github.com/cosmos/ibc/e2e/internal/harness/environment"
	relayerv2 "github.com/cosmos/ibc/link/api/v2/relayer"
)

func TestTransfer_AutoRelay(t *testing.T) {
	t.Parallel()
	spec := dummyClientMeshSpec(e2etest.EVMChains(t, e2etest.EVMRequirements{}, e2etest.ChainA, e2etest.ChainB))
	runtime := e2etest.RuntimeWithProtocolDeployer(environment.Runtime{})
	env := e2etest.Start(t, spec, runtime)
	signers := e2etest.NewSigners(t)
	route := e2etest.AtoB(e2etest.ChainA, e2etest.ChainB)
	driver, deployment := e2etest.Deploy(t, env, signers, route)
	transferApp := e2etest.BindTransfer(t, env, deployment, signers, route)
	relayer := e2etest.StartRelayer(t, driver, env)
	ctx := t.Context()

	transfer, err := transferApp.Send(ctx, e2etest.TransferRequest{Amount: big.NewInt(1_234_000)})
	require.NoError(t, err)
	require.NoError(t, transfer.VerifyEscrowed(ctx))

	_, err = e2etest.AwaitState(ctx, relayer, transfer.Packet(),
		relayerv2.PacketState_PACKET_STATE_SUCCEEDED)
	require.NoError(t, err)
	require.NoError(t, transfer.VerifyDelivered(ctx))
}

func TestTransfer_ManualRelay(t *testing.T) {
	t.Parallel()
	spec := dummyClientMeshSpec(e2etest.EVMChains(t, e2etest.EVMRequirements{}, e2etest.ChainA, e2etest.ChainB))
	runtime := e2etest.RuntimeWithProtocolDeployer(environment.Runtime{})
	env := e2etest.Start(t, spec, runtime)
	signers := e2etest.NewSigners(t)
	route := e2etest.ManualAtoB(e2etest.ChainA, e2etest.ChainB)
	driver, deployment := e2etest.Deploy(t, env, signers, route)
	transferApp := e2etest.BindTransfer(t, env, deployment, signers, route)
	relayer := e2etest.StartRelayer(t, driver, env)
	ctx := t.Context()

	transfer, err := transferApp.Send(ctx, e2etest.TransferRequest{Amount: big.NewInt(1_234_000)})
	require.NoError(t, err)
	require.NoError(t, transfer.VerifyEscrowed(ctx))

	require.NoError(t, e2etest.AwaitStable(ctx, relayer, transfer.Packet(),
		relayerv2.PacketState_PACKET_STATE_PENDING))
	require.NoError(t, transfer.VerifyNotMinted(ctx))
	require.NoError(t, e2etest.Relay(ctx, relayer, transfer.Packet()))
	_, err = e2etest.AwaitState(ctx, relayer, transfer.Packet(),
		relayerv2.PacketState_PACKET_STATE_SUCCEEDED)
	require.NoError(t, err)
	require.NoError(t, transfer.VerifyDelivered(ctx))
}

const (
	// transferTimeout must elapse in real time before the relayer times a
	// packet out (the pipeline gates on the relayer's own clock), so it has
	// to fit well inside the await budget.
	transferTimeout = 5 * time.Second
	// Advance well past transferTimeout so the packet is decisively expired
	// when the relayer starts.
	transferTimeoutAdvance = 5 * transferTimeout
)

func TestTransferTimeout_Refund(t *testing.T) {
	t.Parallel()
	spec := dummyClientMeshSpec(e2etest.EVMChains(t,
		e2etest.EVMRequirements{ControlledMining: true}, e2etest.ChainA, e2etest.ChainB))
	runtime := e2etest.RuntimeWithProtocolDeployer(environment.Runtime{})
	env := e2etest.Start(t, spec, runtime)
	signers := e2etest.NewSigners(t)
	route := e2etest.AtoB(e2etest.ChainA, e2etest.ChainB)
	driver, deployment := e2etest.Deploy(t, env, signers, route)
	transferApp := e2etest.BindTransfer(t, env, deployment, signers, route)
	relayer := e2etest.StartRelayer(t, driver, env)
	ctx := t.Context()

	require.NoError(t, relayer.Stop(ctx))
	transfer, err := transferApp.Send(ctx, e2etest.TransferRequest{
		Amount:  big.NewInt(3_000_000),
		Timeout: transferTimeout,
	})
	require.NoError(t, err)

	chainB, err := env.Chain(route.Destination)
	require.NoError(t, err)
	mining, err := chainB.Mining()
	require.NoError(t, err)
	require.NoError(t, mining.AdvanceTime(ctx, transferTimeoutAdvance))
	relayer = e2etest.StartRelayer(t, driver, env)

	_, err = e2etest.AwaitState(ctx, relayer, transfer.Packet(),
		relayerv2.PacketState_PACKET_STATE_TIMED_OUT)
	require.NoError(t, err)
	require.NoError(t, transfer.VerifyRefunded(ctx))
	require.NoError(t, transfer.VerifyNotMinted(ctx))
}

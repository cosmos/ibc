package e2e_test

import (
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/stretchr/testify/require"

	"github.com/cosmos/ibc/e2e/e2etest"

	relayerv2 "github.com/cosmos/ibc/link/api/v2/relayer"
)

// zeroAddressReceiver is a well-formed but invalid IFT receiver: the
// destination mint reverts on it, forcing an error acknowledgement.
var zeroAddressReceiver = (common.Address{}).Hex()

//nolint:dupl // acceptance tests keep their setup sequences deliberately explicit
func TestIFTTransfer_AutoRelay(t *testing.T) {
	t.Parallel()
	env := e2etest.Start(t, e2etest.SelectedSuite(t))
	signers := e2etest.NewSigners(t)
	route := e2etest.AtoB(e2etest.ChainA, e2etest.ChainB)
	driver, deployment := e2etest.Deploy(t, env, signers, route)
	iftApp := e2etest.BindIFT(t, env, deployment, signers, route)
	relayer := e2etest.StartRelayer(t, driver, env)
	ctx := t.Context()

	transfer, err := iftApp.Send(ctx, e2etest.IFTRequest{Amount: big.NewInt(1_234_000)})
	require.NoError(t, err)
	require.NoError(t, transfer.VerifyBurned(ctx))
	require.NoError(t, transfer.VerifyPending(ctx))

	destination, err := env.Chain(route.Destination)
	require.NoError(t, err)
	err = e2etest.AwaitState(ctx, relayer, transfer.Packet(),
		relayerv2.PacketState_PACKET_STATE_SUCCEEDED, destination.Timing())
	require.NoError(t, err)
	require.NoError(t, transfer.VerifyDelivered(ctx))
	require.NoError(t, transfer.VerifyPendingCleared(ctx))
}

func TestIFTTimeout_Refund(t *testing.T) {
	t.Parallel()
	e2etest.RequireAnvilLane(t)
	env := e2etest.Start(t, e2etest.SelectedSuite(t))
	signers := e2etest.NewSigners(t)
	route := e2etest.AtoB(e2etest.ChainA, e2etest.ChainB)
	driver, deployment := e2etest.Deploy(t, env, signers, route)
	iftApp := e2etest.BindIFT(t, env, deployment, signers, route)
	relayer := e2etest.StartRelayer(t, driver, env)
	ctx := t.Context()

	require.NoError(t, relayer.Stop(ctx))
	transfer, err := iftApp.Send(ctx, e2etest.IFTRequest{
		Amount:  big.NewInt(3_000_000),
		Timeout: transferTimeout,
	})
	require.NoError(t, err)
	require.NoError(t, transfer.VerifyBurned(ctx))
	require.NoError(t, transfer.VerifyPending(ctx))

	chainB, err := env.Chain(route.Destination)
	require.NoError(t, err)
	mining, err := chainB.Mining()
	require.NoError(t, err)
	require.NoError(t, mining.AdvanceTime(ctx, transferTimeoutAdvance))
	relayer = e2etest.StartRelayer(t, driver, env)

	source, err := env.Chain(route.Source)
	require.NoError(t, err)
	err = e2etest.AwaitState(ctx, relayer, transfer.Packet(),
		relayerv2.PacketState_PACKET_STATE_TIMED_OUT, source.Timing())
	require.NoError(t, err)
	require.NoError(t, transfer.VerifyRefunded(ctx))
	require.NoError(t, transfer.VerifyNotMinted(ctx))
	require.NoError(t, transfer.VerifyPendingCleared(ctx))
}

// TestIFTTransfer_ErrorAck_Refund sends to the zero address, which the
// destination mint rejects, forcing an error acknowledgement and a refund.
func TestIFTTransfer_ErrorAck_Refund(t *testing.T) {
	t.Parallel()
	env := e2etest.Start(t, e2etest.SelectedSuite(t))
	signers := e2etest.NewSigners(t)
	route := e2etest.AtoB(e2etest.ChainA, e2etest.ChainB)
	driver, deployment := e2etest.Deploy(t, env, signers, route)
	iftApp := e2etest.BindIFT(t, env, deployment, signers, route)
	relayer := e2etest.StartRelayer(t, driver, env)
	ctx := t.Context()

	transfer, err := iftApp.Send(ctx, e2etest.IFTRequest{
		Amount:   big.NewInt(1_234_000),
		Receiver: zeroAddressReceiver,
	})
	require.NoError(t, err)
	require.NoError(t, transfer.VerifyBurned(ctx))
	require.NoError(t, transfer.VerifyPending(ctx))

	destination, err := env.Chain(route.Destination)
	require.NoError(t, err)
	err = e2etest.AwaitState(ctx, relayer, transfer.Packet(),
		relayerv2.PacketState_PACKET_STATE_REJECTED, destination.Timing())
	require.NoError(t, err)
	require.NoError(t, transfer.VerifyNotMinted(ctx))
	require.NoError(t, transfer.VerifyRefunded(ctx))
	require.NoError(t, transfer.VerifyPendingCleared(ctx))
}

// TestIFTTransfer_ErrorAck_UnregisteredBridge leaves the destination IFT
// bridge unregistered, so onRecvPacket hits IFTBridgeNotFound.
func TestIFTTransfer_ErrorAck_UnregisteredBridge(t *testing.T) {
	t.Parallel()
	env := e2etest.Start(t, e2etest.SelectedSuite(t))
	signers := e2etest.NewSigners(t)
	route := e2etest.AtoB(e2etest.ChainA, e2etest.ChainB)
	route.SkipDestinationIFTBridge = true
	driver, deployment := e2etest.Deploy(t, env, signers, route)
	iftApp := e2etest.BindIFT(t, env, deployment, signers, route)
	relayer := e2etest.StartRelayer(t, driver, env)
	ctx := t.Context()

	transfer, err := iftApp.Send(ctx, e2etest.IFTRequest{Amount: big.NewInt(1_234_000)})
	require.NoError(t, err)
	require.NoError(t, transfer.VerifyBurned(ctx))
	require.NoError(t, transfer.VerifyPending(ctx))

	destination, err := env.Chain(route.Destination)
	require.NoError(t, err)
	err = e2etest.AwaitState(ctx, relayer, transfer.Packet(),
		relayerv2.PacketState_PACKET_STATE_REJECTED, destination.Timing())
	require.NoError(t, err)
	require.NoError(t, transfer.VerifyNotMinted(ctx))
	require.NoError(t, transfer.VerifyRefunded(ctx))
	require.NoError(t, transfer.VerifyPendingCleared(ctx))
}

// TestIFTTransfer_MultiPacketPending sends several transfers before any are
// relayed, so each sequence must get its own pending record and must only be
// cleared by its own acknowledgement.
func TestIFTTransfer_MultiPacketPending(t *testing.T) {
	t.Parallel()
	env := e2etest.Start(t, e2etest.SelectedSuite(t))
	signers := e2etest.NewSigners(t)
	route := e2etest.AtoB(e2etest.ChainA, e2etest.ChainB)
	driver, deployment := e2etest.Deploy(t, env, signers, route)
	iftApp := e2etest.BindIFT(t, env, deployment, signers, route)
	relayer := e2etest.StartRelayer(t, driver, env)
	ctx := t.Context()

	require.NoError(t, relayer.Stop(ctx))
	amounts := []*big.Int{big.NewInt(1_000_000), big.NewInt(2_000_000), big.NewInt(3_000_000)}
	transfers := make([]*e2etest.IFTPacket, len(amounts))
	for i, amount := range amounts {
		transfer, err := iftApp.Send(ctx, e2etest.IFTRequest{Amount: amount})
		require.NoError(t, err)
		require.NoError(t, transfer.VerifyBurned(ctx))
		require.NoError(t, transfer.VerifyPending(ctx))
		transfers[i] = transfer
	}

	relayer = e2etest.StartRelayer(t, driver, env)
	destination, err := env.Chain(route.Destination)
	require.NoError(t, err)
	for _, transfer := range transfers {
		err := e2etest.AwaitState(ctx, relayer, transfer.Packet(),
			relayerv2.PacketState_PACKET_STATE_SUCCEEDED, destination.Timing())
		require.NoError(t, err)
		require.NoError(t, transfer.VerifyDelivered(ctx))
		require.NoError(t, transfer.VerifyPendingCleared(ctx))
	}
}

package e2e_test

import (
	"math/big"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/cosmos/ibc/e2e/internal/e2etest"
	relayerv2 "github.com/cosmos/ibc/link/api/v2/relayer"
)

func TestGMPCall_AutoRelay(t *testing.T) {
	t.Parallel()
	spec, runtime := attestedMesh(e2etest.EVMChains(t, e2etest.EVMRequirements{}, e2etest.ChainA, e2etest.ChainB))
	env := e2etest.Start(t, spec, runtime)
	sender := e2etest.NewSigner(t)
	relayerSigner := e2etest.NewSigner(t)
	route := e2etest.AtoB(e2etest.ChainA, e2etest.ChainB)
	driver, deployment := e2etest.Deploy(t, env, sender, relayerSigner, route)
	gmp := e2etest.NewGMP(t, env, deployment, sender, route)
	relayer := e2etest.StartRelayer(t, driver, env)
	ctx := t.Context()

	call, err := gmp.Call(ctx, e2etest.GMPRequest{})
	require.NoError(t, err)
	_, err = e2etest.AwaitState(ctx, relayer, call.Packet(),
		relayerv2.PacketState_PACKET_STATE_SUCCEEDED)
	require.NoError(t, err)
	require.NoError(t, call.VerifyCounterExecuted(ctx))
}

// TestGMPCall_ICS27AccountTransfer sends a GMP call whose payload is an
// erc20.transfer executed by the destination ICS27 account.
func TestGMPCall_ICS27AccountTransfer(t *testing.T) {
	t.Parallel()
	spec, runtime := attestedMesh(e2etest.EVMChains(t, e2etest.EVMRequirements{}, e2etest.ChainA, e2etest.ChainB))
	env := e2etest.Start(t, spec, runtime)
	sender := e2etest.NewSigner(t)
	relayerSigner := e2etest.NewSigner(t)
	route := e2etest.AtoB(e2etest.ChainA, e2etest.ChainB)
	driver, deployment := e2etest.Deploy(t, env, sender, relayerSigner, route)
	gmp := e2etest.NewGMP(t, env, deployment, sender, route)
	relayer := e2etest.StartRelayer(t, driver, env)
	ctx := t.Context()

	token := gmp.Token()
	salt := []byte("ics27-account-transfer")
	id := gmp.AccountIdentifier(sender.Address(), salt)
	account, err := gmp.AccountAddress(ctx, id)
	require.NoError(t, err)

	amount := big.NewInt(1_000)
	target, err := e2etest.NewAddress()
	require.NoError(t, err)

	require.NoError(t, gmp.FundERC20(ctx, account, amount))

	payload, err := e2etest.PackERC20Transfer(target, amount)
	require.NoError(t, err)

	call, err := gmp.Call(ctx, e2etest.GMPRequest{Receiver: token.Hex(), Salt: salt, Payload: payload})
	require.NoError(t, err)
	_, err = e2etest.AwaitState(ctx, relayer, call.Packet(),
		relayerv2.PacketState_PACKET_STATE_SUCCEEDED)
	require.NoError(t, err)

	require.NoError(t, gmp.AwaitERC20Balance(ctx, account, big.NewInt(0), "ICS27 account drained"))
	require.NoError(t, gmp.AwaitERC20Balance(ctx, target, amount, "ICS27 account transfer target credited"))

	stored, err := gmp.StoredAccountIdentifier(ctx, account)
	require.NoError(t, err)
	require.Equal(t, id, stored)
}

// invalidGMPPayload does not match Counter's call surface, so delivery produces an error acknowledgement.
var invalidGMPPayload = []byte{0xde, 0xad, 0xbe, 0xef}

func TestGMPCall_ErrorAcknowledgement(t *testing.T) {
	t.Parallel()
	spec, runtime := attestedMesh(e2etest.EVMChains(t, e2etest.EVMRequirements{}, e2etest.ChainA, e2etest.ChainB))
	env := e2etest.Start(t, spec, runtime)
	sender := e2etest.NewSigner(t)
	relayerSigner := e2etest.NewSigner(t)
	route := e2etest.AtoB(e2etest.ChainA, e2etest.ChainB)
	driver, deployment := e2etest.Deploy(t, env, sender, relayerSigner, route)
	gmp := e2etest.NewGMP(t, env, deployment, sender, route)
	relayer := e2etest.StartRelayer(t, driver, env)
	ctx := t.Context()

	call, err := gmp.Call(ctx, e2etest.GMPRequest{Payload: invalidGMPPayload})
	require.NoError(t, err)
	_, err = e2etest.AwaitState(ctx, relayer, call.Packet(),
		relayerv2.PacketState_PACKET_STATE_REJECTED)
	require.NoError(t, err)
	require.NoError(t, call.VerifyCounterRejected(ctx))
}

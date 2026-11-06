// SPDX-License-Identifier: Apache-2.0

package e2e_test

import (
	"math/big"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/stretchr/testify/require"

	"github.com/cosmos/ibc-contracts/packages/go-abigen/besumsgs"
	relayerv2 "github.com/cosmos/ibc/cli/api/v2/relayer"
	"github.com/cosmos/ibc/cli/besu"
	"github.com/cosmos/ibc/e2e/internal/e2etest"
	"github.com/cosmos/ibc/e2e/internal/harness/environment"
	"github.com/cosmos/ibc/e2e/internal/harness/ibccli"
)

// Both Besu clients trust a consensus state for two minutes and the relayer
// updates them every twenty seconds; deploying the applications uses about a
// minute of that before the relayer starts. No packet flows until the
// consensus state each client was created with has expired on its host, so
// only the automatic updates can keep the clients usable: each must stay
// unfrozen and within its trusting period throughout. A round trip then
// relays through both clients.
func TestBesuQBFTClient_AutoUpdatePreventsExpiry(t *testing.T) {
	t.Parallel()
	const (
		trustingPeriod = 2 * time.Minute
		updateInterval = 20 * time.Second
	)
	spec, runtime := e2etest.BesuMesh(e2etest.BesuChains(t, besu.QBFT, besu.QBFT))
	for i, connection := range spec.Connections {
		spec.Connections[i].A = withTrustingPeriod(t, connection.A, trustingPeriod)
		spec.Connections[i].B = withTrustingPeriod(t, connection.B, trustingPeriod)
	}
	env := e2etest.Start(t, spec, runtime)
	sender := e2etest.NewSigner(t)
	relayerSigner := e2etest.NewSigner(t)
	route := e2etest.AtoB(e2etest.ChainA, e2etest.ChainB)
	driver, deployment := e2etest.DeployWithRelayerConfig(t, env, sender, relayerSigner,
		func(config *ibccli.RelayerConfig) {
			for i := range config.Connections {
				config.Connections[i].A.AutoUpdateInterval = updateInterval
				config.Connections[i].B.AutoUpdateInterval = updateInterval
			}
		}, route)
	transferApp := e2etest.NewTransfer(t, env, deployment, sender, route)

	clients := []besuExpiry{
		newBesuExpiry(t, env, route.Source, route.Destination),
		newBesuExpiry(t, env, route.Destination, route.Source),
	}
	// deadlines are the host times at which each client's initial consensus
	// state expires
	deadlines := make([]uint64, len(clients))
	for i, client := range clients {
		created := client.state()
		require.Equal(t, uint64(trustingPeriod/time.Second), created.TrustingPeriod)
		client.requireTrusted(created)
		deadlines[i] = client.consensusTime(created) + created.TrustingPeriod
	}

	relayer := e2etest.StartRelayer(t, driver, env)
	ctx := t.Context()

	poll := time.NewTicker(clients[0].host.Timing().BlockInterval)
	defer poll.Stop()
	for {
		passed := true
		for i, client := range clients {
			passed = client.requireTrusted(client.state()) >= deadlines[i] && passed
		}
		if passed {
			break
		}
		select {
		case <-ctx.Done():
			require.FailNow(t, "test context ended", ctx.Err())
		case <-poll.C:
		}
	}

	transfer, err := transferApp.Send(ctx, e2etest.TransferRequest{Amount: big.NewInt(1_000_000)})
	require.NoError(t, err)
	status, err := e2etest.AwaitState(ctx, relayer, transfer.PacketTx(),
		relayerv2.PacketState_PACKET_STATE_SUCCEEDED)
	require.NoError(t, err)
	require.NoError(t, transfer.VerifyDelivered(ctx))
	require.NoError(t, transfer.VerifyAcknowledgementExecuted(ctx, status.GetAckTx().GetTxHash()))
}

// withTrustingPeriod is client, a new Besu client, trusting states for period.
func withTrustingPeriod(t *testing.T, client environment.ClientSpec, period time.Duration) environment.ClientSpec {
	t.Helper()
	besuClient, ok := client.(environment.NewBesuClient)
	require.True(t, ok, "mesh client is %T", client)
	besuClient.TrustingPeriod = uint64(period / time.Second)
	return besuClient
}

// besuExpiry reads whether the Besu client on host, tracking tracked, is still
// within its trusting period. The contract stores only a hash of each
// consensus state, so its timestamp is read from the tracked chain's header at
// the client's latest height.
type besuExpiry struct {
	t       *testing.T
	host    *environment.Chain
	hostEVM *environment.EVM
	tracked *environment.EVM
	client  *environment.IBCClient
}

func newBesuExpiry(t *testing.T, env *environment.Environment, tracked, host environment.ChainID) besuExpiry {
	t.Helper()
	hostChain, err := env.Chain(host)
	require.NoError(t, err)
	hostEVM, err := hostChain.EVM()
	require.NoError(t, err)
	trackedChain, err := env.Chain(tracked)
	require.NoError(t, err)
	trackedEVM, err := trackedChain.EVM()
	require.NoError(t, err)
	return besuExpiry{
		t:       t,
		host:    hostChain,
		hostEVM: hostEVM,
		tracked: trackedEVM,
		client:  besuClientOn(t, env, tracked, host),
	}
}

func (e besuExpiry) state() besumsgs.IBesuLightClientMsgsClientState {
	e.t.Helper()
	var state besumsgs.IBesuLightClientMsgsClientState
	err := e.hostEVM.UseContractCaller(func(caller bind.ContractCaller) error {
		var readErr error
		state, readErr = besu.ReadClientState(e.t.Context(), caller,
			common.HexToAddress(string(e.client.LightClientAddress())))
		return readErr
	})
	require.NoError(e.t, err)
	return state
}

// consensusTime is the timestamp of the consensus state at state's latest height.
func (e besuExpiry) consensusTime(state besumsgs.IBesuLightClientMsgsClientState) uint64 {
	e.t.Helper()
	header, err := e.tracked.HeaderByNumber(e.t.Context(),
		new(big.Int).SetUint64(state.LatestHeight.RevisionHeight))
	require.NoError(e.t, err)
	return header.Time
}

func (e besuExpiry) hostTime() uint64 {
	e.t.Helper()
	head, err := e.hostEVM.HeaderByNumber(e.t.Context(), nil)
	require.NoError(e.t, err)
	return head.Time
}

// requireTrusted requires the client to be unfrozen and its latest consensus
// state to pass the contract's expiry check at the host's latest block, and
// returns that block's time.
func (e besuExpiry) requireTrusted(state besumsgs.IBesuLightClientMsgsClientState) uint64 {
	e.t.Helper()
	require.False(e.t, state.IsFrozen, "client %s is frozen", e.client.ID())
	now := e.hostTime()
	require.Greater(e.t,
		e.consensusTime(state)+state.TrustingPeriod, now,
		"client %s expired at height %d", e.client.ID(), state.LatestHeight.RevisionHeight)
	return now
}

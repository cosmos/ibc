package e2etest

import (
	"context"
	"math/big"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/cosmos/ibc/e2e/internal/harness/environment"
	"github.com/cosmos/ibc/e2e/internal/harness/ibclink"
	relayerv2 "github.com/cosmos/ibc/link/api/v2/relayer"
)

func TestValidAmountOwnsInput(t *testing.T) {
	amount := new(big.Int).Sub(new(big.Int).Lsh(big.NewInt(1), 256), big.NewInt(1))
	owned, err := validAmount(amount)
	require.NoError(t, err)

	amount.SetInt64(3)
	require.NotEqual(t, amount, owned)
	require.Equal(t, 256, owned.BitLen())
}

func TestValidAmountRejectsNonUint256(t *testing.T) {
	tooLarge := new(big.Int).Lsh(big.NewInt(1), 256)
	for _, amount := range []*big.Int{nil, new(big.Int), big.NewInt(-1), tooLarge} {
		_, err := validAmount(amount)
		require.Error(t, err)
	}
}

func TestRouteWaitPolicy(t *testing.T) {
	timing := func(block, budget, poll time.Duration) environment.Timing {
		return environment.Timing{BlockInterval: block, CompletionBudget: budget, PollInterval: poll}
	}
	policy := func(budget, poll, stability time.Duration) ibclink.WaitPolicy {
		return ibclink.WaitPolicy{
			CompletionBudget: budget,
			StatusPoll:       poll,
			StabilityWindow:  stability,
		}
	}
	anvil := timing(time.Second, 20*time.Second, 250*time.Millisecond)
	besu := timing(2*time.Second, 40*time.Second, 250*time.Millisecond)
	tests := []struct {
		name                string
		source, destination environment.Timing
		want                ibclink.WaitPolicy
	}{
		{"Anvil/Anvil", anvil, anvil, policy(40*time.Second, 250*time.Millisecond, 2*time.Second)},
		{"Besu/Besu", besu, besu, policy(80*time.Second, 250*time.Millisecond, 4*time.Second)},
		{"Anvil/Besu", anvil, besu, policy(60*time.Second, 250*time.Millisecond, 4*time.Second)},
		{
			"asymmetric attached",
			timing(500*time.Millisecond, 7*time.Second, 80*time.Millisecond),
			timing(3*time.Second, 11*time.Second, 120*time.Millisecond),
			policy(18*time.Second, 120*time.Millisecond, 6*time.Second),
		},
		{
			"zero block interval",
			timing(0, 7*time.Second, 80*time.Millisecond),
			timing(0, 11*time.Second, 120*time.Millisecond),
			policy(18*time.Second, 120*time.Millisecond, 1500*time.Millisecond),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, routeWaitPolicy(tt.source, tt.destination))
		})
	}
}

func TestAwaitStateRequiresRouteWaitPolicy(t *testing.T) {
	_, err := AwaitState(context.Background(), &ibclink.Relayer{}, Packet{Sequence: 7},
		relayerv2.PacketState_PACKET_STATE_PENDING)
	require.EqualError(t, err, "e2etest: packet sequence 7 has no route id")

	_, err = AwaitState(context.Background(), &ibclink.Relayer{}, Packet{RouteID: "missing", Sequence: 7},
		relayerv2.PacketState_PACKET_STATE_PENDING)
	require.EqualError(t, err, `e2etest: packet missing-7 has no wait policy for route "missing"`)
}

func TestAwaitStableUsesFreshWindowAfterStateReached(t *testing.T) {
	policy := ibclink.WaitPolicy{
		CompletionBudget: 200 * time.Millisecond,
		StatusPoll:       10 * time.Millisecond,
		StabilityWindow:  40 * time.Millisecond,
	}
	packet := Packet{RouteID: "route", Sequence: 1}
	calls := 0
	started := time.Now()
	err := awaitStablePacketState(context.Background(), packet,
		relayerv2.PacketState_PACKET_STATE_PENDING, policy,
		func(context.Context) (*relayerv2.PacketStatus, relayerv2.PacketState, bool, error) {
			calls++
			if calls < 3 {
				return nil, relayerv2.PacketState_PACKET_STATE_SUCCEEDED, true, nil
			}
			return nil, relayerv2.PacketState_PACKET_STATE_PENDING, true, nil
		})
	require.NoError(t, err)
	require.GreaterOrEqual(t, time.Since(started), 55*time.Millisecond)
	require.GreaterOrEqual(t, calls, 6)
}

func TestAwaitStableRejectsStateChangeDuringWindow(t *testing.T) {
	policy := ibclink.WaitPolicy{
		CompletionBudget: time.Second,
		StatusPoll:       time.Millisecond,
		StabilityWindow:  time.Second,
	}
	calls := 0
	err := awaitStablePacketState(context.Background(), Packet{RouteID: "route", Sequence: 2},
		relayerv2.PacketState_PACKET_STATE_PENDING, policy,
		func(context.Context) (*relayerv2.PacketStatus, relayerv2.PacketState, bool, error) {
			calls++
			if calls == 1 {
				return nil, relayerv2.PacketState_PACKET_STATE_PENDING, true, nil
			}
			return nil, relayerv2.PacketState_PACKET_STATE_SUCCEEDED, true, nil
		})
	require.EqualError(t, err, `packet route-2 must remain "PACKET_STATE_PENDING", got "PACKET_STATE_SUCCEEDED"`)
}

func TestAwaitStableObservesAtWindowEnd(t *testing.T) {
	policy := ibclink.WaitPolicy{
		CompletionBudget: time.Second,
		StatusPoll:       time.Hour,
		StabilityWindow:  time.Millisecond,
	}
	calls := 0
	err := awaitStablePacketState(context.Background(), Packet{RouteID: "route", Sequence: 3},
		relayerv2.PacketState_PACKET_STATE_PENDING, policy,
		func(context.Context) (*relayerv2.PacketStatus, relayerv2.PacketState, bool, error) {
			calls++
			if calls == 1 {
				return nil, relayerv2.PacketState_PACKET_STATE_PENDING, true, nil
			}
			return nil, relayerv2.PacketState_PACKET_STATE_SUCCEEDED, true, nil
		})
	require.EqualError(t, err, `packet route-3 must remain "PACKET_STATE_PENDING", got "PACKET_STATE_SUCCEEDED"`)
	require.Equal(t, 2, calls)
}

func TestAwaitStableRevalidatesTerminalStatus(t *testing.T) {
	policy := ibclink.WaitPolicy{
		CompletionBudget: time.Second,
		StatusPoll:       time.Hour,
		StabilityWindow:  time.Millisecond,
	}
	packet := Packet{RouteID: "route", Sequence: 4, SourceTxHash: "send"}
	calls := 0
	err := awaitStablePacketState(context.Background(), packet,
		relayerv2.PacketState_PACKET_STATE_SUCCEEDED, policy,
		func(context.Context) (*relayerv2.PacketStatus, relayerv2.PacketState, bool, error) {
			calls++
			status := &relayerv2.PacketStatus{
				SendTx: &relayerv2.TransactionInfo{TxHash: "send"},
				RecvTx: &relayerv2.TransactionInfo{TxHash: "recv"},
			}
			if calls == 1 {
				status.AckTx = &relayerv2.TransactionInfo{TxHash: "ack"}
			}
			return status, relayerv2.PacketState_PACKET_STATE_SUCCEEDED, true, nil
		})
	require.EqualError(t, err, "e2etest: PACKET_STATE_SUCCEEDED packet route-4 has no acknowledgement transaction")
	require.Equal(t, 2, calls)
}

func TestAwaitStableDoesNotTreatParentCancellationAsSuccess(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	calls := 0
	err := awaitStablePacketState(ctx, Packet{RouteID: "route", Sequence: 3},
		relayerv2.PacketState_PACKET_STATE_PENDING,
		ibclink.WaitPolicy{
			CompletionBudget: time.Second,
			StatusPoll:       time.Hour,
			StabilityWindow:  time.Millisecond,
		},
		func(context.Context) (*relayerv2.PacketStatus, relayerv2.PacketState, bool, error) {
			calls++
			if calls == 2 {
				cancel()
			}
			return nil, relayerv2.PacketState_PACKET_STATE_PENDING, true, nil
		})
	require.ErrorIs(t, err, context.Canceled)
	require.Equal(t, 2, calls)
}

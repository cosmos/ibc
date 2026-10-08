// SPDX-License-Identifier: Apache-2.0

package relayerload

import (
	"bytes"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cosmos/ibc/harness/environment"
	"github.com/cosmos/ibc/harness/ibccli"
)

func TestPacketAwaitPolicyScalesWithBatches(t *testing.T) {
	route := ibccli.WaitPolicy{CompletionBudget: 40 * time.Second, StatusPoll: 250 * time.Millisecond}
	anvil := environment.Timing{BlockInterval: time.Second}
	besu := environment.Timing{BlockInterval: 2 * time.Second}

	policy := packetAwaitPolicy(route, anvil, besu, 1500, 64)
	assert.Equal(t, 24*40*time.Second, policy.CompletionBudget, "ceil(1500/64) batches")
	assert.Equal(t, 2*time.Second, policy.StatusPoll, "no faster than the slower Chain's block")

	small := packetAwaitPolicy(route, anvil, anvil, 10, 64)
	assert.Equal(t, 40*time.Second, small.CompletionBudget)
	assert.Equal(t, time.Second, small.StatusPoll)
}

func TestDirectionReportFailed(t *testing.T) {
	ok := DirectionReport{Target: 2, Attempted: 2, Sent: 2, Included: 2, Packets: 2, PacketsSucceeded: 2}
	assert.False(t, ok.Failed())

	for name, mutate := range map[string]func(*DirectionReport){
		"short":       func(d *DirectionReport) { d.Attempted = 1 },
		"sendError":   func(d *DirectionReport) { d.SendErrors = 1 },
		"reverted":    func(d *DirectionReport) { d.Reverted = 1 },
		"dropped":     func(d *DirectionReport) { d.Dropped = 1 },
		"missing":     func(d *DirectionReport) { d.MissingPackets = 1 },
		"unrelayed":   func(d *DirectionReport) { d.PacketsSucceeded = 1 },
		"errorString": func(d *DirectionReport) { d.Error = "boom" },
	} {
		t.Run(name, func(t *testing.T) {
			d := ok
			mutate(&d)
			assert.True(t, d.Failed())
		})
	}
}

func TestReportTotalsAndText(t *testing.T) {
	start := time.Unix(100, 0)
	gen := generatorStats{
		Attempted: 2, Sent: 2, Included: 2, FirstSend: start, LastSend: start.Add(time.Second),
		SendToInclusion: []time.Duration{time.Second, 2 * time.Second},
	}
	await := awaitStats{Succeeded: 2, LastSettled: start.Add(5 * time.Second)}
	a := newDirectionReport("a-to-b", "chain-a", "chain-b", 2, gen, await, 2, 0, nil)
	b := newDirectionReport("b-to-a", "chain-b", "chain-a", 2, generatorStats{}, awaitStats{}, 0, 0,
		errors.New("boom"))

	assert.Equal(t, int64(1000), a.SendPhaseMS)
	assert.Equal(t, int64(4000), a.RelayDrainMS)
	assert.Equal(t, 2, a.SendToInclusion.Count)

	sum := total([]DirectionReport{a, b})
	assert.Equal(t, 4, sum.Target)
	assert.Equal(t, 2, sum.Sent)
	assert.Equal(t, "b-to-a: boom", sum.Error)

	report := &Report{Directions: []DirectionReport{a, b}, Total: sum}
	var out bytes.Buffer
	require.NoError(t, report.WriteText(&out))
	assert.Contains(t, out.String(), "Relayer load: FAIL")
	assert.Contains(t, out.String(), "[a-to-b] chain-a -> chain-b")
}

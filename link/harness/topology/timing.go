package topology

import "time"

// TimingProfile describes how fast a chain observably makes progress, so every packet-wait and reader
// budget in the harness derives from a property of the chain rather than a package-level literal tuned to
// instant-mining Anvil. A budget for "the packet's effect lands" is a function of the chains involved (their
// block interval, finality, and the daemon's scan cadence), not of the harness, so it belongs here.
//
// Each ChainSpec carries one (Timing); a zero-value Timing means "use the launcher default" via
// ChainSpec.ResolvedTiming. The profile is resolved once at provisioning and consumed at three seams: the
// on-chain readers (effect-wait completion budget + poll), the daemon-status waits (waitPacketState/Stable),
// and the managed-node launch (BlockInterval renders anvil's --block-time / besu's QBFT period so the chain
// actually mines at that rate).
type TimingProfile struct {
	// BlockInterval is the chain's block cadence; 0 means instant/on-demand mining (a tx mines the moment it
	// arrives, as un-timed Anvil does). For a managed anvil the harness renders this into --block-time, and
	// for a managed besu into the QBFT genesis block period, so the node seals at exactly this rate, keeping
	// "how the harness makes the chain progress" and "how long the harness waits for progress" derived from
	// one number.
	BlockInterval time.Duration

	// CompletionBudget bounds the wait for a packet's on-chain effect — the destination mint/delivery or
	// the source refund the reader polls for. It also bounds the daemon-status waits
	// (waitPacketState/Stable), which trail the same effect (see wire.PacketStatus) rather than a
	// separate, smaller literal.
	CompletionBudget time.Duration

	// SettleWindow is how long a status must hold unchanged before it is trusted as stable
	// (waitPacketStable). On a block-interval chain it must exceed one block so a state cannot be "confirmed
	// stable" inside a single unmined block.
	SettleWindow time.Duration

	// PollInterval is the condition-poll cadence shared by every bounded wait (status polls, effect log
	// scans, the status cross-check, and the pending-tx probe); see onchain.Await for the wait doctrine.
	PollInterval time.Duration
}

// besuQBFTBlockPeriod is the block period a managed Besu QBFT network runs at — the single source: the
// wait budgets derive from it here (blockTiming), and the harness passes the same resolved BlockInterval
// into the launcher (besu.Spec.BlockPeriod renders it into the QBFT genesis), so the cadence the network
// seals at and the cadence the waits assume cannot drift apart.
const besuQBFTBlockPeriod = 2 * time.Second

// DefaultTiming is the profile used when a ChainSpec leaves Timing at its zero value. Anvil (un-timed)
// mines on demand, so it takes the instant profile that reproduces the harness's original hard-coded
// budgets. Besu produces blocks on a fixed QBFT period, so it takes a block-derived
// profile. An unknown/empty launcher (e.g. an external chain the harness does not launch) defaults to
// instant — the POC's external nodes are instant anvils; a genuinely slow external chain sets Timing
// explicitly.
func DefaultTiming(launcher string) TimingProfile {
	switch launcher {
	case LauncherBesu:
		return blockTiming(besuQBFTBlockPeriod)
	default:
		return instantTiming()
	}
}

// instantTiming is the on-demand/instant-mining profile: a 60s completion budget, a 1.5s settle window,
// and a 100ms poll — the harness's original hard-coded budgets.
func instantTiming() TimingProfile {
	return TimingProfile{
		BlockInterval:    0,
		CompletionBudget: 60 * time.Second,
		SettleWindow:     1500 * time.Millisecond,
		PollInterval:     100 * time.Millisecond,
	}
}

// blockTiming derives a profile for a fixed-block-interval chain from its block period. A packet round trip
// costs one block for the source effect + the daemon's scan pickup + one block for the destination effect,
// so the block interval is the natural unit to scale by:
//   - CompletionBudget = 20 intervals: ~10x the ~2-block happy-path cost, so block jitter and the 250ms
//     scan cadence are absorbed without ever masking a genuine stall.
//   - SettleWindow = 2 intervals: a status must outlast a full block before it is called stable; a sub-block
//     window could "confirm" stability inside one unmined block.
//   - PollInterval = a quarter interval, clamped to [50ms, 250ms]: fine enough to catch an effect promptly,
//     coarse enough to avoid pointless RPC when blocks are seconds apart.
func blockTiming(block time.Duration) TimingProfile {
	return TimingProfile{
		BlockInterval:    block,
		CompletionBudget: 20 * block,
		SettleWindow:     2 * block,
		PollInterval:     clampPoll(block / 4),
	}
}

// clampPoll keeps a derived poll cadence in a sane band: never so fine it hammers the RPC, never so coarse
// it misses a fast effect.
func clampPoll(d time.Duration) time.Duration {
	const minPoll, maxPoll = 50 * time.Millisecond, 250 * time.Millisecond
	switch {
	case d < minPoll:
		return minPoll
	case d > maxPoll:
		return maxPoll
	default:
		return d
	}
}

// StatusRowBudget bounds the status cross-check: the daemon's persist lag behind an effect the reader
// has already observed (see wire.PacketStatus) is at most a scan tick plus a block, never the full
// completion budget — a quarter of it is ample and still scales with the chain. For the instant profile
// this yields 15s (60s/4).
func (p TimingProfile) StatusRowBudget() time.Duration { return p.CompletionBudget / 4 }

// SettleObservations is how many PollInterval-spaced samples span the settle window — the count
// waitPacketStable loops. Floored at 1 so a degenerate profile still checks the state at least once. For the
// instant profile this yields 15 (1.5s/100ms).
func (p TimingProfile) SettleObservations() int {
	n := int(p.SettleWindow / p.PollInterval)
	if n < 1 {
		return 1
	}
	return n
}

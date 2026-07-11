package topology

import "time"

type TimingProfile struct {
	// BlockInterval is zero for instant mining; managed launchers and wait budgets use the same value.
	BlockInterval time.Duration

	CompletionBudget time.Duration

	SettleWindow time.Duration

	PollInterval time.Duration
}

const besuQBFTBlockPeriod = 2 * time.Second

func DefaultTiming(launcher string) TimingProfile {
	switch launcher {
	case LauncherBesu:
		return blockTiming(besuQBFTBlockPeriod)
	default:
		return instantTiming()
	}
}

func instantTiming() TimingProfile {
	return TimingProfile{
		BlockInterval:    0,
		CompletionBudget: 60 * time.Second,
		SettleWindow:     1500 * time.Millisecond,
		PollInterval:     100 * time.Millisecond,
	}
}

func blockTiming(block time.Duration) TimingProfile {
	return TimingProfile{
		BlockInterval:    block,
		CompletionBudget: 20 * block,
		SettleWindow:     2 * block,
		PollInterval:     clampPoll(block / 4),
	}
}

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

func (p TimingProfile) StatusRowBudget() time.Duration { return p.CompletionBudget / 4 }

func (p TimingProfile) SettleObservations() int {
	n := int(p.SettleWindow / p.PollInterval)
	if n < 1 {
		return 1
	}
	return n
}

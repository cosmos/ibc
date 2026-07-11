package topology

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestDefaultTimingPerLauncher(t *testing.T) {
	require.Equal(t, instantTiming(), DefaultTiming(LauncherAnvil))
	require.Equal(t, instantTiming(), DefaultTiming(""), "unknown/external launchers default to instant")
	require.Equal(t, blockTiming(besuQBFTBlockPeriod), DefaultTiming(LauncherBesu))
}

func TestResolvedTimingZeroTakesLauncherDefault(t *testing.T) {
	spec := ChainSpec{Provision: Provision{Mode: ProvisionManaged, Launcher: LauncherBesu}}
	require.Equal(t, blockTiming(besuQBFTBlockPeriod), spec.ResolvedTiming())
}

func TestResolvedTimingPartialFillsZeroBudgets(t *testing.T) {
	// A custom profile that sets only the completion budget must have every other budget field filled
	// from the launcher default — a zero PollInterval would panic time.NewTicker, a zero SettleWindow
	// would degenerate the stability check.
	spec := ChainSpec{
		Provision: Provision{Mode: ProvisionManaged, Launcher: LauncherAnvil},
		Timing:    TimingProfile{CompletionBudget: 5 * time.Second},
	}
	got := spec.ResolvedTiming()
	def := DefaultTiming(LauncherAnvil)
	require.Equal(t, 5*time.Second, got.CompletionBudget)
	require.Equal(t, def.SettleWindow, got.SettleWindow)
	require.Equal(t, def.PollInterval, got.PollInterval)
}

func TestResolvedTimingLeavesBlockIntervalAsAuthored(t *testing.T) {
	// BlockInterval zero is a legitimate value (instant mining), never filled from the default.
	spec := ChainSpec{
		Provision: Provision{Mode: ProvisionManaged, Launcher: LauncherBesu},
		Timing:    TimingProfile{CompletionBudget: 5 * time.Second},
	}
	require.Zero(t, spec.ResolvedTiming().BlockInterval)
}

func TestBlockTimingDerivation(t *testing.T) {
	got := blockTiming(2 * time.Second)
	require.Equal(t, 2*time.Second, got.BlockInterval)
	require.Equal(t, 40*time.Second, got.CompletionBudget, "20 intervals")
	require.Equal(t, 4*time.Second, got.SettleWindow, "2 intervals")
	require.Equal(t, 250*time.Millisecond, got.PollInterval, "quarter interval clamped to 250ms")
}

func TestClampPoll(t *testing.T) {
	require.Equal(t, 50*time.Millisecond, clampPoll(10*time.Millisecond))
	require.Equal(t, 100*time.Millisecond, clampPoll(100*time.Millisecond))
	require.Equal(t, 250*time.Millisecond, clampPoll(2*time.Second))
}

func TestSettleObservationsFlooredAtOne(t *testing.T) {
	// A degenerate profile (window shorter than one poll) must still check the state at least once.
	degenerate := TimingProfile{SettleWindow: time.Millisecond, PollInterval: time.Second}
	require.Equal(t, 1, degenerate.SettleObservations())
}

package onchain

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

const (
	testBudget = 300 * time.Millisecond
	testPoll   = 5 * time.Millisecond
)

func TestAwaitImmediateSuccess(t *testing.T) {
	v, err := Await(t.Context(), testBudget, testPoll, "value", func(context.Context) (int, bool, error) {
		return 42, true, nil
	})
	require.NoError(t, err)
	require.Equal(t, 42, v)
}

func TestAwaitRetriesThroughTransientErrors(t *testing.T) {
	calls := 0
	v, err := Await(t.Context(), testBudget, testPoll, "value", func(context.Context) (int, bool, error) {
		calls++
		if calls < 3 {
			return 0, false, errors.New("transient probe hiccup")
		}
		return 7, true, nil
	})
	require.NoError(t, err)
	require.Equal(t, 7, v)
	require.Equal(t, 3, calls)
}

func TestAwaitFatalObservationReturnsImmediately(t *testing.T) {
	fatal := errors.New("matched effect contradicts the action")
	calls := 0
	_, err := Await(t.Context(), testBudget, testPoll, "value", func(context.Context) (int, bool, error) {
		calls++
		return 0, true, fatal
	})
	require.ErrorIs(t, err, fatal)
	require.Equal(t, 1, calls, "a fatal observation must not be retried")
}

func TestAwaitDeadlineCarriesDescAndLastProbeError(t *testing.T) {
	probeErr := errors.New("still at height 3")
	_, err := Await(
		t.Context(), 40*time.Millisecond, testPoll, "the mint on chain-b",
		func(context.Context) (int, bool, error) { return 0, false, probeErr },
	)
	require.Error(t, err)
	require.ErrorIs(t, err, context.DeadlineExceeded)
	require.ErrorIs(t, err, probeErr)
	require.ErrorContains(t, err, "waiting for the mint on chain-b")
}

func TestAwaitDeadlineWithoutProbeError(t *testing.T) {
	_, err := Await(
		t.Context(), 40*time.Millisecond, testPoll, "the mint on chain-b",
		func(context.Context) (int, bool, error) { return 0, false, nil },
	)
	require.Error(t, err)
	require.ErrorIs(t, err, context.DeadlineExceeded)
	require.NotContains(t, err.Error(), "last probe error")
}

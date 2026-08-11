// SPDX-License-Identifier: Apache-2.0

package e2etest

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestAwaitReturnsCompletedValue(t *testing.T) {
	calls := 0
	got, err := await(
		context.Background(),
		time.Second,
		time.Millisecond,
		"value",
		func(context.Context) (int, bool, error) {
			calls++
			return 42, true, nil
		},
	)
	require.NoError(t, err)
	require.Equal(t, 42, got)
	require.Equal(t, 1, calls)
}

func TestAwaitRetriesTransientObservationError(t *testing.T) {
	transient := errors.New("temporarily unavailable")
	calls := 0
	got, err := await(
		context.Background(),
		time.Second,
		time.Millisecond,
		"value",
		func(context.Context) (int, bool, error) {
			calls++
			if calls == 1 {
				return 0, false, transient
			}
			return 42, true, nil
		},
	)
	require.NoError(t, err)
	require.Equal(t, 42, got)
	require.Equal(t, 2, calls)
}

func TestAwaitReturnsTerminalObservationError(t *testing.T) {
	terminal := errors.New("invalid observation")
	calls := 0
	got, err := await(
		context.Background(),
		time.Second,
		time.Millisecond,
		"value",
		func(context.Context) (int, bool, error) {
			calls++
			return 42, true, terminal
		},
	)
	require.ErrorIs(t, err, terminal)
	require.Zero(t, got)
	require.Equal(t, 1, calls)
}

func TestAwaitTimeoutIncludesLastObservationError(t *testing.T) {
	lastObservation := errors.New("status unavailable")
	calls := 0
	got, err := await(
		context.Background(),
		time.Nanosecond,
		time.Hour,
		"packet status",
		func(context.Context) (int, bool, error) {
			calls++
			return 0, false, lastObservation
		},
	)
	require.ErrorIs(t, err, context.DeadlineExceeded)
	require.ErrorIs(t, err, lastObservation)
	require.EqualError(
		t,
		err,
		"waiting for packet status: context deadline exceeded (last observation error: status unavailable)",
	)
	require.Zero(t, got)
	require.Equal(t, 1, calls)
}

func TestAwaitCancellationWithoutObservationError(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	got, err := await(
		ctx,
		time.Second,
		time.Hour,
		"packet status",
		func(context.Context) (int, bool, error) { return 0, false, nil },
	)
	require.ErrorIs(t, err, context.Canceled)
	require.EqualError(t, err, "waiting for packet status: context canceled")
	require.Zero(t, got)
}

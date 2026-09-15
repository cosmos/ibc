// SPDX-License-Identifier: Apache-2.0

package processors

import (
	"context"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestClientLockCancellationAndIsolation(t *testing.T) {
	unlock, err := lockClient(t.Context(), "chain", t.Name())
	require.NoError(t, err)
	defer unlock()
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	_, err = lockClient(ctx, "chain", t.Name())
	require.ErrorIs(t, err, context.Canceled)

	// A held lock must not block another client or chain.
	for _, key := range [][2]string{{"other-chain", t.Name()}, {"chain", "other-client"}} {
		release, lockErr := lockClient(t.Context(), key[0], key[1])
		require.NoError(t, lockErr)
		release()
	}
}

func TestClientLockSerializesConcurrentRelays(t *testing.T) {
	var active atomic.Int32
	var overlap atomic.Bool
	var failed atomic.Bool
	var wg sync.WaitGroup
	for range 20 {
		wg.Go(func() {
			for range 10 {
				unlock, err := lockClient(t.Context(), "chain", t.Name())
				if err != nil {
					failed.Store(true)
					return
				}
				if active.Add(1) != 1 {
					overlap.Store(true)
				}
				runtime.Gosched()
				active.Add(-1)
				unlock()
			}
		})
	}
	wg.Wait()
	require.False(t, failed.Load())
	require.False(t, overlap.Load())
	require.Zero(t, active.Load())
}

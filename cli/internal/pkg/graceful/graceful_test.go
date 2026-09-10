// SPDX-License-Identifier: Apache-2.0

package graceful

import (
	"errors"
	"os"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGraceful(t *testing.T) {
	t.Run("AddCallback", func(t *testing.T) {
		t.Run("runs callbacks in reverse order and ignores callback errors", func(t *testing.T) {
			// ARRANGE
			restoreHandler := replaceHandler(t)
			defer restoreHandler()

			calls := make([]string, 0, 3)
			AddCallback(func() error {
				calls = append(calls, "first")
				return nil
			})
			AddCallback(func() error {
				calls = append(calls, "second")
				return errors.New("callback failed")
			})
			AddCallback(func() error {
				calls = append(calls, "third")
				return nil
			})

			errCh := startWaitShutdown(globalHandler)

			// ACT
			globalHandler.stop <- syscall.SIGTERM
			err := waitShutdownResult(t, errCh)

			// ASSERT
			require.NoError(t, err)
			assert.Equal(t, []string{"third", "second", "first"}, calls)
		})
	})

	t.Run("WaitShutdown", func(t *testing.T) {
		t.Run("returns force shutdown when interrupted during callbacks", func(t *testing.T) {
			// ARRANGE
			h := newTestHandler()
			callbackStarted := make(chan struct{})
			releaseCallback := make(chan struct{})
			defer close(releaseCallback)

			h.add(func() error {
				close(callbackStarted)
				<-releaseCallback
				return nil
			})
			h.signalNotify = func(forceNotify chan<- os.Signal, _ ...os.Signal) {
				go func() {
					<-callbackStarted
					forceNotify <- syscall.SIGTERM
				}()
			}

			errCh := startWaitShutdown(h)

			// ACT
			h.stop <- syscall.SIGTERM
			err := waitShutdownResult(t, errCh)

			// ASSERT
			require.ErrorIs(t, err, ErrForceShutdown)
		})

		t.Run("returns timeout when callbacks do not finish", func(t *testing.T) {
			// ARRANGE
			h := newTestHandler()
			h.shutdownTimeout = 10 * time.Millisecond

			releaseCallback := make(chan struct{})
			defer close(releaseCallback)

			h.add(func() error {
				<-releaseCallback
				return nil
			})

			errCh := startWaitShutdown(h)

			// ACT
			h.stop <- syscall.SIGTERM
			err := waitShutdownResult(t, errCh)

			// ASSERT
			require.ErrorIs(t, err, ErrTimeoutExceeded)
		})
	})
}

func replaceHandler(t *testing.T) func() {
	t.Helper()

	original := globalHandler
	globalHandler = newTestHandler()

	return func() {
		globalHandler = original
	}
}

func newTestHandler() *shutdownHandler {
	h := newHandler(make(chan os.Signal, 1))
	h.signalNotify = func(chan<- os.Signal, ...os.Signal) {}
	h.shutdownTimeout = time.Second
	return h
}

func startWaitShutdown(h *shutdownHandler) <-chan error {
	errCh := make(chan error, 1)
	go func() {
		errCh <- h.waitShutdown()
	}()
	return errCh
}

func waitShutdownResult(t *testing.T, errCh <-chan error) error {
	t.Helper()

	select {
	case err := <-errCh:
		return err
	case <-time.After(time.Second):
		require.FailNow(t, "timed out waiting for shutdown")
	}

	return nil
}

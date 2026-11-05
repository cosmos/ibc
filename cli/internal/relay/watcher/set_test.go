// SPDX-License-Identifier: Apache-2.0

package watcher

import (
	"log/slog"
	"testing"
	"testing/synctest"
	"time"

	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cosmos/ibc/cli/internal/chains"
	"github.com/cosmos/ibc/cli/internal/config"
	"github.com/cosmos/ibc/cli/internal/tests/mocks"
)

func testSetConfig(autoRelay bool) config.Config {
	connections := testConnections()
	connections[0].ClientA.AutoRelay = config.AutoRelayConfig{Enabled: &autoRelay}

	return config.Config{
		Chains: []config.ChainConfig{
			{ChainID: sourceChainID, EVM: &config.EVMChainConfig{}},
			{ChainID: destChainID, EVM: &config.EVMChainConfig{}},
		},
		Relayer: config.RelayerConfig{Connections: connections},
	}
}

func testClientSet(t *testing.T) *chains.ClientSet {
	t.Helper()

	return chains.NewClientSet(map[string]chains.Client{
		sourceChainID: mocks.NewMockClient(t),
		destChainID:   mocks.NewMockClient(t),
	})
}

func TestNewSetFromConfig(t *testing.T) {
	t.Run("oneWatcherPerAutoRelayedChain", func(t *testing.T) {
		set, err := NewSetFromConfig(testSetConfig(true), testClientSet(t), watcherStore(t), slog.Default())
		require.NoError(t, err)
		require.Len(t, set, 1)
		assert.Equal(t, sourceChainID, set[0].chainID)
		assert.Equal(t, []string{sourceClientID}, set[0].clientIDs)
		assert.Equal(t, DefaultMinBackoff, set[0].cfg.MinBackoff)
		assert.Equal(t, DefaultMaxBackoff, set[0].cfg.MaxBackoff)
		assert.Equal(t, config.DefaultClearInterval, set[0].cfg.ClearInterval)
		assert.True(t, set[0].cfg.CleanOnStart)
		assert.False(t, set[0].cfg.AbandonUnrecoverablePackets)
	})

	t.Run("noAutoRelayedEndsWatchNothing", func(t *testing.T) {
		set, err := NewSetFromConfig(testSetConfig(false), testClientSet(t), watcherStore(t), slog.Default())
		require.NoError(t, err)
		assert.Empty(t, set)
	})

	t.Run("missingChainClientErrors", func(t *testing.T) {
		set, err := NewSetFromConfig(
			testSetConfig(true), chains.NewClientSet(nil), watcherStore(t), slog.Default(),
		)
		require.ErrorContains(t, err, sourceChainID)
		assert.Nil(t, set)
	})
}

// TestSetStartKeepsSiblings covers a chain whose first subscribe fails: the
// chain that did subscribe stays up while the other retries, and Stop tears
// both down.
func TestSetStartKeepsSiblings(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		running, failing := newFakeChain(t), newFakeChain(t)
		failing.failNextSub(errors.New("dial failed"))

		set := Set{
			newTestWatcher(running, watcherStore(t)),
			newTestWatcher(failing, watcherStore(t)),
		}

		set.Start()
		synctest.Wait()

		assert.False(t, running.latestSub().unsubscribed)
		assert.Zero(t, failing.openedSubs())

		time.Sleep(DefaultMinBackoff)
		synctest.Wait()
		assert.Equal(t, 1, failing.openedSubs())

		require.NoError(t, set.Stop())
		assert.True(t, running.latestSub().unsubscribed)
		assert.True(t, failing.latestSub().unsubscribed)
	})
}

func TestSetStartStop(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		chain := newFakeChain(t)
		set := Set{newTestWatcher(chain, watcherStore(t))}

		set.Start()
		synctest.Wait()

		require.NoError(t, set.Stop())
		assert.True(t, chain.latestSub().unsubscribed)
	})
}

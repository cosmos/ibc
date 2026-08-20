// SPDX-License-Identifier: Apache-2.0

package watcher

import (
	"log/slog"
	"testing"
	"testing/synctest"

	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cosmos/ibc/link/internal/chains"
	"github.com/cosmos/ibc/link/internal/config"
	"github.com/cosmos/ibc/link/internal/tests/mocks"
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
		set, err := NewSetFromConfig(testSetConfig(true), testClientSet(t), newPacketStore(nil), slog.Default())
		require.NoError(t, err)
		require.Len(t, set, 1)
		assert.Equal(t, sourceChainID, set[0].chainID)
		assert.Equal(t, []string{sourceClientID}, set[0].clientIDs)
	})

	t.Run("noAutoRelayedEndsWatchNothing", func(t *testing.T) {
		set, err := NewSetFromConfig(testSetConfig(false), testClientSet(t), newPacketStore(nil), slog.Default())
		require.NoError(t, err)
		assert.Empty(t, set)
	})

	t.Run("missingChainClientErrors", func(t *testing.T) {
		set, err := NewSetFromConfig(
			testSetConfig(true), chains.NewClientSet(nil), newPacketStore(nil), slog.Default(),
		)
		require.ErrorContains(t, err, sourceChainID)
		assert.Nil(t, set)
	})
}

// TestSetStartUnwinds covers the watchers a failed start leaves behind: the
// ones already running have to be stopped, or their subscriptions outlive the
// startup that failed.
func TestSetStartUnwinds(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		running, failing := newChain(), newChain()
		failing.failNext(errors.New("dial failed"))

		set := Set{
			newTestWatcher(running, newPacketStore(nil)),
			newTestWatcher(failing, newPacketStore(nil)),
		}

		require.ErrorContains(t, set.Start(), sourceChainID)
		synctest.Wait()

		assert.True(t, running.unsubscribed)
	})
}

func TestSetStartStop(t *testing.T) {
	c := newChain()
	set := Set{newTestWatcher(c, newPacketStore(nil))}

	require.NoError(t, set.Start())
	require.NoError(t, set.Stop())

	assert.True(t, c.unsubscribed)
}

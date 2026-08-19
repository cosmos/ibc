// SPDX-License-Identifier: Apache-2.0

package watcher

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/rpc"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cosmos/ibc/link/internal/chains"
	"github.com/cosmos/ibc/link/internal/config"
	"github.com/cosmos/ibc/link/internal/tests/mocks"
)

const testRouter = "0xe20BccD900Fa1B48f46F5a483d9De063b07eDFCC"

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

func TestNewSetFromConfig(t *testing.T) {
	cfg := testSetConfig(true)

	t.Run("oneWatcherPerAutoRelayedChain", func(t *testing.T) {
		clientSet := chains.NewClientSet(map[string]chains.Client{
			sourceChainID: mocks.NewMockClient(t),
			destChainID:   mocks.NewMockClient(t),
		})

		set, err := NewSetFromConfig(cfg, clientSet, mocks.NewMockPacketStore(t), slog.Default())
		require.NoError(t, err)
		require.Len(t, set, 1)
		assert.Equal(t, sourceChainID, set[0].chainID)
		assert.Equal(t, []string{sourceClientID}, set[0].clientIDs)
	})

	t.Run("missingChainClientErrors", func(t *testing.T) {
		set, err := NewSetFromConfig(cfg, chains.NewClientSet(nil), mocks.NewMockPacketStore(t), slog.Default())
		require.ErrorContains(t, err, sourceChainID)
		assert.Nil(t, set)
	})
}

// logsAPI serves eth_subscribe("logs") and reports the filter it was given.
type logsAPI struct {
	subscribed chan json.RawMessage
}

func (a *logsAPI) Logs(ctx context.Context, criteria json.RawMessage) (*rpc.Subscription, error) {
	notifier, supported := rpc.NotifierFromContext(ctx)
	if !supported {
		return nil, rpc.ErrNotificationsUnsupported
	}

	a.subscribed <- criteria

	return notifier.CreateSubscription(), nil
}

func testWebsocket(t *testing.T) (string, *logsAPI) {
	t.Helper()

	api := &logsAPI{subscribed: make(chan json.RawMessage, 1)}

	srv := rpc.NewServer()
	require.NoError(t, srv.RegisterName("eth", api))

	httpSrv := httptest.NewServer(srv.WebsocketHandler([]string{"*"}))
	t.Cleanup(httpSrv.Close)

	return "ws" + strings.TrimPrefix(httpSrv.URL, "http"), api
}

// TestSetSubscribes drives the whole discovery path a running relayer takes:
// config to chain client to a real websocket, which is where a wrong filter
// topic or a missing endpoint shows up and a scripted subscriber cannot.
func TestSetSubscribes(t *testing.T) {
	start := func(t *testing.T, autoRelay bool) *logsAPI {
		t.Helper()

		wsURL, api := testWebsocket(t)

		cfg := testSetConfig(autoRelay)
		cfg.Chains[0].EVM = &config.EVMChainConfig{
			RPC:         "http://127.0.0.1:1",
			WS:          wsURL,
			ICS26Router: testRouter,
		}
		cfg.Chains[1].EVM = &config.EVMChainConfig{
			RPC:         "http://127.0.0.1:1",
			ICS26Router: testRouter,
		}

		clientSet, err := chains.NewClientSetFromConfig(cfg)
		require.NoError(t, err)

		set, err := NewSetFromConfig(cfg, clientSet, watcherStore(t), slog.Default())
		require.NoError(t, err)

		require.NoError(t, set.Start())
		t.Cleanup(func() { require.NoError(t, set.Stop()) })

		return api
	}

	t.Run("anAutoRelayedEndSubscribesOnStart", func(t *testing.T) {
		api := start(t, true)

		select {
		case criteria := <-api.subscribed:
			filter := strings.ToLower(string(criteria))
			assert.Contains(t, filter, strings.ToLower(testRouter))
			assert.Contains(t, filter, crypto.Keccak256Hash([]byte(sourceClientID)).Hex())
		case <-time.After(waitFor):
			t.Fatal("no send packet subscription opened")
		}
	})

	t.Run("noAutoRelayedEndsWatchNothing", func(t *testing.T) {
		api := start(t, false)

		assert.Empty(t, api.subscribed)
	})
}

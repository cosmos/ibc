package ibclink

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParseReadiness(t *testing.T) {
	valid := `{"event":"ready","configLoaded":true,"dbReady":true,` +
		`"chainsConnected":["chain-a"],"relayerSubscribed":true,"status":{"http":"127.0.0.1:4242"}}` + "\n"
	res := parseReadiness(valid)
	require.NoError(t, res.err)
	require.Equal(t, "127.0.0.1:4242", res.readiness.Status.HTTP)
	require.Equal(t, []string{"chain-a"}, res.readiness.ChainsConnected)

	res = parseReadiness("plain log line, not json\n")
	require.Error(t, res.err)
	require.ErrorContains(t, res.err, "not readiness JSON")

	wrongEvent := `{"event":"started","configLoaded":true,"dbReady":true,"relayerSubscribed":true,` +
		`"status":{"http":"127.0.0.1:4242"}}`
	res = parseReadiness(wrongEvent)
	require.Error(t, res.err)
	require.ErrorContains(t, res.err, "invalid readiness")

	notReady := `{"event":"ready","configLoaded":true,"dbReady":false,"relayerSubscribed":true,` +
		`"status":{"http":"127.0.0.1:4242"}}`
	res = parseReadiness(notReady)
	require.Error(t, res.err)
	require.ErrorContains(t, res.err, "dbReady")
}

func TestDaemonInstancePaths(t *testing.T) {
	require.Equal(t, "/w/ibc-link-daemon-2.log", daemonLogPath("/w/ibc-link.log", 2))
	require.Equal(t, "/w/ibc-link-daemon-2.pid", daemonPidPath("/w/ibc-link.log", 2))
}

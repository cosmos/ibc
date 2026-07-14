package ibclink

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/cosmos/ibc/e2e/internal/harness/ibclink/wire"
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

func TestRelayerOmitsPartialHTTPErrorBody(t *testing.T) {
	const partialBody = "partial response must stay hidden"
	readErr := errors.New("read failed")
	relayer := &Relayer{
		httpAddr: "127.0.0.1:1",
		http: &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusServiceUnavailable,
				Status:     "503 Service Unavailable",
				Header:     make(http.Header),
				Body: io.NopCloser(io.MultiReader(
					strings.NewReader(partialBody),
					errorReader{err: readErr},
				)),
			}, nil
		})},
	}

	err := relayer.probeHealth(context.Background())
	require.ErrorIs(t, err, readErr)
	require.NotContains(t, err.Error(), partialBody)
	require.Contains(t, err.Error(), "read response body: read failed")
}

func TestRelayerOmitsTruncatedHTTPErrorBody(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte(strings.Repeat("x", errorBodyLimit+1)))
	}))
	defer server.Close()
	relayer := &Relayer{
		httpAddr: strings.TrimPrefix(server.URL, "http://"),
		http:     server.Client(),
	}

	err := relayer.probeHealth(context.Background())
	require.Error(t, err)
	require.Contains(t, err.Error(), "response body omitted")
}

func TestRelayerPreservesStatusReason(t *testing.T) {
	const reason = "upstream failed at https://user:password@rpc.example.invalid/?token=value"
	body, err := json.Marshal(wire.Status{Packets: []wire.PacketStatus{{
		PacketID: "packet-1",
		State:    wire.PacketTimedOut,
		Reason:   reason,
	}}})
	require.NoError(t, err)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(body)
	}))
	defer server.Close()
	relayer := &Relayer{
		httpAddr: strings.TrimPrefix(server.URL, "http://"),
		http:     server.Client(),
	}

	status, err := relayer.Status(context.Background(), wire.StatusQuery{})
	require.NoError(t, err)
	require.Len(t, status.Packets, 1)
	require.Equal(t, reason, status.Packets[0].Reason)
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) { return f(req) }

type errorReader struct{ err error }

func (r errorReader) Read([]byte) (int, error) { return 0, r.err }

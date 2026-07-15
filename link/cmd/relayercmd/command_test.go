package relayercmd

import (
	"encoding/json"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"
)

func TestCommandRunsInjectedRunHandler(t *testing.T) {
	called := false
	cmd := NewCommand(func(_ *cobra.Command, _ []string, options RunOptions) error {
		called = true
		require.True(t, options.NoMigrate)
		return nil
	})
	cmd.SetArgs([]string{"run", "--no-migrate"})

	require.NoError(t, cmd.Execute())
	require.True(t, called)
}

func TestTransportJSONSerialization(t *testing.T) {
	tests := []struct {
		name  string
		value any
		want  string
	}{
		{
			name: "readiness",
			value: Readiness{
				Event: ReadinessEvent, ConfigLoaded: true, DBReady: true,
				ChainsConnected: []string{"chain-a"}, RelayerSubscribed: true,
				Status: ReadinessStatus{HTTP: "127.0.0.1:1234"},
			},
			want: `{"event":"ready","configLoaded":true,"dbReady":true,"chainsConnected":["chain-a"],"relayerSubscribed":true,"status":{"http":"127.0.0.1:1234"}}`,
		},
		{name: "relay", value: RelayResult{PacketIDs: []string{"route-ift-1"}}, want: `{"packetIds":["route-ift-1"]}`},
		{
			name: "status omits empty terminal fields",
			value: Status{Packets: []PacketStatus{{
				PacketID: "route-ift-1", RouteID: "route", Sequence: 1,
				State: PacketPending, SourceTxHash: "0xsource",
			}}},
			want: `{"packets":[{"packetId":"route-ift-1","routeId":"route","sequence":1,"state":"pending","sourceTxHash":"0xsource"}]}`,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			encoded, err := json.Marshal(test.value)
			require.NoError(t, err)
			require.JSONEq(t, test.want, string(encoded))
		})
	}
}

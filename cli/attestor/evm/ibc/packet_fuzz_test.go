// SPDX-License-Identifier: Apache-2.0

package ibc

import (
	"testing"

	"github.com/stretchr/testify/require"

	channeltypesv2 "github.com/cosmos/ibc-go/v11/modules/core/04-channel/v2/types"
)

func FuzzDecodePacket(f *testing.F) {
	seed, err := EncodePacket(channeltypesv2.Packet{
		Sequence:          7,
		SourceClient:      "source-client",
		DestinationClient: "destination-client",
		TimeoutTimestamp:  1_700_000_000,
		Payloads: []channeltypesv2.Payload{{
			SourcePort:      "transfer",
			DestinationPort: "transfer",
			Version:         "ics20-1",
			Encoding:        "application/json",
			Value:           []byte("payload"),
		}},
	})
	require.NoError(f, err)
	f.Add(seed)

	f.Fuzz(func(t *testing.T, data []byte) {
		packet, err := DecodePacket(data)
		if err != nil {
			return
		}

		encoded, err := EncodePacket(packet)
		require.NoError(t, err)
		got, err := DecodePacket(encoded)
		require.NoError(t, err)
		require.Equal(t, packet, got)
	})
}

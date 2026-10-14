// SPDX-License-Identifier: Apache-2.0

package ibc

import (
	"reflect"
	"testing"

	channeltypesv2 "github.com/cosmos/ibc-go/v11/modules/core/04-channel/v2/types"
)

func FuzzDecodePacket(f *testing.F) {
	for _, packet := range []channeltypesv2.Packet{
		{},
		{
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
		},
	} {
		data, err := EncodePacket(packet)
		if err != nil {
			f.Fatal(err)
		}
		f.Add(data)
	}

	f.Fuzz(func(t *testing.T, data []byte) {
		packet, err := DecodePacket(data)
		if err != nil {
			return
		}

		encoded, err := EncodePacket(packet)
		if err != nil {
			t.Fatalf("re-encoding decoded packet: %v", err)
		}
		got, err := DecodePacket(encoded)
		if err != nil || !reflect.DeepEqual(got, packet) {
			t.Fatalf("round trip gave (%+v, %v), want %+v", got, err, packet)
		}
	})
}

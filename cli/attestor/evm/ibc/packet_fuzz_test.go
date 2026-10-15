// SPDX-License-Identifier: Apache-2.0

package ibc

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func FuzzDecodePacket(f *testing.F) {
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
